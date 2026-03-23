package httpx

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"enclava-go/internal/auth"
	"enclava-go/internal/extract"
	"enclava-go/internal/llm"
	"enclava-go/internal/store/sqlite"
)

type App struct {
	authSvc          *auth.Service
	provider         llm.Provider
	prober           *llm.Prober
	keysRepo         *sqlite.APIKeysRepo
	extractJobs      *sqlite.ExtractJobsRepo
	extractTemplates *sqlite.ExtractTemplatesRepo
	extractSettings  *sqlite.ExtractSettingsRepo
	extractSvc       *extract.Service
	usageRepo        *sqlite.UsageRepo
	apiKeyAlgo       string
	apiKeyPepper     string
	apiKeyPrefix     string
	adminEmail       string
	adminPassword    string
	maxUploadMB      int64
	trustProxy       bool
	rl               *RateLimiter
	mux              *http.ServeMux
}

const (
	preAuthRateLimitPerMinute = 120
	preAuthRateLimitPerHour   = 1000
	preAuthRateLimitPerDay    = 20000

	maxSSELineBytes  = 256 * 1024
	maxSSEEventBytes = 128 * 1024
)

type rateLimitCheck struct {
	limit  int64
	window time.Duration
}

func NewApp(
	authSvc *auth.Service,
	provider llm.Provider,
	prober *llm.Prober,
	keysRepo *sqlite.APIKeysRepo,
	extractJobs *sqlite.ExtractJobsRepo,
	extractTemplates *sqlite.ExtractTemplatesRepo,
	extractSettings *sqlite.ExtractSettingsRepo,
	extractSvc *extract.Service,
	usageRepo *sqlite.UsageRepo,
	apiKeyAlgo string,
	apiKeyPepper string,
	apiKeyPrefix string,
	adminEmail string,
	adminPassword string,
	maxUploadMB int64,
	trustProxy bool,
) *App {
	a := &App{
		authSvc:          authSvc,
		provider:         provider,
		prober:           prober,
		keysRepo:         keysRepo,
		extractJobs:      extractJobs,
		extractTemplates: extractTemplates,
		extractSettings:  extractSettings,
		extractSvc:       extractSvc,
		usageRepo:        usageRepo,
		apiKeyAlgo:       apiKeyAlgo,
		apiKeyPepper:     apiKeyPepper,
		apiKeyPrefix:     apiKeyPrefix,
		adminEmail:       strings.TrimSpace(adminEmail),
		adminPassword:    adminPassword,
		maxUploadMB:      maxUploadMB,
		trustProxy:       trustProxy,
		rl:               NewRateLimiter(),
	}

	a.mux = http.NewServeMux()
	a.registerBaseRoutes()
	return a
}

func (a *App) Handler() http.Handler {
	return recoverMiddleware(a.mux)
}

func (a *App) Mux() *http.ServeMux {
	return a.mux
}

// Authenticated exposes the platform auth wrapper for modules.
func (a *App) Authenticated(scope string, next func(http.ResponseWriter, *http.Request, auth.Principal)) http.HandlerFunc {
	return a.authenticated(scope, next)
}

func (a *App) registerBaseRoutes() {
	a.mux.HandleFunc("/health", a.handleHealth)
	a.mux.HandleFunc("/livez", a.handleLivez)
	a.mux.HandleFunc("/readyz", a.handleReadyz)

	a.mux.HandleFunc("/api/v1/models", a.authenticated("models.list", a.handleModels))
	a.mux.HandleFunc("/api/v1/models/", a.authenticated("models.list", a.handleModelByID))
	a.mux.HandleFunc("/api/v1/chat/completions", a.authenticated("chat.completions", a.handleChatCompletions))
	a.mux.HandleFunc("/api/v1/embeddings", a.authenticated("embeddings.create", a.handleEmbeddings))
	a.mux.HandleFunc("/api/v1/attestation/status", a.authenticated("models.list", a.handleAttestationStatus))
	a.mux.HandleFunc("/api/v1/attestation/verify", a.authenticated("models.list", a.handleAttestationVerify))

	a.mux.HandleFunc("/api/v1/admin/api-keys", a.authenticated("platform.admin", a.handleAdminAPIKeys))
	a.mux.HandleFunc("/api/v1/admin/api-keys/", a.authenticated("platform.admin", a.handleAdminAPIKeyByID))
	a.mux.HandleFunc("/api/v1/admin/usage/summary", a.authenticated("platform.admin", a.handleAdminUsageSummary))

	a.registerDashboardRoutes()
}

func recoverMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if recovered := recover(); recovered != nil {
				log.Printf("panic: %v", recovered)
				WriteOpenAIError(w, http.StatusInternalServerError, "internal server error", "server_error", "internal_error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func (a *App) authenticated(scope string, next func(http.ResponseWriter, *http.Request, auth.Principal)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		apiKey := auth.ParseAPIKey(r)
		now := time.Now().UTC()
		if a.rl != nil {
			if !a.enforceRateLimits(w, a.unauthenticatedRateLimitKey(r, apiKey), now, []rateLimitCheck{
				{limit: preAuthRateLimitPerMinute, window: time.Minute},
				{limit: preAuthRateLimitPerHour, window: time.Hour},
				{limit: preAuthRateLimitPerDay, window: 24 * time.Hour},
			}) {
				return
			}
		}
		principal, err := a.authSvc.Authenticate(r.Context(), apiKey)
		if err != nil {
			WriteUnauthorized(w, err.Error())
			return
		}
		if a.rl != nil {
			checks := []rateLimitCheck{}
			if principal.RateLimitPerMinute != nil {
				checks = append(checks, rateLimitCheck{limit: int64(*principal.RateLimitPerMinute), window: time.Minute})
			}
			if principal.RateLimitPerHour != nil {
				checks = append(checks, rateLimitCheck{limit: int64(*principal.RateLimitPerHour), window: time.Hour})
			}
			if principal.RateLimitPerDay != nil {
				checks = append(checks, rateLimitCheck{limit: int64(*principal.RateLimitPerDay), window: 24 * time.Hour})
			}
			if !a.enforceRateLimits(w, fmt.Sprintf("api-key:%d", principal.APIKeyID), now, checks) {
				return
			}
		}
		if len(principal.AllowedEndpoints) > 0 {
			path := r.URL.Path
			if _, ok := principal.AllowedEndpoints["*"]; !ok {
				if !endpointAllowed(principal.AllowedEndpoints, path) {
					WriteForbidden(w, "endpoint is not allowed for this API key")
					return
				}
			}
		}
		if len(principal.AllowedIPs) > 0 {
			if _, ok := principal.AllowedIPs["*"]; !ok {
				ip := clientIP(r, a.trustProxy)
				if ip == "" {
					WriteForbidden(w, "source IP could not be determined")
					return
				}
				if _, ok := principal.AllowedIPs[ip]; !ok {
					WriteForbidden(w, "source IP is not allowed for this API key")
					return
				}
			}
		}
		if !auth.HasScope(principal, scope) {
			WriteForbidden(w, fmt.Sprintf("missing required scope: %s", scope))
			return
		}
		next(w, r.WithContext(auth.WithPrincipal(r.Context(), principal)), principal)
	}
}

func (a *App) unauthenticatedRateLimitKey(r *http.Request, apiKey string) string {
	if key := strings.TrimSpace(apiKey); key != "" {
		if len(key) > 12 {
			key = key[:12]
		}
		return "api-key:" + key
	}
	ip := strings.TrimSpace(clientIP(r, a.trustProxy))
	if ip == "" {
		return "ip:unknown"
	}
	return "ip:" + ip
}

func (a *App) enforceRateLimits(w http.ResponseWriter, key string, now time.Time, checks []rateLimitCheck) bool {
	if a.rl == nil || len(checks) == 0 {
		return true
	}
	for _, check := range checks {
		if check.limit <= 0 {
			continue
		}
		dec := a.rl.AllowKey(key, check.limit, check.window, now)
		if !dec.Allowed {
			writeRateLimited(w, dec)
			return false
		}
	}
	return true
}

func (a *App) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteMethodNotAllowed(w)
		return
	}
	payload := map[string]any{
		"status": "ok",
		"time":   time.Now().UTC().Format(time.RFC3339),
	}
	if a.prober != nil {
		payload["provider"] = a.prober.Snapshot()
	}
	if attested, ok := a.provider.(llm.AttestationProvider); ok {
		status := attested.AttestationStatus()
		// /health is unauthenticated; redact sensitive attestation details.
		payload["attestation"] = map[string]any{
			"enabled":     status.Enabled,
			"provider":    status.Provider,
			"verified":    status.Verified,
			"verified_at": status.VerifiedAt,
		}
	}
	WriteJSON(w, http.StatusOK, payload)
}

func (a *App) handleLivez(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteMethodNotAllowed(w)
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{
		"status": "ok",
		"time":   time.Now().UTC().Format(time.RFC3339),
	})
}

func (a *App) handleReadyz(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteMethodNotAllowed(w)
		return
	}
	if a.prober == nil {
		WriteJSON(w, http.StatusOK, map[string]any{"ready": true})
		return
	}
	snap := a.prober.Snapshot()
	if !snap.Ready {
		WriteJSON(w, http.StatusServiceUnavailable, snap)
		return
	}
	WriteJSON(w, http.StatusOK, snap)
}

func (a *App) handleModels(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
	if r.Method != http.MethodGet {
		WriteMethodNotAllowed(w)
		return
	}
	if a.prober != nil && !a.prober.Ready() {
		WriteOpenAIError(w, http.StatusServiceUnavailable, "provider not ready", "server_error", "provider_not_ready")
		return
	}
	modelsResp, err := a.provider.ListModels(r.Context())
	if err != nil {
		a.WriteProviderError(w, err)
		return
	}
	if len(principal.AllowedModels) > 0 {
		filtered := make([]llm.Model, 0, len(modelsResp.Data))
		for _, m := range modelsResp.Data {
			if auth.ModelAllowed(principal, m.ID) {
				filtered = append(filtered, m)
			}
		}
		modelsResp.Data = filtered
	}
	WriteJSON(w, http.StatusOK, modelsResp)
}

func (a *App) handleModelByID(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
	if r.Method != http.MethodGet {
		WriteMethodNotAllowed(w)
		return
	}
	if a.prober != nil && !a.prober.Ready() {
		WriteOpenAIError(w, http.StatusServiceUnavailable, "provider not ready", "server_error", "provider_not_ready")
		return
	}
	modelID := strings.TrimPrefix(r.URL.Path, "/api/v1/models/")
	if strings.TrimSpace(modelID) == "" {
		WriteOpenAIError(w, http.StatusBadRequest, "model id is required", "invalid_request_error", "missing_model")
		return
	}
	if !auth.ModelAllowed(principal, modelID) {
		WriteForbidden(w, "model is not allowed for this API key")
		return
	}
	model, err := a.provider.GetModel(r.Context(), modelID)
	if err != nil {
		a.WriteProviderError(w, err)
		return
	}
	WriteJSON(w, http.StatusOK, model)
}

func (a *App) handleChatCompletions(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
	if r.Method != http.MethodPost {
		WriteMethodNotAllowed(w)
		return
	}
	if a.prober != nil && !a.prober.Ready() {
		WriteOpenAIError(w, http.StatusServiceUnavailable, "provider not ready", "server_error", "provider_not_ready")
		return
	}
	if err := a.EnforceBudget(r.Context(), principal); err != nil {
		a.WriteBudgetError(w, err)
		return
	}
	requestBytes, err := io.ReadAll(io.LimitReader(r.Body, 16*1024*1024))
	if err != nil {
		WriteOpenAIError(w, http.StatusBadRequest, "invalid request body", "invalid_request_error", "invalid_body")
		return
	}

	var chatReq llm.ChatRequest
	if err := json.Unmarshal(requestBytes, &chatReq); err != nil {
		WriteOpenAIError(w, http.StatusBadRequest, "invalid JSON payload", "invalid_request_error", "invalid_json")
		return
	}
	if strings.TrimSpace(chatReq.Model) == "" {
		WriteOpenAIError(w, http.StatusBadRequest, "model is required", "invalid_request_error", "missing_model")
		return
	}
	if !auth.ModelAllowed(principal, chatReq.Model) {
		WriteForbidden(w, "model is not allowed for this API key")
		return
	}

	if chatReq.Stream {
		stream, err := a.provider.CreateChatCompletionStream(r.Context(), requestBytes)
		if err != nil {
			a.WriteProviderError(w, err)
			return
		}
		defer func() {
			_ = stream.Close()
		}()

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no")
		w.WriteHeader(http.StatusOK)

		flusher, _ := w.(http.Flusher)
		scanner := bufio.NewScanner(stream)
		scanner.Buffer(make([]byte, 0, maxSSELineBytes), maxSSELineBytes)
		usage := usageParsed{}
		usageSeen := false
		streamStatusCode := http.StatusOK
		eventData := make([]byte, 0, 256)

		for {
			if !scanner.Scan() {
				if err := scanner.Err(); err != nil {
					streamStatusCode = http.StatusBadGateway
				}
				break
			}

			line := scanner.Text()
			if _, writeErr := io.WriteString(w, line); writeErr != nil {
				streamStatusCode = http.StatusBadGateway
				break
			}
			if _, writeErr := w.Write([]byte{'\n'}); writeErr != nil {
				streamStatusCode = http.StatusBadGateway
				break
			}
			if flusher != nil {
				flusher.Flush()
			}

			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "data:") {
				data := strings.TrimSpace(strings.TrimPrefix(trimmed, "data:"))
				if data == "" || data == "[DONE]" {
					continue
				}
				if len(eventData)+len(data)+1 > maxSSEEventBytes {
					// Prevent unbounded memory usage from pathological SSE payloads.
					eventData = eventData[:0]
					continue
				}
				if data != "" && len(eventData) > 0 {
					eventData = append(eventData, '\n')
				}
				eventData = append(eventData, []byte(data)...)
			} else if trimmed == "" && len(eventData) > 0 {
				parsed := parseUsageFromResponse(eventData)
				if parsed != (usageParsed{}) {
					if parsed.Model == "" {
						parsed.Model = chatReq.Model
					}
					usage = parsed
					usageSeen = true
				}
				eventData = eventData[:0]
			}
		}
		if len(eventData) > 0 {
			parsed := parseUsageFromResponse(eventData)
			if parsed != (usageParsed{}) {
				if parsed.Model == "" {
					parsed.Model = chatReq.Model
				}
				usage = parsed
				usageSeen = true
			}
		}

		if !usageSeen {
			usage.Model = chatReq.Model
		}
		if a.usageRepo != nil {
			if usage.TotalTokens == 0 {
				usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
			}
			recordCtx, recordCancel := context.WithTimeout(context.WithoutCancel(r.Context()), 5*time.Second)
			defer recordCancel()
			_ = a.usageRepo.Record(recordCtx, sqlite.UsageRecord{
				ID:               NewID("usage"),
				APIKeyID:         principal.APIKeyID,
				Endpoint:         "/api/v1/chat/completions",
				Model:            usage.Model,
				PromptTokens:     usage.PromptTokens,
				CompletionTokens: usage.CompletionTokens,
				TotalTokens:      usage.TotalTokens,
				CostCents:        0,
				StatusCode:       streamStatusCode,
			})
		}
		return
	}

	resp, err := a.provider.CreateChatCompletion(r.Context(), requestBytes)
	if err != nil {
		a.WriteProviderError(w, err)
		return
	}
	if a.usageRepo != nil {
		u := parseUsageFromResponse(resp)
		recordCtx, recordCancel := context.WithTimeout(context.WithoutCancel(r.Context()), 5*time.Second)
		defer recordCancel()
		_ = a.usageRepo.Record(recordCtx, sqlite.UsageRecord{
			ID:               NewID("usage"),
			APIKeyID:         principal.APIKeyID,
			Endpoint:         "/api/v1/chat/completions",
			Model:            chatReq.Model,
			PromptTokens:     u.PromptTokens,
			CompletionTokens: u.CompletionTokens,
			TotalTokens:      u.TotalTokens,
			CostCents:        0,
			StatusCode:       http.StatusOK,
		})
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(resp)
}

func (a *App) handleEmbeddings(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
	if r.Method != http.MethodPost {
		WriteMethodNotAllowed(w)
		return
	}
	if a.prober != nil && !a.prober.Ready() {
		WriteOpenAIError(w, http.StatusServiceUnavailable, "provider not ready", "server_error", "provider_not_ready")
		return
	}
	if err := a.EnforceBudget(r.Context(), principal); err != nil {
		a.WriteBudgetError(w, err)
		return
	}
	requestBytes, err := io.ReadAll(io.LimitReader(r.Body, 8*1024*1024))
	if err != nil {
		WriteOpenAIError(w, http.StatusBadRequest, "invalid request body", "invalid_request_error", "invalid_body")
		return
	}

	var payload struct {
		Model string `json:"model"`
	}
	if err := json.Unmarshal(requestBytes, &payload); err != nil {
		WriteOpenAIError(w, http.StatusBadRequest, "invalid JSON payload", "invalid_request_error", "invalid_json")
		return
	}
	if strings.TrimSpace(payload.Model) == "" {
		WriteOpenAIError(w, http.StatusBadRequest, "model is required", "invalid_request_error", "missing_model")
		return
	}
	if !auth.ModelAllowed(principal, payload.Model) {
		WriteForbidden(w, "model is not allowed for this API key")
		return
	}

	resp, err := a.provider.CreateEmbeddings(r.Context(), requestBytes)
	if err != nil {
		a.WriteProviderError(w, err)
		return
	}
	if a.usageRepo != nil {
		u := parseUsageFromResponse(resp)
		if u.TotalTokens == 0 {
			u.TotalTokens = u.PromptTokens + u.CompletionTokens
		}
		recordCtx, recordCancel := context.WithTimeout(context.WithoutCancel(r.Context()), 5*time.Second)
		defer recordCancel()
		_ = a.usageRepo.Record(recordCtx, sqlite.UsageRecord{
			ID:               NewID("usage"),
			APIKeyID:         principal.APIKeyID,
			Endpoint:         "/api/v1/embeddings",
			Model:            payload.Model,
			PromptTokens:     u.PromptTokens,
			CompletionTokens: u.CompletionTokens,
			TotalTokens:      u.TotalTokens,
			CostCents:        0,
			StatusCode:       http.StatusOK,
		})
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(resp)
}

func (a *App) handleAttestationStatus(w http.ResponseWriter, r *http.Request, _ auth.Principal) {
	if r.Method != http.MethodGet {
		WriteMethodNotAllowed(w)
		return
	}
	attested, ok := a.provider.(llm.AttestationProvider)
	if !ok {
		WriteJSON(w, http.StatusOK, map[string]any{
			"enabled":  false,
			"provider": "non-attested",
		})
		return
	}
	WriteJSON(w, http.StatusOK, attested.AttestationStatus())
}

func (a *App) handleAttestationVerify(w http.ResponseWriter, r *http.Request, _ auth.Principal) {
	if r.Method != http.MethodPost {
		WriteMethodNotAllowed(w)
		return
	}
	attested, ok := a.provider.(llm.AttestationProvider)
	if !ok {
		WriteOpenAIError(w, http.StatusBadRequest, "configured provider does not support attestation verification", "invalid_request_error", "attestation_unsupported")
		return
	}
	status, err := attested.VerifyAttestation(r.Context())
	if err != nil {
		log.Printf("attestation verification failed: %v", err)
		WriteOpenAIError(w, http.StatusBadGateway, "attestation verification failed", "server_error", "attestation_verify_failed")
		return
	}
	WriteJSON(w, http.StatusOK, status)
}

func (a *App) WriteProviderError(w http.ResponseWriter, err error) {
	var upstreamErr *llm.UpstreamError
	if errors.As(err, &upstreamErr) {
		if len(upstreamErr.Body) > 0 {
			var existing map[string]any
			if json.Unmarshal(upstreamErr.Body, &existing) == nil {
				if _, ok := existing["error"]; ok {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(upstreamErr.StatusCode)
					_, _ = w.Write(upstreamErr.Body)
					return
				}
			}
		}
		WriteOpenAIError(w, upstreamErr.StatusCode, "upstream provider error", "server_error", "upstream_error")
		return
	}
	log.Printf("upstream provider error: %v", err)
	WriteOpenAIError(w, http.StatusBadGateway, "upstream provider unavailable", "server_error", "upstream_unavailable")
}
