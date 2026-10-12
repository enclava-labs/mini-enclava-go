package config

import "testing"

func TestLoadTinfoilAllowsDefaultEnclaveAndRepo(t *testing.T) {
	clearConfigEnv(t)
	t.Setenv("INFERENCE_PROVIDER", "tinfoil")
	t.Setenv("TINFOIL_API_KEY", "tf_test_key")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.Provider != "tinfoil" {
		t.Fatalf("expected tinfoil provider, got %q", cfg.Provider)
	}
	if cfg.TinfoilEnclave != "" || cfg.TinfoilRepo != "" {
		t.Fatalf("expected empty explicit Tinfoil target, got enclave=%q repo=%q", cfg.TinfoilEnclave, cfg.TinfoilRepo)
	}
}

func TestLoadTinfoilRequiresEnclaveAndRepoTogether(t *testing.T) {
	clearConfigEnv(t)
	t.Setenv("INFERENCE_PROVIDER", "tinfoil")
	t.Setenv("TINFOIL_API_KEY", "tf_test_key")
	t.Setenv("TINFOIL_ENCLAVE", "inference.tinfoil.sh")

	if _, err := Load(); err == nil {
		t.Fatalf("expected error for half-configured Tinfoil target")
	}
}

func TestLoadCORSAllowedOrigins(t *testing.T) {
	clearConfigEnv(t)
	t.Setenv("PRIVATEMODE_API_KEY", "pm_test_key")
	t.Setenv("PRIVATEMODE_BASE_URL", "https://example.invalid/v1")
	t.Setenv("CORS_ALLOWED_ORIGINS", "https://app.example.com, https://admin.example.com")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if len(cfg.CORSAllowedOrigins) != 2 {
		t.Fatalf("expected two origins, got %#v", cfg.CORSAllowedOrigins)
	}
	if cfg.CORSAllowedOrigins[0] != "https://app.example.com" || cfg.CORSAllowedOrigins[1] != "https://admin.example.com" {
		t.Fatalf("unexpected origins: %#v", cfg.CORSAllowedOrigins)
	}
}

func TestLoadOpenAICompatibleProvider(t *testing.T) {
	clearConfigEnv(t)
	t.Setenv("INFERENCE_PROVIDER", "openai_compatible")
	t.Setenv("OPENAI_COMPATIBLE_API_KEY", "oa_test_key")
	t.Setenv("OPENAI_COMPATIBLE_BASE_URL", "https://llm.example.com/v1")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.Provider != "openai_compatible" || cfg.OpenAICompatibleBaseURL != "https://llm.example.com/v1" {
		t.Fatalf("unexpected provider config: %#v", cfg)
	}
}

func TestLoadRedPillProvider(t *testing.T) {
	clearConfigEnv(t)
	t.Setenv("INFERENCE_PROVIDER", "redpill")
	t.Setenv("REDPILL_API_KEY", "rp_test_key")
	t.Setenv("REDPILL_BASE_URL", "https://redpill.example.com/v1")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.Provider != "redpill" || cfg.RedPillBaseURL != "https://redpill.example.com/v1" {
		t.Fatalf("unexpected provider config: %#v", cfg)
	}
}

func clearConfigEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"APP_HOST",
		"APP_PORT",
		"DATABASE_URL",
		"HTTP_READ_TIMEOUT",
		"HTTP_WRITE_TIMEOUT",
		"HTTP_IDLE_TIMEOUT",
		"HTTP_READ_HEADER_TIMEOUT",
		"MAX_UPLOAD_SIZE_MB",
		"INFERENCE_PROVIDER",
		"PRIVATEMODE_BASE_URL",
		"PRIVATEMODE_API_KEY",
		"PRIVATEMODE_TIMEOUT",
		"OPENAI_COMPATIBLE_BASE_URL",
		"OPENAI_COMPATIBLE_API_KEY",
		"REDPILL_BASE_URL",
		"REDPILL_API_KEY",
		"PROVIDER_PROBE_INTERVAL",
		"PROVIDER_PROBE_TIMEOUT",
		"PROVIDER_MODEL_CACHE_TTL",
		"QDRANT_URL",
		"DEFAULT_EMBEDDING_MODEL",
		"TINFOIL_API_KEY",
		"TINFOIL_ENCLAVE",
		"TINFOIL_REPO",
		"TINFOIL_VERIFY_AT_START",
		"TINFOIL_VERIFY_PER_CALL",
		"TINFOIL_VERIFY_MAX_AGE",
		"API_KEY_HASH_ALGO",
		"API_KEY_PEPPER",
		"API_KEY_PREFIX",
		"CORS_ALLOWED_ORIGINS",
		"ADMIN_EMAIL",
		"ADMIN_PASSWORD",
		"TRUST_PROXY_HEADERS",
		"BOOTSTRAP_API_KEY",
		"BOOTSTRAP_ENABLED",
		"BOOTSTRAP_API_KEY_MIN_LENGTH",
	} {
		t.Setenv(key, "")
	}
}
