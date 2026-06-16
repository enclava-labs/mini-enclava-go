package config

import (
	"strings"
	"testing"
)

func TestLoadAllowsTinfoilDefaultRouting(t *testing.T) {
	clearConfigEnv(t)
	t.Setenv("INFERENCE_PROVIDER", "tinfoil")
	t.Setenv("TINFOIL_API_KEY", "test-tinfoil-key")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Provider != "tinfoil" {
		t.Fatalf("Provider = %q, want tinfoil", cfg.Provider)
	}
	if cfg.TinfoilEnclave != "" {
		t.Fatalf("TinfoilEnclave = %q, want empty for SDK default routing", cfg.TinfoilEnclave)
	}
	if cfg.TinfoilRepo != "" {
		t.Fatalf("TinfoilRepo = %q, want empty for SDK default routing", cfg.TinfoilRepo)
	}
}

func TestLoadRejectsPartialTinfoilRouting(t *testing.T) {
	tests := []struct {
		name    string
		enclave string
		repo    string
	}{
		{name: "enclave only", enclave: "inference.tinfoil.sh"},
		{name: "repo only", repo: "tinfoilsh/confidential-model-router"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearConfigEnv(t)
			t.Setenv("INFERENCE_PROVIDER", "tinfoil")
			t.Setenv("TINFOIL_API_KEY", "test-tinfoil-key")
			t.Setenv("TINFOIL_ENCLAVE", tt.enclave)
			t.Setenv("TINFOIL_REPO", tt.repo)

			_, err := Load()
			if err == nil {
				t.Fatal("Load() error = nil, want routing error")
			}
			if !strings.Contains(err.Error(), "TINFOIL_ENCLAVE and TINFOIL_REPO must be set together") {
				t.Fatalf("Load() error = %q, want Tinfoil routing error", err)
			}
		})
	}
}

func clearConfigEnv(t *testing.T) {
	t.Helper()

	keys := []string{
		"APP_HOST",
		"APP_PORT",
		"DATABASE_URL",
		"MAX_UPLOAD_SIZE_MB",
		"HTTP_READ_TIMEOUT",
		"HTTP_WRITE_TIMEOUT",
		"HTTP_IDLE_TIMEOUT",
		"HTTP_READ_HEADER_TIMEOUT",
		"INFERENCE_PROVIDER",
		"PRIVATEMODE_BASE_URL",
		"PRIVATEMODE_API_KEY",
		"PRIVATEMODE_TIMEOUT",
		"PROVIDER_PROBE_INTERVAL",
		"PROVIDER_PROBE_TIMEOUT",
		"TINFOIL_API_KEY",
		"TINFOIL_ENCLAVE",
		"TINFOIL_REPO",
		"TINFOIL_VERIFY_AT_START",
		"TINFOIL_VERIFY_PER_CALL",
		"TINFOIL_VERIFY_MAX_AGE",
		"API_KEY_HASH_ALGO",
		"API_KEY_PEPPER",
		"API_KEY_PREFIX",
		"ADMIN_EMAIL",
		"ADMIN_PASSWORD",
		"TRUST_PROXY_HEADERS",
		"BOOTSTRAP_API_KEY",
		"BOOTSTRAP_ENABLED",
		"BOOTSTRAP_API_KEY_MIN_LENGTH",
	}
	for _, key := range keys {
		t.Setenv(key, "")
	}
}
