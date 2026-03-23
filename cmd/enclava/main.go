package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"enclava-go/internal/auth"
	"enclava-go/internal/config"
	"enclava-go/internal/extract"
	"enclava-go/internal/httpx"
	"enclava-go/internal/llm"
	extractmodule "enclava-go/internal/modules/extract"
	"enclava-go/internal/store/sqlite"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config error: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	db, err := sqlite.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("database init error: %v", err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			log.Printf("database close failed: %v", err)
		}
	}()

	apiKeysRepo := sqlite.NewAPIKeysRepo(db.SQL)
	templatesRepo := sqlite.NewExtractTemplatesRepo(db.SQL)
	settingsRepo := sqlite.NewExtractSettingsRepo(db.SQL)
	jobsRepo := sqlite.NewExtractJobsRepo(db.SQL)
	usageRepo := sqlite.NewUsageRepo(db.SQL)

	if err := settingsRepo.EnsureDefaultRow(ctx, cfg.MaxUploadSizeMB); err != nil {
		log.Fatalf("settings init error: %v", err)
	}
	if err := templatesRepo.EnsureDefaults(ctx, extract.DefaultTemplates()); err != nil {
		log.Fatalf("template seed error: %v", err)
	}
	if err := apiKeysRepo.EnsureBootstrapKey(ctx, cfg.BootstrapAPIKey, cfg.APIKeyHashAlgo, cfg.APIKeyPepper, cfg.BootstrapMinLength); err != nil {
		log.Fatalf("bootstrap key init error: %v", err)
	}

	authSvc := auth.NewService(apiKeysRepo, cfg.APIKeyPepper)
	provider, err := buildProvider(cfg)
	if err != nil {
		log.Fatalf("provider init error: %v", err)
	}
	prober := llm.NewProber(provider, cfg.ProviderProbeInterval, cfg.ProviderProbeTimeout)
	go prober.Start(ctx)

	extractSvc := extract.NewService(
		templatesRepo,
		settingsRepo,
		jobsRepo,
		provider,
		cfg.MaxUploadSizeMB,
	)

	app := httpx.NewApp(
		authSvc,
		provider,
		prober,
		apiKeysRepo,
		jobsRepo,
		templatesRepo,
		settingsRepo,
		extractSvc,
		usageRepo,
		cfg.APIKeyHashAlgo,
		cfg.APIKeyPepper,
		cfg.APIKeyPrefix,
		cfg.AdminEmail,
		cfg.AdminPassword,
		cfg.MaxUploadSizeMB,
		cfg.TrustProxyHeaders,
	)

	extractMod := extractmodule.New(extractSvc, cfg.MaxUploadSizeMB)
	extractMod.Register(app.Mux(), app)

	srv := &http.Server{
		Addr:              cfg.Addr(),
		Handler:           app.Handler(),
		ReadTimeout:       cfg.ReadTimeout,
		WriteTimeout:      cfg.WriteTimeout,
		ReadHeaderTimeout: cfg.ReadHeaderTimeout,
		IdleTimeout:       cfg.IdleTimeout,
	}

	go func() {
		log.Printf("enclava-go listening on %s", cfg.Addr())
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("server error: %v", err)
		}
	}()

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("graceful shutdown failed: %v", err)
		if closeErr := srv.Close(); closeErr != nil {
			log.Printf("forced server close failed: %v", closeErr)
		}
	}
	log.Printf("shutdown complete")
}

func buildProvider(cfg config.Config) (llm.Provider, error) {
	switch cfg.Provider {
	case "privatemode", "openai":
		return llm.NewOpenAICompatibleAdapter(
			cfg.UpstreamOpenAIBaseURL,
			cfg.UpstreamOpenAIAPIKey,
			cfg.UpstreamTimeout,
		), nil
	case "tinfoil":
		provider, err := llm.NewTinfoilProvider(
			cfg.TinfoilAPIKey,
			cfg.TinfoilEnclave,
			cfg.TinfoilRepo,
			cfg.UpstreamTimeout,
			cfg.TinfoilVerifyAtStart,
			cfg.TinfoilVerifyPerCall,
			cfg.TinfoilVerifyMaxAge,
		)
		if err != nil {
			return nil, err
		}
		if attested, ok := any(provider).(llm.AttestationProvider); ok {
			status := attested.AttestationStatus()
			if status.Verified {
				log.Printf("tinfoil attestation verified: enclave=%s repo=%s digest=%s", status.Enclave, status.Repo, status.Digest)
			}
		}
		return provider, nil
	default:
		return nil, fmt.Errorf("unsupported INFERENCE_PROVIDER %q", cfg.Provider)
	}
}
