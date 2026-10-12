package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

type BudgetsRepo struct {
	db *sql.DB
}

func NewBudgetsRepo(db *sql.DB) *BudgetsRepo {
	return &BudgetsRepo{db: db}
}

func (r *BudgetsRepo) Upsert(ctx context.Context, b Budget) error {
	if b.ID == "" {
		return fmt.Errorf("budget id is required")
	}
	if b.Period == "" {
		b.Period = "monthly"
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO budgets(
			id, name, subject_type, subject_id, api_key_id, limit_tokens, limit_cents,
			period, starts_at, ends_at, is_active, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, datetime('now'), datetime('now'))
		ON CONFLICT(id) DO UPDATE SET
			name = excluded.name,
			subject_type = excluded.subject_type,
			subject_id = excluded.subject_id,
			api_key_id = excluded.api_key_id,
			limit_tokens = excluded.limit_tokens,
			limit_cents = excluded.limit_cents,
			period = excluded.period,
			starts_at = excluded.starts_at,
			ends_at = excluded.ends_at,
			is_active = excluded.is_active,
			updated_at = datetime('now')
	`, b.ID, b.Name, b.SubjectType, nullableString(b.SubjectID), nullableInt64(b.APIKeyID),
		nullableInt64(b.LimitTokens), nullableInt64(b.LimitCents), b.Period,
		nullableTime(b.StartsAt), nullableTime(b.EndsAt), boolToInt(b.IsActive))
	if err != nil {
		return fmt.Errorf("upsert budget: %w", err)
	}
	return nil
}

func (r *BudgetsRepo) ListActiveForAPIKey(ctx context.Context, apiKeyID int64, at time.Time) ([]Budget, error) {
	apiKeyIDText := fmt.Sprintf("%d", apiKeyID)
	now := formatDBTime(at.UTC())
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, name, subject_type, subject_id, api_key_id, limit_tokens, limit_cents,
		       period, starts_at, ends_at, is_active, created_at, updated_at
		FROM budgets
		WHERE is_active = 1
		  AND (api_key_id = ? OR (subject_type = 'api_key' AND subject_id = ?))
		  AND (starts_at IS NULL OR starts_at <= ?)
		  AND (ends_at IS NULL OR ends_at > ?)
		ORDER BY created_at ASC
	`, apiKeyID, apiKeyIDText, now, now)
	if err != nil {
		return nil, fmt.Errorf("list active budgets for api key: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := []Budget{}
	for rows.Next() {
		b, err := scanBudget(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate budgets: %w", err)
	}
	return out, nil
}

type budgetScanner interface {
	Scan(dest ...any) error
}

func scanBudget(s budgetScanner) (Budget, error) {
	var b Budget
	var subjectID sql.NullString
	var apiKeyID sql.NullInt64
	var limitTokens sql.NullInt64
	var limitCents sql.NullInt64
	var startsRaw sql.NullString
	var endsRaw sql.NullString
	var isActive int
	var createdRaw, updatedRaw string
	if err := s.Scan(
		&b.ID,
		&b.Name,
		&b.SubjectType,
		&subjectID,
		&apiKeyID,
		&limitTokens,
		&limitCents,
		&b.Period,
		&startsRaw,
		&endsRaw,
		&isActive,
		&createdRaw,
		&updatedRaw,
	); err != nil {
		return Budget{}, fmt.Errorf("scan budget: %w", err)
	}
	if subjectID.Valid {
		b.SubjectID = subjectID.String
	}
	if apiKeyID.Valid {
		v := apiKeyID.Int64
		b.APIKeyID = &v
	}
	if limitTokens.Valid {
		v := limitTokens.Int64
		b.LimitTokens = &v
	}
	if limitCents.Valid {
		v := limitCents.Int64
		b.LimitCents = &v
	}
	if startsRaw.Valid && startsRaw.String != "" {
		t, err := parseDBTime(startsRaw.String)
		if err != nil {
			return Budget{}, fmt.Errorf("parse budget starts_at: %w", err)
		}
		b.StartsAt = &t
	}
	if endsRaw.Valid && endsRaw.String != "" {
		t, err := parseDBTime(endsRaw.String)
		if err != nil {
			return Budget{}, fmt.Errorf("parse budget ends_at: %w", err)
		}
		b.EndsAt = &t
	}
	createdAt, err := parseDBTime(createdRaw)
	if err != nil {
		return Budget{}, fmt.Errorf("parse budget created_at: %w", err)
	}
	updatedAt, err := parseDBTime(updatedRaw)
	if err != nil {
		return Budget{}, fmt.Errorf("parse budget updated_at: %w", err)
	}
	b.IsActive = isActive == 1
	b.CreatedAt = createdAt
	b.UpdatedAt = updatedAt
	return b, nil
}
