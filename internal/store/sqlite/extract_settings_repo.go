package sqlite

import (
	"context"
	"database/sql"
	"fmt"
)

type ExtractSettingsRepo struct {
	db *sql.DB
}

func NewExtractSettingsRepo(db *sql.DB) *ExtractSettingsRepo {
	return &ExtractSettingsRepo{db: db}
}

func (r *ExtractSettingsRepo) EnsureDefaultRow(ctx context.Context, maxFileSizeMB int64) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO extract_settings(id, default_model, max_file_size_mb, created_at, updated_at)
		VALUES (1, NULL, ?, datetime('now'), datetime('now'))
		ON CONFLICT(id) DO NOTHING
	`, maxFileSizeMB)
	if err != nil {
		return fmt.Errorf("ensure extract settings row: %w", err)
	}
	return nil
}

func (r *ExtractSettingsRepo) Get(ctx context.Context) (ExtractSettings, error) {
	var s ExtractSettings
	var defaultModel sql.NullString
	var createdAtRaw, updatedAtRaw string
	if err := r.db.QueryRowContext(ctx, `
		SELECT id, default_model, max_file_size_mb, created_at, updated_at
		FROM extract_settings WHERE id = 1
	`).Scan(&s.ID, &defaultModel, &s.MaxFileSizeMB, &createdAtRaw, &updatedAtRaw); err != nil {
		return ExtractSettings{}, fmt.Errorf("get extract settings: %w", err)
	}
	if defaultModel.Valid {
		s.DefaultModel = defaultModel.String
	}
	createdAt, err := parseDBTime(createdAtRaw)
	if err != nil {
		return ExtractSettings{}, fmt.Errorf("parse settings created_at: %w", err)
	}
	updatedAt, err := parseDBTime(updatedAtRaw)
	if err != nil {
		return ExtractSettings{}, fmt.Errorf("parse settings updated_at: %w", err)
	}
	s.CreatedAt = createdAt
	s.UpdatedAt = updatedAt
	return s, nil
}

func (r *ExtractSettingsRepo) Update(ctx context.Context, defaultModel string, maxFileSizeMB int64) (ExtractSettings, error) {
	_, err := r.db.ExecContext(ctx, `
		UPDATE extract_settings
		SET default_model = ?,
		    max_file_size_mb = ?,
		    updated_at = datetime('now')
		WHERE id = 1
	`, nullableString(defaultModel), maxFileSizeMB)
	if err != nil {
		return ExtractSettings{}, fmt.Errorf("update extract settings: %w", err)
	}
	return r.Get(ctx)
}
