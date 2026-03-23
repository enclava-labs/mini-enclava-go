package config

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Host                  string
	Port                  int
	DatabaseURL           string
	ReadTimeout           time.Duration
	WriteTimeout          time.Duration
	IdleTimeout           time.Duration
	ReadHeaderTimeout     time.Duration
	MaxUploadSizeMB       int64
	Provider              string
	UpstreamOpenAIBaseURL string
	UpstreamOpenAIAPIKey  string
	UpstreamTimeout       time.Duration
	ProviderProbeInterval time.Duration
	ProviderProbeTimeout  time.Duration
	TinfoilAPIKey         string
	TinfoilEnclave        string
	TinfoilRepo           string
	TinfoilVerifyAtStart  bool
	TinfoilVerifyPerCall  bool
	TinfoilVerifyMaxAge   time.Duration
	APIKeyHashAlgo        string
	APIKeyPepper          string
	APIKeyPrefix          string
	AdminEmail            string
	AdminPassword         string
	TrustProxyHeaders     bool
	BootstrapAPIKey       string
	BootstrapEnabled      bool
	BootstrapMinLength    int
}

func Load() (Config, error) {
	privatemodeAPIKey := getEnv("PRIVATEMODE_API_KEY", "")
	privatemodeBaseURL := getEnv("PRIVATEMODE_BASE_URL", "")

	cfg := Config{
		Host:        getEnv("APP_HOST", "0.0.0.0"),
		Port:        getEnvInt("APP_PORT", 8080),
		DatabaseURL: getEnv("DATABASE_URL", "./enclava.db"),
		ReadTimeout: getEnvDuration("HTTP_READ_TIMEOUT", 30*time.Second),
		// Streaming (SSE) can exceed fixed write timeouts; prefer disabling unless you know you don't need streaming.
		WriteTimeout:          getEnvDuration("HTTP_WRITE_TIMEOUT", 0),
		IdleTimeout:           getEnvDuration("HTTP_IDLE_TIMEOUT", 60*time.Second),
		ReadHeaderTimeout:     getEnvDuration("HTTP_READ_HEADER_TIMEOUT", 10*time.Second),
		MaxUploadSizeMB:       int64(getEnvInt("MAX_UPLOAD_SIZE_MB", 15)),
		Provider:              strings.ToLower(getEnv("INFERENCE_PROVIDER", "privatemode")),
		UpstreamOpenAIBaseURL: privatemodeBaseURL,
		UpstreamOpenAIAPIKey:  privatemodeAPIKey,
		UpstreamTimeout:       getEnvDuration("PRIVATEMODE_TIMEOUT", 120*time.Second),
		ProviderProbeInterval: getEnvDuration("PROVIDER_PROBE_INTERVAL", 5*time.Second),
		ProviderProbeTimeout:  getEnvDuration("PROVIDER_PROBE_TIMEOUT", 10*time.Second),
		TinfoilAPIKey:         getEnv("TINFOIL_API_KEY", ""),
		TinfoilEnclave:        getEnv("TINFOIL_ENCLAVE", ""),
		TinfoilRepo:           getEnv("TINFOIL_REPO", ""),
		TinfoilVerifyAtStart:  getEnvBool("TINFOIL_VERIFY_AT_START", true),
		TinfoilVerifyPerCall:  getEnvBool("TINFOIL_VERIFY_PER_CALL", false),
		TinfoilVerifyMaxAge:   getEnvDuration("TINFOIL_VERIFY_MAX_AGE", 10*time.Minute),
		APIKeyHashAlgo:        getEnv("API_KEY_HASH_ALGO", "sha256"),
		APIKeyPepper:          getEnv("API_KEY_PEPPER", ""),
		APIKeyPrefix:          getEnv("API_KEY_PREFIX", "enc_"),
		AdminEmail:            getEnv("ADMIN_EMAIL", ""),
		AdminPassword:         getEnv("ADMIN_PASSWORD", ""),
		TrustProxyHeaders:     getEnvBool("TRUST_PROXY_HEADERS", false),
		BootstrapAPIKey:       getEnv("BOOTSTRAP_API_KEY", ""),
		BootstrapEnabled:      getEnvBool("BOOTSTRAP_ENABLED", false),
		BootstrapMinLength:    getEnvInt("BOOTSTRAP_API_KEY_MIN_LENGTH", 48),
	}
	if cfg.Provider == "openai" {
		cfg.Provider = "privatemode"
	}
	if cfg.Port <= 0 {
		return Config{}, fmt.Errorf("APP_PORT must be > 0")
	}
	if cfg.MaxUploadSizeMB <= 0 {
		return Config{}, fmt.Errorf("MAX_UPLOAD_SIZE_MB must be > 0")
	}
	if cfg.ProviderProbeInterval <= 0 {
		return Config{}, fmt.Errorf("PROVIDER_PROBE_INTERVAL must be > 0")
	}
	if cfg.ProviderProbeTimeout <= 0 {
		return Config{}, fmt.Errorf("PROVIDER_PROBE_TIMEOUT must be > 0")
	}
	switch cfg.APIKeyHashAlgo {
	case "sha256":
	case "hmac_sha256":
		if cfg.APIKeyPepper == "" {
			return Config{}, fmt.Errorf("API_KEY_PEPPER is required when API_KEY_HASH_ALGO=hmac_sha256")
		}
	default:
		return Config{}, fmt.Errorf("API_KEY_HASH_ALGO must be one of: sha256, hmac_sha256")
	}
	if cfg.APIKeyPrefix == "" {
		return Config{}, fmt.Errorf("API_KEY_PREFIX must be non-empty")
	}
	cfg.AdminEmail = strings.TrimSpace(cfg.AdminEmail)
	if cfg.AdminEmail == "" && strings.TrimSpace(cfg.AdminPassword) != "" {
		return Config{}, fmt.Errorf("ADMIN_EMAIL is required when ADMIN_PASSWORD is set")
	}
	if cfg.AdminEmail != "" && strings.TrimSpace(cfg.AdminPassword) == "" {
		return Config{}, fmt.Errorf("ADMIN_PASSWORD is required when ADMIN_EMAIL is set")
	}

	if cfg.BootstrapAPIKey != "" {
		cfg.BootstrapAPIKey = strings.TrimSpace(cfg.BootstrapAPIKey)
		if cfg.BootstrapAPIKey == "" {
			return Config{}, fmt.Errorf("BOOTSTRAP_API_KEY must not be blank")
		}
		if !cfg.BootstrapEnabled {
			return Config{}, fmt.Errorf("BOOTSTRAP_API_KEY is set but BOOTSTRAP_ENABLED=false; set BOOTSTRAP_ENABLED=true to use bootstrap seeding")
		}
		if cfg.BootstrapMinLength <= 0 {
			return Config{}, fmt.Errorf("BOOTSTRAP_API_KEY_MIN_LENGTH must be > 0")
		}
		if err := validateBootstrapAPIKey(cfg.BootstrapAPIKey, cfg.BootstrapMinLength); err != nil {
			return Config{}, err
		}
	} else if cfg.BootstrapEnabled {
		return Config{}, fmt.Errorf("BOOTSTRAP_ENABLED=true requires BOOTSTRAP_API_KEY")
	}

	if cfg.BootstrapMinLength < 24 && cfg.BootstrapAPIKey != "" {
		return Config{}, fmt.Errorf("BOOTSTRAP_API_KEY_MIN_LENGTH must be at least 24")
	}
	if cfg.Provider != "privatemode" && cfg.Provider != "tinfoil" {
		return Config{}, fmt.Errorf("INFERENCE_PROVIDER must be one of: privatemode, tinfoil")
	}
	if cfg.Provider == "privatemode" && strings.TrimSpace(cfg.UpstreamOpenAIAPIKey) == "" {
		return Config{}, fmt.Errorf("PRIVATEMODE_API_KEY is required when INFERENCE_PROVIDER=privatemode")
	}
	if cfg.Provider == "privatemode" && strings.TrimSpace(cfg.UpstreamOpenAIBaseURL) == "" {
		return Config{}, fmt.Errorf("PRIVATEMODE_BASE_URL is required when INFERENCE_PROVIDER=privatemode")
	}
	if cfg.Provider == "tinfoil" {
		if cfg.TinfoilAPIKey == "" {
			return Config{}, fmt.Errorf("TINFOIL_API_KEY is required when INFERENCE_PROVIDER=tinfoil")
		}
		if cfg.TinfoilEnclave == "" || cfg.TinfoilRepo == "" {
			return Config{}, fmt.Errorf("TINFOIL_ENCLAVE and TINFOIL_REPO are required when INFERENCE_PROVIDER=tinfoil")
		}
	}

	return cfg, nil
}

func (c Config) Addr() string {
	return fmt.Sprintf("%s:%d", c.Host, c.Port)
}

func (c Config) MaxUploadBytes() int64 {
	return c.MaxUploadSizeMB * 1024 * 1024
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	i, err := strconv.Atoi(v)
	if err != nil {
		log.Printf("warning: invalid value %q for %s, using default %d", v, key, fallback)
		return fallback
	}
	return i
}

func getEnvDuration(key string, fallback time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		log.Printf("warning: invalid duration %q for %s, using default %v", v, key, fallback)
		return fallback
	}
	return d
}

func validateBootstrapAPIKey(value string, minLength int) error {
	normalized := strings.ToLower(strings.TrimSpace(value))
	switch normalized {
	case "",
		"enc_demo_replace_me",
		"demo",
		"change_me",
		"changeme",
		"bootstrap",
		"default",
		"password",
		"test",
		"abc123",
		"abc123456":
		return fmt.Errorf("BOOTSTRAP_API_KEY uses a forbidden placeholder value")
	}

	if len(value) < minLength {
		return fmt.Errorf("BOOTSTRAP_API_KEY must be at least %d characters", minLength)
	}

	if strings.ContainsAny(value, " \t\r\n") {
		return fmt.Errorf("BOOTSTRAP_API_KEY must not contain whitespace")
	}

	return nil
}

func getEnvBool(key string, fallback bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(v)
	if err != nil {
		return fallback
	}
	return parsed
}
