package httpx

import (
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"enclava-go/internal/auth"
	"enclava-go/internal/store/sqlite"
)

type adminPricingCreateRequest struct {
	ProviderID               string  `json:"provider_id"`
	ModelID                  string  `json:"model_id"`
	Currency                 string  `json:"currency,omitempty"`
	InputTokenCentsPer1K     float64 `json:"input_token_cents_per_1k,omitempty"`
	OutputTokenCentsPer1K    float64 `json:"output_token_cents_per_1k,omitempty"`
	EmbeddingTokenCentsPer1K float64 `json:"embedding_token_cents_per_1k,omitempty"`
	EffectiveAt              *string `json:"effective_at,omitempty"`
}

func (a *App) handleAdminPricing(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
	setNoStoreHeaders(w)
	if a.pricingRepo == nil {
		WriteOpenAIError(w, http.StatusNotImplemented, "pricing is not configured", "server_error", "not_configured")
		return
	}
	switch r.Method {
	case http.MethodGet:
		limit, offset := paginationFromRequest(r)
		prices, err := a.pricingRepo.List(r.Context(), r.URL.Query().Get("provider_id"), r.URL.Query().Get("model_id"), limit, offset)
		if err != nil {
			log.Printf("failed to list pricing: %v", err)
			WriteOpenAIError(w, http.StatusInternalServerError, "internal server error", "server_error", "pricing_list_failed")
			return
		}
		audit, err := a.pricingRepo.ListPricingAudit(r.Context(), 20, 0)
		if err != nil {
			log.Printf("failed to list pricing audit: %v", err)
			WriteOpenAIError(w, http.StatusInternalServerError, "internal server error", "server_error", "pricing_audit_list_failed")
			return
		}
		WriteJSON(w, http.StatusOK, map[string]any{
			"pricing": prices,
			"audit":   audit,
			"limit":   limit,
			"offset":  offset,
		})
	case http.MethodPost:
		var req adminPricingCreateRequest
		if err := DecodeJSON(r, &req); err != nil {
			WriteOpenAIError(w, http.StatusBadRequest, "invalid JSON payload", "invalid_request_error", "invalid_json")
			return
		}
		pricing, err := pricingFromAdminRequest(req)
		if err != nil {
			WriteOpenAIError(w, http.StatusBadRequest, err.Error(), "invalid_request_error", "pricing_invalid")
			return
		}
		created, err := a.pricingRepo.Create(r.Context(), pricing, sqlite.PricingAuditLog{
			ID:        NewID("priceaudit"),
			Action:    "create",
			ActorType: "api_key",
			ActorID:   strconv.FormatInt(principal.APIKeyID, 10),
		})
		if err != nil {
			log.Printf("failed to create pricing: %v", err)
			WriteOpenAIError(w, http.StatusInternalServerError, "internal server error", "server_error", "pricing_create_failed")
			return
		}
		WriteJSON(w, http.StatusCreated, created)
	default:
		WriteMethodNotAllowed(w)
	}
}

func (a *App) handleAdminBillingAudit(w http.ResponseWriter, r *http.Request, _ auth.Principal) {
	setNoStoreHeaders(w)
	if a.billingAuditRepo == nil {
		WriteOpenAIError(w, http.StatusNotImplemented, "billing audit is not configured", "server_error", "not_configured")
		return
	}
	if r.Method != http.MethodGet {
		WriteMethodNotAllowed(w)
		return
	}
	limit, offset := paginationFromRequest(r)
	var apiKeyID *int64
	if raw := strings.TrimSpace(r.URL.Query().Get("api_key_id")); raw != "" {
		id, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || id <= 0 {
			WriteOpenAIError(w, http.StatusBadRequest, "invalid api_key_id", "invalid_request_error", "invalid_api_key_id")
			return
		}
		apiKeyID = &id
	}
	audit, err := a.billingAuditRepo.List(r.Context(), apiKeyID, r.URL.Query().Get("subject_type"), limit, offset)
	if err != nil {
		log.Printf("failed to list billing audit: %v", err)
		WriteOpenAIError(w, http.StatusInternalServerError, "internal server error", "server_error", "billing_audit_list_failed")
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{
		"audit":  audit,
		"limit":  limit,
		"offset": offset,
	})
}

func pricingFromAdminRequest(req adminPricingCreateRequest) (sqlite.ProviderPricing, error) {
	req.ProviderID = strings.TrimSpace(req.ProviderID)
	req.ModelID = strings.TrimSpace(req.ModelID)
	req.Currency = strings.ToUpper(strings.TrimSpace(req.Currency))
	if req.ProviderID == "" {
		return sqlite.ProviderPricing{}, errInvalidPricing("provider_id is required")
	}
	if req.ModelID == "" {
		return sqlite.ProviderPricing{}, errInvalidPricing("model_id is required")
	}
	if req.Currency == "" {
		req.Currency = "USD"
	}
	if len(req.Currency) != 3 {
		return sqlite.ProviderPricing{}, errInvalidPricing("currency must be a three-letter code")
	}
	if req.InputTokenCentsPer1K < 0 || req.OutputTokenCentsPer1K < 0 || req.EmbeddingTokenCentsPer1K < 0 {
		return sqlite.ProviderPricing{}, errInvalidPricing("pricing values must be zero or greater")
	}
	effectiveAt := time.Now().UTC()
	if req.EffectiveAt != nil && strings.TrimSpace(*req.EffectiveAt) != "" {
		parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(*req.EffectiveAt))
		if err != nil {
			return sqlite.ProviderPricing{}, errInvalidPricing("effective_at must be RFC3339")
		}
		effectiveAt = parsed.UTC()
	}
	return sqlite.ProviderPricing{
		ProviderID:               req.ProviderID,
		ModelID:                  req.ModelID,
		Currency:                 req.Currency,
		InputTokenCentsPer1K:     req.InputTokenCentsPer1K,
		OutputTokenCentsPer1K:    req.OutputTokenCentsPer1K,
		EmbeddingTokenCentsPer1K: req.EmbeddingTokenCentsPer1K,
		EffectiveAt:              effectiveAt,
	}, nil
}

type invalidPricingError string

func (e invalidPricingError) Error() string { return string(e) }

func errInvalidPricing(msg string) error {
	return invalidPricingError(msg)
}
