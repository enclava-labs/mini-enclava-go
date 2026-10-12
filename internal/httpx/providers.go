package httpx

import (
	"database/sql"
	"errors"
	"log"
	"net/http"
	"strings"

	"enclava-go/internal/auth"
	"enclava-go/internal/store/sqlite"
)

type providerConfigRequest struct {
	ID            string         `json:"id,omitempty"`
	Type          string         `json:"type"`
	Name          string         `json:"name"`
	BaseURL       string         `json:"base_url,omitempty"`
	CredentialRef string         `json:"credential_ref,omitempty"`
	Config        map[string]any `json:"config,omitempty"`
	IsEnabled     *bool          `json:"is_enabled,omitempty"`
	IsDefault     *bool          `json:"is_default,omitempty"`
}

func (a *App) handleProviders(w http.ResponseWriter, r *http.Request, _ auth.Principal) {
	setNoStoreHeaders(w)
	if a.providersRepo == nil {
		WriteOpenAIError(w, http.StatusNotImplemented, "providers are not configured", "server_error", "not_configured")
		return
	}
	switch r.Method {
	case http.MethodGet:
		includeDisabled := strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("include_disabled")), "true")
		providers, err := a.providersRepo.List(r.Context(), r.URL.Query().Get("type"), includeDisabled)
		if err != nil {
			log.Printf("failed to list providers: %v", err)
			WriteOpenAIError(w, http.StatusInternalServerError, "internal server error", "server_error", "providers_list_failed")
			return
		}
		WriteJSON(w, http.StatusOK, map[string]any{
			"providers": providers,
			"runtime": map[string]any{
				"provider_id": a.providerID,
				"ready":       a.ProviderReady(),
				"snapshot":    a.ProviderSnapshot(),
			},
		})
	case http.MethodPost:
		var req providerConfigRequest
		if err := DecodeJSON(r, &req); err != nil {
			WriteOpenAIError(w, http.StatusBadRequest, "invalid JSON payload", "invalid_request_error", "invalid_json")
			return
		}
		provider, err := providerConfigFromCreateRequest(req)
		if err != nil {
			WriteOpenAIError(w, http.StatusBadRequest, err.Error(), "invalid_request_error", "provider_invalid")
			return
		}
		if err := a.providersRepo.Create(r.Context(), provider); err != nil {
			log.Printf("failed to create provider config: %v", err)
			WriteOpenAIError(w, http.StatusInternalServerError, "internal server error", "server_error", "provider_create_failed")
			return
		}
		created, err := a.providersRepo.Get(r.Context(), provider.ID)
		if err != nil {
			log.Printf("failed to fetch created provider config: %v", err)
			WriteOpenAIError(w, http.StatusInternalServerError, "internal server error", "server_error", "provider_fetch_failed")
			return
		}
		WriteJSON(w, http.StatusCreated, created)
	default:
		WriteMethodNotAllowed(w)
	}
}

func (a *App) handleProviderByID(w http.ResponseWriter, r *http.Request, _ auth.Principal) {
	setNoStoreHeaders(w)
	if a.providersRepo == nil {
		WriteOpenAIError(w, http.StatusNotImplemented, "providers are not configured", "server_error", "not_configured")
		return
	}
	id := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/providers/"), "/")
	if id == "" {
		WriteOpenAIError(w, http.StatusBadRequest, "provider id is required", "invalid_request_error", "missing_provider")
		return
	}
	switch r.Method {
	case http.MethodGet:
		provider, err := a.providersRepo.Get(r.Context(), id)
		if err != nil {
			writeProviderNotFoundOrError(w, err, "provider_fetch_failed")
			return
		}
		WriteJSON(w, http.StatusOK, provider)
	case http.MethodPut:
		existing, err := a.providersRepo.Get(r.Context(), id)
		if err != nil {
			writeProviderNotFoundOrError(w, err, "provider_fetch_failed")
			return
		}
		var req providerConfigRequest
		if err := DecodeJSON(r, &req); err != nil {
			WriteOpenAIError(w, http.StatusBadRequest, "invalid JSON payload", "invalid_request_error", "invalid_json")
			return
		}
		updated, err := providerConfigFromUpdateRequest(existing, req)
		if err != nil {
			WriteOpenAIError(w, http.StatusBadRequest, err.Error(), "invalid_request_error", "provider_invalid")
			return
		}
		if err := a.providersRepo.Update(r.Context(), updated); err != nil {
			writeProviderNotFoundOrError(w, err, "provider_update_failed")
			return
		}
		provider, err := a.providersRepo.Get(r.Context(), id)
		if err != nil {
			writeProviderNotFoundOrError(w, err, "provider_fetch_failed")
			return
		}
		WriteJSON(w, http.StatusOK, provider)
	case http.MethodDelete:
		if err := a.providersRepo.Delete(r.Context(), id); err != nil {
			writeProviderNotFoundOrError(w, err, "provider_delete_failed")
			return
		}
		WriteJSON(w, http.StatusOK, map[string]any{"deleted": true, "id": id})
	default:
		WriteMethodNotAllowed(w)
	}
}

func providerConfigFromCreateRequest(req providerConfigRequest) (sqlite.ProviderConfig, error) {
	provider := sqlite.ProviderConfig{
		ID:            strings.TrimSpace(req.ID),
		Type:          strings.TrimSpace(req.Type),
		Name:          strings.TrimSpace(req.Name),
		BaseURL:       strings.TrimSpace(req.BaseURL),
		CredentialRef: strings.TrimSpace(req.CredentialRef),
		Config:        nonNilProviderConfig(req.Config),
		IsEnabled:     true,
		IsDefault:     false,
	}
	if provider.ID == "" {
		provider.ID = NewID("provider")
	}
	if req.IsEnabled != nil {
		provider.IsEnabled = *req.IsEnabled
	}
	if req.IsDefault != nil {
		provider.IsDefault = *req.IsDefault
	}
	return validateProviderConfig(provider)
}

func providerConfigFromUpdateRequest(existing sqlite.ProviderConfig, req providerConfigRequest) (sqlite.ProviderConfig, error) {
	provider := existing
	if strings.TrimSpace(req.Type) != "" {
		provider.Type = strings.TrimSpace(req.Type)
	}
	if strings.TrimSpace(req.Name) != "" {
		provider.Name = strings.TrimSpace(req.Name)
	}
	provider.BaseURL = strings.TrimSpace(req.BaseURL)
	provider.CredentialRef = strings.TrimSpace(req.CredentialRef)
	if req.Config != nil {
		provider.Config = nonNilProviderConfig(req.Config)
	}
	if req.IsEnabled != nil {
		provider.IsEnabled = *req.IsEnabled
	}
	if req.IsDefault != nil {
		provider.IsDefault = *req.IsDefault
	}
	return validateProviderConfig(provider)
}

func validateProviderConfig(provider sqlite.ProviderConfig) (sqlite.ProviderConfig, error) {
	if strings.TrimSpace(provider.ID) == "" {
		return sqlite.ProviderConfig{}, errInvalidProvider("id is required")
	}
	if !validProviderID(provider.ID) {
		return sqlite.ProviderConfig{}, errInvalidProvider("id may contain only letters, numbers, hyphen, underscore, and dot")
	}
	if strings.TrimSpace(provider.Type) == "" {
		return sqlite.ProviderConfig{}, errInvalidProvider("type is required")
	}
	if !knownProviderType(provider.Type) {
		return sqlite.ProviderConfig{}, errInvalidProvider("type must be one of: tinfoil, privatemode, openai_compatible, redpill")
	}
	if strings.TrimSpace(provider.Name) == "" {
		return sqlite.ProviderConfig{}, errInvalidProvider("name is required")
	}
	if providerConfigHasSecretKeys(provider.Config) {
		return sqlite.ProviderConfig{}, errInvalidProvider("config must not contain secrets; use credential_ref")
	}
	return provider, nil
}

func knownProviderType(providerType string) bool {
	switch strings.ToLower(strings.TrimSpace(providerType)) {
	case "tinfoil", "privatemode", "openai_compatible", "redpill":
		return true
	default:
		return false
	}
}

func validProviderID(id string) bool {
	if id == "" || len(id) > 128 {
		return false
	}
	for _, r := range id {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' {
			continue
		}
		return false
	}
	return true
}

func providerConfigHasSecretKeys(config map[string]any) bool {
	for key, value := range config {
		normalized := strings.ToLower(strings.TrimSpace(key))
		if strings.Contains(normalized, "secret") ||
			strings.Contains(normalized, "api_key") ||
			strings.Contains(normalized, "apikey") ||
			strings.Contains(normalized, "password") ||
			strings.Contains(normalized, "token") {
			return true
		}
		if nested, ok := value.(map[string]any); ok && providerConfigHasSecretKeys(nested) {
			return true
		}
	}
	return false
}

func nonNilProviderConfig(config map[string]any) map[string]any {
	if config == nil {
		return map[string]any{}
	}
	return config
}

type invalidProviderError string

func (e invalidProviderError) Error() string { return string(e) }

func errInvalidProvider(msg string) error {
	return invalidProviderError(msg)
}

func writeProviderNotFoundOrError(w http.ResponseWriter, err error, code string) {
	if errors.Is(err, sql.ErrNoRows) {
		WriteOpenAIError(w, http.StatusNotFound, "provider not found", "invalid_request_error", "not_found")
		return
	}
	log.Printf("provider error: %v", err)
	WriteOpenAIError(w, http.StatusInternalServerError, "internal server error", "server_error", code)
}
