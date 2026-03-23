package httpx

import (
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"enclava-go/internal/auth"
	"enclava-go/internal/store/sqlite"
)

type adminAPIKeyCreateRequest struct {
	Name                    string   `json:"name"`
	Description             string   `json:"description,omitempty"`
	Scopes                  []string `json:"scopes"`
	AllowedModels           []string `json:"allowed_models,omitempty"`
	AllowedEndpoints        []string `json:"allowed_endpoints,omitempty"`
	AllowedExtractTemplates []string `json:"allowed_extract_templates,omitempty"`
	AllowedIPs              []string `json:"allowed_ips,omitempty"`
	RateLimitPerMinute      *int     `json:"rate_limit_per_minute,omitempty"`
	RateLimitPerHour        *int     `json:"rate_limit_per_hour,omitempty"`
	RateLimitPerDay         *int     `json:"rate_limit_per_day,omitempty"`
	IsUnlimited             *bool    `json:"is_unlimited,omitempty"`
	BudgetLimitTokens       *int64   `json:"budget_limit_tokens,omitempty"`
	BudgetLimitCents        *int64   `json:"budget_limit_cents,omitempty"`
	BudgetPeriod            string   `json:"budget_period,omitempty"` // 'total' or 'monthly'
	Tags                    []string `json:"tags,omitempty"`
	ExpiresAt               *string  `json:"expires_at,omitempty"` // RFC3339; null or "" clears expiry
	IsActive                *bool    `json:"is_active,omitempty"`
}

type adminAPIKeyCreateResponse struct {
	APIKey    any    `json:"api_key"`
	SecretKey string `json:"secret_key"`
}

func (a *App) handleAdminAPIKeys(w http.ResponseWriter, r *http.Request, _ auth.Principal) {
	setNoStoreHeaders(w)
	if a.keysRepo == nil {
		WriteOpenAIError(w, http.StatusNotImplemented, "admin api key management not configured", "server_error", "not_configured")
		return
	}
	switch r.Method {
	case http.MethodGet:
		includeDeleted := r.URL.Query().Get("include_deleted") == "true"
		keys, err := a.keysRepo.List(r.Context(), includeDeleted)
		if err != nil {
			log.Printf("failed to list api keys: %v", err)
			WriteOpenAIError(w, http.StatusInternalServerError, "internal server error", "server_error", "api_keys_list_failed")
			return
		}
		for i := range keys {
			keys[i] = redactAdminAPIKey(keys[i])
		}
		WriteJSON(w, http.StatusOK, map[string]any{"api_keys": keys})
	case http.MethodPost:
		var req adminAPIKeyCreateRequest
		if err := DecodeJSON(r, &req); err != nil {
			WriteOpenAIError(w, http.StatusBadRequest, "invalid JSON payload", "invalid_request_error", "invalid_json")
			return
		}
		if err := sanitizeAdminAPIKeyRequest(&req, true); err != nil {
			WriteOpenAIError(w, http.StatusBadRequest, err.Error(), "invalid_request_error", "invalid_request")
			return
		}
		if req.Name == "" {
			WriteOpenAIError(w, http.StatusBadRequest, "name is required", "invalid_request_error", "missing_name")
			return
		}
		plaintext, prefix, hash, usedAlgo, err := a.generateKey(a.apiKeyPrefix, a.apiKeyAlgo, a.apiKeyPepper)
		if err != nil {
			log.Printf("failed to generate api key: %v", err)
			WriteOpenAIError(w, http.StatusInternalServerError, "internal server error", "server_error", "key_generate_failed")
			return
		}

		k := sqliteAPIKeyFromAdminRequest(req, prefix, hash, usedAlgo)
		id, err := a.keysRepo.CreateAPIKey(r.Context(), k.Name, plaintext, a.apiKeyAlgo, a.apiKeyPepper, k.Scopes)
		if err != nil {
			log.Printf("failed to create api key: %v", err)
			WriteOpenAIError(w, http.StatusInternalServerError, "internal server error", "server_error", "api_key_create_failed")
			return
		}

		// Update the rest of the metadata in a second step (CreateAPIKey only seeds basic columns).
		k.ID = id
		if err := a.keysRepo.Update(r.Context(), k); err != nil {
			log.Printf("failed to update api key metadata: %v", err)
			WriteOpenAIError(w, http.StatusInternalServerError, "internal server error", "server_error", "api_key_update_failed")
			return
		}

		// Key was created with is_active=0; activate only after all metadata is set.
		if k.IsActive {
			if err := a.keysRepo.ActivateKey(r.Context(), id); err != nil {
				log.Printf("failed to activate api key: %v", err)
				WriteOpenAIError(w, http.StatusInternalServerError, "internal server error", "server_error", "api_key_activate_failed")
				return
			}
		}

		created, err := a.keysRepo.GetByID(r.Context(), id)
		if err != nil {
			log.Printf("failed to fetch created api key: %v", err)
			WriteOpenAIError(w, http.StatusInternalServerError, "internal server error", "server_error", "api_key_fetch_failed")
			return
		}
		created = redactAdminAPIKey(created)
		WriteJSON(w, http.StatusCreated, adminAPIKeyCreateResponse{
			APIKey:    created,
			SecretKey: plaintext,
		})
	default:
		WriteMethodNotAllowed(w)
	}
}

func (a *App) handleAdminAPIKeyByID(w http.ResponseWriter, r *http.Request, _ auth.Principal) {
	setNoStoreHeaders(w)
	if a.keysRepo == nil {
		WriteOpenAIError(w, http.StatusNotImplemented, "admin api key management not configured", "server_error", "not_configured")
		return
	}
	raw := strings.TrimPrefix(r.URL.Path, "/api/v1/admin/api-keys/")
	if raw == "" {
		WriteOpenAIError(w, http.StatusBadRequest, "api key id is required", "invalid_request_error", "missing_id")
		return
	}

	// Sub-actions
	if strings.HasSuffix(raw, "/regenerate") {
		idStr := strings.TrimSuffix(raw, "/regenerate")
		id, err := strconv.ParseInt(strings.TrimSuffix(idStr, "/"), 10, 64)
		if err != nil || id <= 0 {
			WriteOpenAIError(w, http.StatusBadRequest, "invalid api key id", "invalid_request_error", "invalid_id")
			return
		}
		if r.Method != http.MethodPost {
			WriteMethodNotAllowed(w)
			return
		}
		plaintext, prefix, hash, usedAlgo, err := a.generateKey(a.apiKeyPrefix, a.apiKeyAlgo, a.apiKeyPepper)
		if err != nil {
			log.Printf("failed to generate key for regeneration: %v", err)
			WriteOpenAIError(w, http.StatusInternalServerError, "internal server error", "server_error", "key_generate_failed")
			return
		}
		if err := a.keysRepo.SetSecret(r.Context(), id, prefix, hash, usedAlgo); err != nil {
			log.Printf("failed to regenerate api key secret: %v", err)
			WriteOpenAIError(w, http.StatusInternalServerError, "internal server error", "server_error", "api_key_regenerate_failed")
			return
		}
		key, err := a.keysRepo.GetByID(r.Context(), id)
		if err != nil {
			log.Printf("failed to fetch api key after regeneration: %v", err)
			WriteOpenAIError(w, http.StatusInternalServerError, "internal server error", "server_error", "api_key_fetch_failed")
			return
		}
		key = redactAdminAPIKey(key)
		WriteJSON(w, http.StatusOK, adminAPIKeyCreateResponse{APIKey: key, SecretKey: plaintext})
		return
	}

	if strings.HasSuffix(raw, "/delete") {
		idStr := strings.TrimSuffix(raw, "/delete")
		id, err := strconv.ParseInt(strings.TrimSuffix(idStr, "/"), 10, 64)
		if err != nil || id <= 0 {
			WriteOpenAIError(w, http.StatusBadRequest, "invalid api key id", "invalid_request_error", "invalid_id")
			return
		}
		if r.Method != http.MethodPost {
			WriteMethodNotAllowed(w)
			return
		}
		if err := a.keysRepo.SoftDelete(r.Context(), id); err != nil {
			log.Printf("failed to soft-delete api key: %v", err)
			WriteOpenAIError(w, http.StatusInternalServerError, "internal server error", "server_error", "api_key_delete_failed")
			return
		}
		WriteJSON(w, http.StatusOK, map[string]any{"deleted": true, "id": id})
		return
	}

	if strings.HasSuffix(raw, "/restore") {
		idStr := strings.TrimSuffix(raw, "/restore")
		id, err := strconv.ParseInt(strings.TrimSuffix(idStr, "/"), 10, 64)
		if err != nil || id <= 0 {
			WriteOpenAIError(w, http.StatusBadRequest, "invalid api key id", "invalid_request_error", "invalid_id")
			return
		}
		if r.Method != http.MethodPost {
			WriteMethodNotAllowed(w)
			return
		}
		if err := a.keysRepo.Restore(r.Context(), id); err != nil {
			log.Printf("failed to restore api key: %v", err)
			WriteOpenAIError(w, http.StatusInternalServerError, "internal server error", "server_error", "api_key_restore_failed")
			return
		}
		WriteJSON(w, http.StatusOK, map[string]any{"restored": true, "id": id})
		return
	}

	id, err := strconv.ParseInt(strings.TrimSuffix(raw, "/"), 10, 64)
	if err != nil || id <= 0 {
		WriteOpenAIError(w, http.StatusBadRequest, "invalid api key id", "invalid_request_error", "invalid_id")
		return
	}

	switch r.Method {
	case http.MethodGet:
		key, err := a.keysRepo.GetByID(r.Context(), id)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				WriteOpenAIError(w, http.StatusNotFound, "api key not found", "invalid_request_error", "not_found")
				return
			}
			log.Printf("failed to fetch api key by id: %v", err)
			WriteOpenAIError(w, http.StatusInternalServerError, "internal server error", "server_error", "api_key_fetch_failed")
			return
		}
		WriteJSON(w, http.StatusOK, redactAdminAPIKey(key))
	case http.MethodPut:
		key, err := a.keysRepo.GetByID(r.Context(), id)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				WriteOpenAIError(w, http.StatusNotFound, "api key not found", "invalid_request_error", "not_found")
				return
			}
			log.Printf("failed to fetch api key for update: %v", err)
			WriteOpenAIError(w, http.StatusInternalServerError, "internal server error", "server_error", "api_key_fetch_failed")
			return
		}
		var req adminAPIKeyCreateRequest
		if err := DecodeJSON(r, &req); err != nil {
			WriteOpenAIError(w, http.StatusBadRequest, "invalid JSON payload", "invalid_request_error", "invalid_json")
			return
		}
		if err := sanitizeAdminAPIKeyRequest(&req, false); err != nil {
			WriteOpenAIError(w, http.StatusBadRequest, err.Error(), "invalid_request_error", "invalid_request")
			return
		}
		applyAdminPatch(&key, req)
		if err := a.keysRepo.Update(r.Context(), key); err != nil {
			log.Printf("failed to update api key: %v", err)
			WriteOpenAIError(w, http.StatusInternalServerError, "internal server error", "server_error", "api_key_update_failed")
			return
		}
		updated, err := a.keysRepo.GetByID(r.Context(), id)
		if err != nil {
			log.Printf("failed to fetch api key after update: %v", err)
			WriteOpenAIError(w, http.StatusInternalServerError, "internal server error", "server_error", "api_key_fetch_failed")
			return
		}
		WriteJSON(w, http.StatusOK, redactAdminAPIKey(updated))
	default:
		WriteMethodNotAllowed(w)
	}
}

func (a *App) handleAdminUsageSummary(w http.ResponseWriter, r *http.Request, _ auth.Principal) {
	setNoStoreHeaders(w)
	if a.keysRepo == nil || a.usageRepo == nil {
		WriteOpenAIError(w, http.StatusNotImplemented, "usage tracking not configured", "server_error", "not_configured")
		return
	}
	if r.Method != http.MethodGet {
		WriteMethodNotAllowed(w)
		return
	}
	since := time.Now().UTC().AddDate(0, 0, -30)
	usage, err := a.usageRepo.SumAll(r.Context(), since)
	if err != nil {
		log.Printf("failed to fetch usage summary: %v", err)
		WriteOpenAIError(w, http.StatusInternalServerError, "internal server error", "server_error", "usage_summary_failed")
		return
	}
	var totalTokens int64
	for _, u := range usage {
		totalTokens += u.TotalTokens
	}
	WriteJSON(w, http.StatusOK, map[string]any{
		"usage":        usage,
		"total_tokens": totalTokens,
		"period_days":  30,
	})
}

func setNoStoreHeaders(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store, no-cache, max-age=0, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
}

func (a *App) generateKey(prefix, algo, pepper string) (plaintext string, storagePrefix string, hash string, usedAlgo string, err error) {
	// 32 bytes -> 43 chars base64url without padding
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", "", "", "", err
	}
	secret := base64.RawURLEncoding.EncodeToString(buf)
	plaintext = prefix + secret
	storagePrefix, hash, usedAlgo, err = auth.HashForStorage(plaintext, algo, pepper)
	return plaintext, storagePrefix, hash, usedAlgo, err
}

func sanitizeAdminAPIKeyRequest(req *adminAPIKeyCreateRequest, requireName bool) error {
	req.Name = strings.TrimSpace(req.Name)
	req.Description = strings.TrimSpace(req.Description)
	req.BudgetPeriod = strings.TrimSpace(req.BudgetPeriod)
	if req.ExpiresAt != nil {
		v := strings.TrimSpace(*req.ExpiresAt)
		req.ExpiresAt = &v
	}

	if requireName && req.Name == "" {
		return errors.New("name is required")
	}
	if req.RateLimitPerMinute != nil && *req.RateLimitPerMinute <= 0 {
		return errors.New("rate_limit_per_minute must be a positive integer")
	}
	if req.RateLimitPerHour != nil && *req.RateLimitPerHour <= 0 {
		return errors.New("rate_limit_per_hour must be a positive integer")
	}
	if req.RateLimitPerDay != nil && *req.RateLimitPerDay <= 0 {
		return errors.New("rate_limit_per_day must be a positive integer")
	}
	if req.BudgetLimitTokens != nil && *req.BudgetLimitTokens < 0 {
		return errors.New("budget_limit_tokens must be zero or greater")
	}
	if req.BudgetLimitCents != nil && *req.BudgetLimitCents < 0 {
		return errors.New("budget_limit_cents must be zero or greater")
	}
	if req.BudgetPeriod != "" && req.BudgetPeriod != "total" && req.BudgetPeriod != "monthly" {
		return errors.New("budget_period must be empty, 'total', or 'monthly'")
	}
	if req.ExpiresAt != nil && *req.ExpiresAt != "" {
		if _, parseErr := time.Parse(time.RFC3339, *req.ExpiresAt); parseErr != nil {
			return errors.New("expires_at must be RFC3339")
		}
	}
	return nil
}

func redactAdminAPIKey(k sqlite.APIKey) sqlite.APIKey {
	k.KeyHash = ""
	k.KeyHashAlgo = ""
	return k
}

func sqliteAPIKeyFromAdminRequest(req adminAPIKeyCreateRequest, prefix, hash, usedAlgo string) sqlite.APIKey {
	k := sqlite.APIKey{
		Name:                    strings.TrimSpace(req.Name),
		Description:             strings.TrimSpace(req.Description),
		KeyPrefix:               prefix,
		KeyHash:                 hash,
		KeyHashAlgo:             usedAlgo,
		IsActive:                true,
		Scopes:                  req.Scopes,
		AllowedModels:           req.AllowedModels,
		AllowedEndpoints:        req.AllowedEndpoints,
		AllowedExtractTemplates: req.AllowedExtractTemplates,
		AllowedIPs:              req.AllowedIPs,
		RateLimitPerMinute:      req.RateLimitPerMinute,
		RateLimitPerHour:        req.RateLimitPerHour,
		RateLimitPerDay:         req.RateLimitPerDay,
		IsUnlimited:             false,
		BudgetLimitTokens:       req.BudgetLimitTokens,
		BudgetLimitCents:        req.BudgetLimitCents,
		BudgetPeriod:            strings.TrimSpace(req.BudgetPeriod),
		Tags:                    req.Tags,
	}
	if req.IsUnlimited != nil {
		k.IsUnlimited = *req.IsUnlimited
	}
	if req.IsActive != nil {
		k.IsActive = *req.IsActive
	}
	if req.ExpiresAt != nil && *req.ExpiresAt != "" {
		if t, parseErr := time.Parse(time.RFC3339, *req.ExpiresAt); parseErr == nil {
			tt := t.UTC()
			k.ExpiresAt = &tt
		}
	}
	return k
}

func applyAdminPatch(k *sqlite.APIKey, req adminAPIKeyCreateRequest) {
	if strings.TrimSpace(req.Name) != "" {
		k.Name = strings.TrimSpace(req.Name)
	}
	if req.Description != "" {
		k.Description = strings.TrimSpace(req.Description)
	}
	if req.Scopes != nil {
		k.Scopes = req.Scopes
	}
	if req.AllowedModels != nil {
		k.AllowedModels = req.AllowedModels
	}
	if req.AllowedEndpoints != nil {
		k.AllowedEndpoints = req.AllowedEndpoints
	}
	if req.AllowedExtractTemplates != nil {
		k.AllowedExtractTemplates = req.AllowedExtractTemplates
	}
	if req.AllowedIPs != nil {
		k.AllowedIPs = req.AllowedIPs
	}
	if req.RateLimitPerMinute != nil {
		k.RateLimitPerMinute = req.RateLimitPerMinute
	}
	if req.RateLimitPerHour != nil {
		k.RateLimitPerHour = req.RateLimitPerHour
	}
	if req.RateLimitPerDay != nil {
		k.RateLimitPerDay = req.RateLimitPerDay
	}
	if req.IsUnlimited != nil {
		k.IsUnlimited = *req.IsUnlimited
	}
	if req.BudgetLimitTokens != nil {
		k.BudgetLimitTokens = req.BudgetLimitTokens
	}
	if req.BudgetLimitCents != nil {
		k.BudgetLimitCents = req.BudgetLimitCents
	}
	if strings.TrimSpace(req.BudgetPeriod) != "" {
		k.BudgetPeriod = strings.TrimSpace(req.BudgetPeriod)
	}
	if req.Tags != nil {
		k.Tags = req.Tags
	}
	if req.IsActive != nil {
		k.IsActive = *req.IsActive
	}
	if req.ExpiresAt != nil {
		if *req.ExpiresAt == "" {
			k.ExpiresAt = nil
		} else if t, parseErr := time.Parse(time.RFC3339, *req.ExpiresAt); parseErr == nil {
			tt := t.UTC()
			k.ExpiresAt = &tt
		}
	}
}
