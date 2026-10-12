package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

type PricingRepo struct {
	db *sql.DB
}

func NewPricingRepo(db *sql.DB) *PricingRepo {
	return &PricingRepo{db: db}
}

func (r *PricingRepo) Create(ctx context.Context, p ProviderPricing, audit PricingAuditLog) (ProviderPricing, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return ProviderPricing{}, fmt.Errorf("begin pricing tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	res, err := tx.ExecContext(ctx, `
		INSERT INTO provider_pricing(
			provider_id, model_id, currency,
			input_token_cents_per_1k, output_token_cents_per_1k, embedding_token_cents_per_1k,
			effective_at
		) VALUES (?, ?, ?, ?, ?, ?, ?)
	`, p.ProviderID, p.ModelID, p.Currency,
		p.InputTokenCentsPer1K, p.OutputTokenCentsPer1K, p.EmbeddingTokenCentsPer1K,
		formatDBTime(p.EffectiveAt))
	if err != nil {
		return ProviderPricing{}, fmt.Errorf("insert provider pricing: %w", err)
	}
	id, _ := res.LastInsertId()
	p.ID = id
	created, err := getProviderPricingTx(ctx, tx, id)
	if err != nil {
		return ProviderPricing{}, err
	}

	if audit.ID != "" {
		afterJSON, err := marshalJSON(created)
		if err != nil {
			return ProviderPricing{}, fmt.Errorf("marshal pricing audit after: %w", err)
		}
		if audit.Action == "" {
			audit.Action = "create"
		}
		if audit.ActorType == "" {
			audit.ActorType = "api_key"
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO pricing_audit_log(
				id, provider_pricing_id, action, actor_type, actor_id, before_json, after_json
			) VALUES (?, ?, ?, ?, ?, NULL, ?)
		`, audit.ID, id, audit.Action, audit.ActorType, nullableString(audit.ActorID), afterJSON); err != nil {
			return ProviderPricing{}, fmt.Errorf("insert pricing audit log: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return ProviderPricing{}, fmt.Errorf("commit pricing tx: %w", err)
	}
	return created, nil
}

func (r *PricingRepo) List(ctx context.Context, providerID, modelID string, limit, offset int) ([]ProviderPricing, error) {
	limit, offset = normalizeLimitOffset(limit, offset)
	where := []string{"1=1"}
	args := []any{}
	if strings.TrimSpace(providerID) != "" {
		where = append(where, "provider_id = ?")
		args = append(args, strings.TrimSpace(providerID))
	}
	if strings.TrimSpace(modelID) != "" {
		where = append(where, "model_id = ?")
		args = append(args, strings.TrimSpace(modelID))
	}
	args = append(args, limit, offset)
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, provider_id, model_id, currency,
		       input_token_cents_per_1k, output_token_cents_per_1k, embedding_token_cents_per_1k,
		       effective_at, created_at
		FROM provider_pricing
		WHERE `+strings.Join(where, " AND ")+`
		ORDER BY provider_id ASC, model_id ASC, effective_at DESC
		LIMIT ? OFFSET ?
	`, args...)
	if err != nil {
		return nil, fmt.Errorf("list provider pricing: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := []ProviderPricing{}
	for rows.Next() {
		p, err := scanProviderPricing(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate provider pricing: %w", err)
	}
	return out, nil
}

func (r *PricingRepo) ListPricingAudit(ctx context.Context, limit, offset int) ([]PricingAuditLog, error) {
	limit, offset = normalizeLimitOffset(limit, offset)
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, provider_pricing_id, action, actor_type, actor_id,
		       before_json, after_json, created_at
		FROM pricing_audit_log
		ORDER BY created_at DESC
		LIMIT ? OFFSET ?
	`, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list pricing audit: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := []PricingAuditLog{}
	for rows.Next() {
		a, err := scanPricingAudit(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate pricing audit: %w", err)
	}
	return out, nil
}

type BillingAuditRepo struct {
	db *sql.DB
}

func NewBillingAuditRepo(db *sql.DB) *BillingAuditRepo {
	return &BillingAuditRepo{db: db}
}

func (r *BillingAuditRepo) Create(ctx context.Context, audit BillingAuditLog) error {
	metadataJSON, err := marshalJSON(nonNilSettings(audit.Metadata))
	if err != nil {
		return fmt.Errorf("marshal billing audit metadata: %w", err)
	}
	_, err = r.db.ExecContext(ctx, `
		INSERT INTO billing_audit_log(
			id, api_key_id, subject_type, subject_id, action, actor_type,
			actor_id, amount_cents, metadata_json
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, audit.ID, nullableInt64(audit.APIKeyID), audit.SubjectType, nullableString(audit.SubjectID),
		audit.Action, audit.ActorType, nullableString(audit.ActorID), nullableInt64(audit.AmountCents), metadataJSON)
	if err != nil {
		return fmt.Errorf("insert billing audit: %w", err)
	}
	return nil
}

func (r *BillingAuditRepo) List(ctx context.Context, apiKeyID *int64, subjectType string, limit, offset int) ([]BillingAuditLog, error) {
	limit, offset = normalizeLimitOffset(limit, offset)
	where := []string{"1=1"}
	args := []any{}
	if apiKeyID != nil {
		where = append(where, "api_key_id = ?")
		args = append(args, *apiKeyID)
	}
	if strings.TrimSpace(subjectType) != "" {
		where = append(where, "subject_type = ?")
		args = append(args, strings.TrimSpace(subjectType))
	}
	args = append(args, limit, offset)
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, api_key_id, subject_type, subject_id, action, actor_type,
		       actor_id, amount_cents, metadata_json, created_at
		FROM billing_audit_log
		WHERE `+strings.Join(where, " AND ")+`
		ORDER BY created_at DESC
		LIMIT ? OFFSET ?
	`, args...)
	if err != nil {
		return nil, fmt.Errorf("list billing audit: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := []BillingAuditLog{}
	for rows.Next() {
		a, err := scanBillingAudit(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate billing audit: %w", err)
	}
	return out, nil
}

type pricingScanner interface {
	Scan(dest ...any) error
}

func getProviderPricingTx(ctx context.Context, tx *sql.Tx, id int64) (ProviderPricing, error) {
	row := tx.QueryRowContext(ctx, `
		SELECT id, provider_id, model_id, currency,
		       input_token_cents_per_1k, output_token_cents_per_1k, embedding_token_cents_per_1k,
		       effective_at, created_at
		FROM provider_pricing
		WHERE id = ?
	`, id)
	return scanProviderPricing(row)
}

func scanProviderPricing(s pricingScanner) (ProviderPricing, error) {
	var p ProviderPricing
	var effectiveAtRaw, createdAtRaw string
	if err := s.Scan(
		&p.ID,
		&p.ProviderID,
		&p.ModelID,
		&p.Currency,
		&p.InputTokenCentsPer1K,
		&p.OutputTokenCentsPer1K,
		&p.EmbeddingTokenCentsPer1K,
		&effectiveAtRaw,
		&createdAtRaw,
	); err != nil {
		return ProviderPricing{}, err
	}
	effectiveAt, err := parseDBTime(effectiveAtRaw)
	if err != nil {
		return ProviderPricing{}, fmt.Errorf("parse pricing effective_at: %w", err)
	}
	createdAt, err := parseDBTime(createdAtRaw)
	if err != nil {
		return ProviderPricing{}, fmt.Errorf("parse pricing created_at: %w", err)
	}
	p.EffectiveAt = effectiveAt
	p.CreatedAt = createdAt
	return p, nil
}

func scanPricingAudit(s pricingScanner) (PricingAuditLog, error) {
	var a PricingAuditLog
	var pricingID sql.NullInt64
	var actorID, beforeRaw, afterRaw sql.NullString
	var createdAtRaw string
	if err := s.Scan(
		&a.ID,
		&pricingID,
		&a.Action,
		&a.ActorType,
		&actorID,
		&beforeRaw,
		&afterRaw,
		&createdAtRaw,
	); err != nil {
		return PricingAuditLog{}, err
	}
	if pricingID.Valid {
		v := pricingID.Int64
		a.ProviderPricingID = &v
	}
	if actorID.Valid {
		a.ActorID = actorID.String
	}
	if beforeRaw.Valid {
		a.Before = unmarshalMap(beforeRaw.String)
	}
	if afterRaw.Valid {
		a.After = unmarshalMap(afterRaw.String)
	}
	createdAt, err := parseDBTime(createdAtRaw)
	if err != nil {
		return PricingAuditLog{}, fmt.Errorf("parse pricing audit created_at: %w", err)
	}
	a.CreatedAt = createdAt
	return a, nil
}

func scanBillingAudit(s pricingScanner) (BillingAuditLog, error) {
	var a BillingAuditLog
	var apiKeyID, amountCents sql.NullInt64
	var subjectID, actorID sql.NullString
	var metadataRaw string
	var createdAtRaw string
	if err := s.Scan(
		&a.ID,
		&apiKeyID,
		&a.SubjectType,
		&subjectID,
		&a.Action,
		&a.ActorType,
		&actorID,
		&amountCents,
		&metadataRaw,
		&createdAtRaw,
	); err != nil {
		return BillingAuditLog{}, err
	}
	if apiKeyID.Valid {
		v := apiKeyID.Int64
		a.APIKeyID = &v
	}
	if subjectID.Valid {
		a.SubjectID = subjectID.String
	}
	if actorID.Valid {
		a.ActorID = actorID.String
	}
	if amountCents.Valid {
		v := amountCents.Int64
		a.AmountCents = &v
	}
	a.Metadata = unmarshalMap(metadataRaw)
	createdAt, err := parseDBTime(createdAtRaw)
	if err != nil {
		return BillingAuditLog{}, fmt.Errorf("parse billing audit created_at: %w", err)
	}
	a.CreatedAt = createdAt
	return a, nil
}

func normalizeLimitOffset(limit, offset int) (int, int) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}

func formatDBTime(t time.Time) string {
	if t.IsZero() {
		t = time.Now().UTC()
	}
	return t.UTC().Format(time.RFC3339)
}
