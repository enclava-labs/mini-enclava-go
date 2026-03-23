package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

type UsageRecord struct {
	ID               string    `json:"id"`
	APIKeyID         int64     `json:"api_key_id"`
	Endpoint         string    `json:"endpoint"`
	Model            string    `json:"model,omitempty"`
	PromptTokens     int64     `json:"prompt_tokens"`
	CompletionTokens int64     `json:"completion_tokens"`
	TotalTokens      int64     `json:"total_tokens"`
	CostCents        int64     `json:"cost_cents"`
	StatusCode       int       `json:"status_code"`
	CreatedAt        time.Time `json:"created_at"`
}

type UsageRepo struct {
	db *sql.DB
}

func NewUsageRepo(db *sql.DB) *UsageRepo {
	return &UsageRepo{db: db}
}

func (r *UsageRepo) Record(ctx context.Context, rec UsageRecord) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin usage tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO usage_records(
			id, api_key_id, endpoint, model,
			prompt_tokens, completion_tokens, total_tokens, cost_cents,
			status_code, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, datetime('now'))
	`, rec.ID, rec.APIKeyID, rec.Endpoint, nullableString(rec.Model),
		rec.PromptTokens, rec.CompletionTokens, rec.TotalTokens, rec.CostCents,
		rec.StatusCode,
	); err != nil {
		return fmt.Errorf("insert usage record: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE api_keys
		SET total_tokens = total_tokens + ?,
		    total_cost_cents = total_cost_cents + ?,
		    updated_at = datetime('now')
		WHERE id = ?
		  AND is_deleted = 0
	`, rec.TotalTokens, rec.CostCents, rec.APIKeyID); err != nil {
		return fmt.Errorf("update api key totals: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit usage tx: %w", err)
	}
	return nil
}

type UsageSummary struct {
	APIKeyID         int64 `json:"api_key_id"`
	TotalTokens      int64 `json:"total_tokens"`
	TotalCostCents   int64 `json:"total_cost_cents"`
	PromptTokens     int64 `json:"prompt_tokens"`
	CompletionTokens int64 `json:"completion_tokens"`
	RequestCount     int64 `json:"request_count"`
}

func (r *UsageRepo) SumAll(ctx context.Context, since time.Time) ([]UsageSummary, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT api_key_id,
		       COALESCE(SUM(total_tokens), 0),
		       COALESCE(SUM(cost_cents), 0),
		       COALESCE(SUM(prompt_tokens), 0),
		       COALESCE(SUM(completion_tokens), 0),
		       COUNT(*)
		FROM usage_records
		WHERE created_at >= ?
		GROUP BY api_key_id
	`, since.UTC().Format("2006-01-02 15:04:05"))
	if err != nil {
		return nil, fmt.Errorf("sum all usage: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	var out []UsageSummary
	for rows.Next() {
		var s UsageSummary
		if err := rows.Scan(&s.APIKeyID, &s.TotalTokens, &s.TotalCostCents, &s.PromptTokens, &s.CompletionTokens, &s.RequestCount); err != nil {
			return nil, fmt.Errorf("scan usage summary: %w", err)
		}
		out = append(out, s)
	}
	return out, nil
}

type UsageByEndpoint struct {
	Endpoint     string
	RequestCount int64
	TotalTokens  int64
}

type UsageByModel struct {
	Model          string
	RequestCount   int64
	TotalTokens    int64
	TotalCostCents int64
}

func (r *UsageRepo) SumByEndpoint(ctx context.Context, since time.Time) ([]UsageByEndpoint, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT endpoint, COUNT(*) as request_count, COALESCE(SUM(total_tokens),0) as total_tokens
		FROM usage_records
		WHERE created_at >= ?
		GROUP BY endpoint
		ORDER BY request_count DESC
	`, since.UTC().Format("2006-01-02 15:04:05"))
	if err != nil {
		return nil, fmt.Errorf("sum by endpoint: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []UsageByEndpoint
	for rows.Next() {
		var e UsageByEndpoint
		if err := rows.Scan(&e.Endpoint, &e.RequestCount, &e.TotalTokens); err != nil {
			return nil, fmt.Errorf("scan usage by endpoint: %w", err)
		}
		out = append(out, e)
	}
	return out, nil
}

func (r *UsageRepo) SumByModel(ctx context.Context, since time.Time) ([]UsageByModel, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT model, COUNT(*) as request_count, COALESCE(SUM(total_tokens),0) as total_tokens, COALESCE(SUM(cost_cents),0) as total_cost_cents
		FROM usage_records
		WHERE created_at >= ?
		GROUP BY model
		ORDER BY request_count DESC
	`, since.UTC().Format("2006-01-02 15:04:05"))
	if err != nil {
		return nil, fmt.Errorf("sum by model: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []UsageByModel
	for rows.Next() {
		var m UsageByModel
		if err := rows.Scan(&m.Model, &m.RequestCount, &m.TotalTokens, &m.TotalCostCents); err != nil {
			return nil, fmt.Errorf("scan usage by model: %w", err)
		}
		out = append(out, m)
	}
	return out, nil
}

func (r *UsageRepo) SumTotal(ctx context.Context, since time.Time) (totalRequests, totalTokens, totalCostCents int64, err error) {
	err = r.db.QueryRowContext(ctx, `
		SELECT COUNT(*) as total_requests, COALESCE(SUM(total_tokens),0) as total_tokens, COALESCE(SUM(cost_cents),0) as total_cost_cents
		FROM usage_records
		WHERE created_at >= ?
	`, since.UTC().Format("2006-01-02 15:04:05")).Scan(&totalRequests, &totalTokens, &totalCostCents)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("sum total usage: %w", err)
	}
	return totalRequests, totalTokens, totalCostCents, nil
}

func (r *UsageRepo) SumSince(ctx context.Context, apiKeyID int64, since time.Time) (totalTokens int64, totalCostCents int64, err error) {
	var tokens sql.NullInt64
	var cents sql.NullInt64
	err = r.db.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(total_tokens), 0), COALESCE(SUM(cost_cents), 0)
		FROM usage_records
		WHERE api_key_id = ?
		  AND created_at >= ?
	`, apiKeyID, since.UTC().Format("2006-01-02 15:04:05")).Scan(&tokens, &cents)
	if err != nil {
		return 0, 0, fmt.Errorf("sum usage since: %w", err)
	}
	return tokens.Int64, cents.Int64, nil
}
