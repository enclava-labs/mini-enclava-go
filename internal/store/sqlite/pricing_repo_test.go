package sqlite

import (
	"context"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func TestPricingRepoCreateListsPricingAndAudit(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, filepath.Join(t.TempDir(), "pricing.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer func() { _ = db.Close() }()

	repo := NewPricingRepo(db.SQL)
	effectiveAt := time.Date(2026, 6, 22, 12, 0, 0, 0, time.UTC)
	created, err := repo.Create(ctx, ProviderPricing{
		ProviderID:               "tinfoil",
		ModelID:                  "llama3-3-70b",
		Currency:                 "USD",
		InputTokenCentsPer1K:     0.12,
		OutputTokenCentsPer1K:    0.34,
		EmbeddingTokenCentsPer1K: 0.02,
		EffectiveAt:              effectiveAt,
	}, PricingAuditLog{
		ID:        "priceaudit_test",
		Action:    "create",
		ActorType: "api_key",
		ActorID:   "7",
	})
	if err != nil {
		t.Fatalf("create pricing: %v", err)
	}
	if created.ID == 0 || created.ProviderID != "tinfoil" || !created.EffectiveAt.Equal(effectiveAt) {
		t.Fatalf("unexpected created pricing: %#v", created)
	}

	prices, err := repo.List(ctx, "tinfoil", "llama3-3-70b", 10, 0)
	if err != nil {
		t.Fatalf("list pricing: %v", err)
	}
	if len(prices) != 1 || prices[0].ID != created.ID {
		t.Fatalf("unexpected prices: %#v", prices)
	}

	audit, err := repo.ListPricingAudit(ctx, 10, 0)
	if err != nil {
		t.Fatalf("list pricing audit: %v", err)
	}
	if len(audit) != 1 || audit[0].ProviderPricingID == nil || *audit[0].ProviderPricingID != created.ID {
		t.Fatalf("unexpected pricing audit: %#v", audit)
	}
	if audit[0].After["model_id"] != "llama3-3-70b" {
		t.Fatalf("pricing audit missing after snapshot: %#v", audit[0].After)
	}
}

func TestBillingAuditRepoListFilters(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, filepath.Join(t.TempDir(), "billing-audit.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer func() { _ = db.Close() }()

	keyID, err := NewAPIKeysRepo(db.SQL).CreateAPIKey(ctx, "billing-test", "enc_test_key_billing_audit_secret", "sha256", "", []string{"platform.admin"})
	if err != nil {
		t.Fatalf("create api key: %v", err)
	}
	repo := NewBillingAuditRepo(db.SQL)
	amount := int64(123)
	if err := repo.Create(ctx, BillingAuditLog{
		ID:          "billaudit_test",
		APIKeyID:    &keyID,
		SubjectType: "api_key",
		SubjectID:   "key",
		Action:      "budget.updated",
		ActorType:   "api_key",
		ActorID:     "7",
		AmountCents: &amount,
		Metadata: map[string]any{
			"period": "monthly",
		},
	}); err != nil {
		t.Fatalf("create billing audit: %v", err)
	}

	rows, err := repo.List(ctx, &keyID, "api_key", 10, 0)
	if err != nil {
		t.Fatalf("list billing audit: %v", err)
	}
	if len(rows) != 1 || rows[0].AmountCents == nil || *rows[0].AmountCents != amount {
		t.Fatalf("unexpected billing audit rows: %#v", rows)
	}
	if rows[0].Metadata["period"] != "monthly" {
		t.Fatalf("unexpected metadata: %#v", rows[0].Metadata)
	}
}

func TestUsageRepoRecordsPricingSnapshotAndCost(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, filepath.Join(t.TempDir(), "usage-pricing.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer func() { _ = db.Close() }()

	keyID, err := NewAPIKeysRepo(db.SQL).CreateAPIKey(ctx, "usage-pricing-test", "enc_test_key_usage_pricing_secret", "sha256", "", []string{"chat.completions"})
	if err != nil {
		t.Fatalf("create api key: %v", err)
	}
	pricing, err := NewPricingRepo(db.SQL).Create(ctx, ProviderPricing{
		ProviderID:            "tinfoil",
		ModelID:               "priced-model",
		Currency:              "USD",
		InputTokenCentsPer1K:  10,
		OutputTokenCentsPer1K: 20,
		EffectiveAt:           time.Now().Add(-time.Minute).UTC(),
	}, PricingAuditLog{ID: "priceaudit_usage_test"})
	if err != nil {
		t.Fatalf("create pricing: %v", err)
	}

	if err := NewUsageRepo(db.SQL).Record(ctx, UsageRecord{
		ID:               "usage_priced_test",
		APIKeyID:         keyID,
		Endpoint:         "/api/v1/chat/completions",
		Model:            "priced-model",
		ProviderID:       "tinfoil",
		PromptTokens:     1000,
		CompletionTokens: 500,
		StatusCode:       200,
	}); err != nil {
		t.Fatalf("record usage: %v", err)
	}

	var providerID, currency string
	var pricingID, costCents, costMicrocents, totalCostCents, totalCostMicrocents int64
	var inputRate, outputRate float64
	if err := db.SQL.QueryRowContext(ctx, `
		SELECT provider_id, provider_pricing_id, pricing_currency,
		       input_token_cents_per_1k, output_token_cents_per_1k, cost_cents, cost_microcents
		FROM usage_records
		WHERE id = ?
	`, "usage_priced_test").Scan(&providerID, &pricingID, &currency, &inputRate, &outputRate, &costCents, &costMicrocents); err != nil {
		t.Fatalf("query usage snapshot: %v", err)
	}
	if providerID != "tinfoil" || pricingID != pricing.ID || currency != "USD" {
		t.Fatalf("unexpected pricing identity snapshot: provider=%s pricing=%d currency=%s", providerID, pricingID, currency)
	}
	if inputRate != 10 || outputRate != 20 || costCents != 20 || costMicrocents != 20_000_000 {
		t.Fatalf("unexpected pricing rates/cost: input=%f output=%f cost=%d micro=%d", inputRate, outputRate, costCents, costMicrocents)
	}
	if err := db.SQL.QueryRowContext(ctx, `SELECT total_cost_cents, total_cost_microcents FROM api_keys WHERE id = ?`, keyID).Scan(&totalCostCents, &totalCostMicrocents); err != nil {
		t.Fatalf("query api key cost totals: %v", err)
	}
	if totalCostCents != 20 || totalCostMicrocents != 20_000_000 {
		t.Fatalf("unexpected api key total cost: cents=%d micro=%d", totalCostCents, totalCostMicrocents)
	}
}

func TestUsageRepoFractionalEmbeddingCostMicrocents(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, filepath.Join(t.TempDir(), "usage-fractional.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer func() { _ = db.Close() }()

	keyID, err := NewAPIKeysRepo(db.SQL).CreateAPIKey(ctx, "fractional-cost", "enc_test_key_fractional_cost_secret", "sha256", "", []string{"embeddings.create"})
	if err != nil {
		t.Fatalf("create api key: %v", err)
	}
	if _, err := NewPricingRepo(db.SQL).Create(ctx, ProviderPricing{
		ProviderID:               "tinfoil",
		ModelID:                  "embed-fractional",
		Currency:                 "USD",
		EmbeddingTokenCentsPer1K: 0.12,
		EffectiveAt:              time.Now().Add(-time.Minute).UTC(),
	}, PricingAuditLog{ID: "priceaudit_fractional"}); err != nil {
		t.Fatalf("create pricing: %v", err)
	}

	repo := NewUsageRepo(db.SQL)
	if err := repo.Record(ctx, UsageRecord{
		ID:          "usage_fractional_1",
		APIKeyID:    keyID,
		Endpoint:    "/api/v1/embeddings",
		Model:       "embed-fractional",
		ProviderID:  "tinfoil",
		TotalTokens: 1000,
		StatusCode:  200,
	}); err != nil {
		t.Fatalf("record usage: %v", err)
	}

	var costCents, costMicrocents int64
	if err := db.SQL.QueryRowContext(ctx, `SELECT cost_cents, cost_microcents FROM usage_records WHERE id = ?`, "usage_fractional_1").Scan(&costCents, &costMicrocents); err != nil {
		t.Fatalf("query fractional usage: %v", err)
	}
	if costCents != 0 || costMicrocents != 120_000 {
		t.Fatalf("single sub-cent row cost_cents=%d cost_microcents=%d", costCents, costMicrocents)
	}

	for i := 2; i <= 10; i++ {
		if err := repo.Record(ctx, UsageRecord{
			ID:          "usage_fractional_" + strconv.Itoa(i),
			APIKeyID:    keyID,
			Endpoint:    "/api/v1/embeddings",
			Model:       "embed-fractional",
			ProviderID:  "tinfoil",
			TotalTokens: 1000,
			StatusCode:  200,
		}); err != nil {
			t.Fatalf("record usage %d: %v", i, err)
		}
	}

	var sumMicro, totalMicro, totalCents int64
	if err := db.SQL.QueryRowContext(ctx, `SELECT COALESCE(SUM(cost_microcents), 0) FROM usage_records WHERE api_key_id = ?`, keyID).Scan(&sumMicro); err != nil {
		t.Fatalf("sum microcents: %v", err)
	}
	if err := db.SQL.QueryRowContext(ctx, `SELECT total_cost_cents, total_cost_microcents FROM api_keys WHERE id = ?`, keyID).Scan(&totalCents, &totalMicro); err != nil {
		t.Fatalf("query key totals: %v", err)
	}
	if sumMicro != 1_200_000 || totalMicro != 1_200_000 || totalCents != 0 {
		t.Fatalf("ten sub-cent rows sum=%d key micro=%d key cents=%d", sumMicro, totalMicro, totalCents)
	}
	tokens, sinceMicro, err := repo.SumSince(ctx, keyID, time.Now().Add(-time.Hour))
	if err != nil {
		t.Fatalf("sum since: %v", err)
	}
	if tokens != 10_000 || sinceMicro != 1_200_000 {
		t.Fatalf("SumSince tokens=%d microcents=%d", tokens, sinceMicro)
	}
}
