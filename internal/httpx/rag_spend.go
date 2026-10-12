package httpx

import (
	"context"
	"net/http"

	"enclava-go/internal/auth"
	"enclava-go/internal/rag"
	"enclava-go/internal/store/sqlite"
)

var _ rag.SpendGuard = (*App)(nil)

func (a *App) Before(ctx context.Context, apiKeyID int64) error {
	if apiKeyID <= 0 || a == nil || a.keysRepo == nil {
		return nil
	}
	key, err := a.keysRepo.GetByID(ctx, apiKeyID)
	if err != nil {
		return err
	}
	return a.EnforceBudget(ctx, auth.Principal{
		APIKeyID:          key.ID,
		IsUnlimited:       key.IsUnlimited,
		BudgetLimitTokens: key.BudgetLimitTokens,
		BudgetLimitCents:  key.BudgetLimitCents,
		BudgetPeriod:      key.BudgetPeriod,
	})
}

func (a *App) RecordEmbedding(ctx context.Context, apiKeyID int64, model string, promptTokens, totalTokens int) error {
	if a == nil || a.usageRepo == nil {
		return nil
	}
	return a.usageRepo.Record(ctx, sqlite.UsageRecord{
		ID:           NewID("usage"),
		APIKeyID:     apiKeyID,
		Endpoint:     "/api/v1/embeddings",
		Model:        model,
		PromptTokens: int64(promptTokens),
		TotalTokens:  int64(totalTokens),
		CostCents:    0,
		StatusCode:   http.StatusOK,
	})
}
