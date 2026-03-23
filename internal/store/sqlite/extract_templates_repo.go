package sqlite

import (
	"context"
	"database/sql"
	"fmt"
)

type ExtractTemplatesRepo struct {
	db *sql.DB
}

func NewExtractTemplatesRepo(db *sql.DB) *ExtractTemplatesRepo {
	return &ExtractTemplatesRepo{db: db}
}

func (r *ExtractTemplatesRepo) List(ctx context.Context) ([]ExtractTemplate, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, name, description, system_prompt, user_prompt, context_schema, model, is_default, is_active, created_at, updated_at
		FROM extract_templates
		ORDER BY id ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("list templates: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	out := []ExtractTemplate{}
	for rows.Next() {
		t, err := scanTemplate(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate rows: %w", err)
	}
	return out, nil
}

func (r *ExtractTemplatesRepo) GetByID(ctx context.Context, id string) (ExtractTemplate, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, name, description, system_prompt, user_prompt, context_schema, model, is_default, is_active, created_at, updated_at
		FROM extract_templates
		WHERE id = ?
	`, id)
	return scanTemplate(row)
}

func (r *ExtractTemplatesRepo) Upsert(ctx context.Context, t ExtractTemplate) error {
	if t.ContextSchema == nil {
		t.ContextSchema = map[string]any{}
	}
	schemaJSON, err := marshalJSON(t.ContextSchema)
	if err != nil {
		return fmt.Errorf("marshal context_schema: %w", err)
	}
	_, err = r.db.ExecContext(ctx, `
		INSERT INTO extract_templates(id, name, description, system_prompt, user_prompt, context_schema, model, is_default, is_active, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, datetime('now'), datetime('now'))
		ON CONFLICT(id) DO UPDATE SET
			name = excluded.name,
			description = excluded.description,
			system_prompt = excluded.system_prompt,
			user_prompt = excluded.user_prompt,
			context_schema = excluded.context_schema,
			model = excluded.model,
			is_default = excluded.is_default,
			is_active = excluded.is_active,
			updated_at = datetime('now')
	`, t.ID, t.Name, t.Description, t.SystemPrompt, t.UserPrompt, schemaJSON, nullableString(t.Model), boolToInt(t.IsDefault), boolToInt(t.IsActive))
	if err != nil {
		return fmt.Errorf("upsert template: %w", err)
	}
	return nil
}

func (r *ExtractTemplatesRepo) Delete(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM extract_templates WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete template: %w", err)
	}
	return nil
}

func (r *ExtractTemplatesRepo) ReplaceDefaults(ctx context.Context, templates []ExtractTemplate) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin replace defaults tx: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	if _, err := tx.ExecContext(ctx, `DELETE FROM extract_templates WHERE is_default = 1`); err != nil {
		return fmt.Errorf("delete default templates: %w", err)
	}

	for _, t := range templates {
		if t.ContextSchema == nil {
			t.ContextSchema = map[string]any{}
		}
		schemaJSON, err := marshalJSON(t.ContextSchema)
		if err != nil {
			return fmt.Errorf("marshal context_schema for %s: %w", t.ID, err)
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO extract_templates(id, name, description, system_prompt, user_prompt, context_schema, model, is_default, is_active, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, datetime('now'), datetime('now'))
			ON CONFLICT(id) DO UPDATE SET
				name = excluded.name,
				description = excluded.description,
				system_prompt = excluded.system_prompt,
				user_prompt = excluded.user_prompt,
				context_schema = excluded.context_schema,
				model = excluded.model,
				is_default = excluded.is_default,
				is_active = excluded.is_active,
				updated_at = datetime('now')
		`, t.ID, t.Name, t.Description, t.SystemPrompt, t.UserPrompt, schemaJSON, nullableString(t.Model), boolToInt(t.IsDefault), boolToInt(t.IsActive)); err != nil {
			return fmt.Errorf("insert default template %s: %w", t.ID, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit replace defaults: %w", err)
	}
	return nil
}

func (r *ExtractTemplatesRepo) EnsureDefaults(ctx context.Context, templates []ExtractTemplate) error {
	var count int
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM extract_templates`).Scan(&count); err != nil {
		return fmt.Errorf("count templates: %w", err)
	}
	if count > 0 {
		return nil
	}

	for _, t := range templates {
		if err := r.Upsert(ctx, t); err != nil {
			return err
		}
	}
	return nil
}

type scanner interface {
	Scan(dest ...any) error
}

func scanTemplate(s scanner) (ExtractTemplate, error) {
	var t ExtractTemplate
	var desc, model sql.NullString
	var contextRaw string
	var isDefault, isActive int
	var createdAtRaw, updatedAtRaw string
	if err := s.Scan(
		&t.ID,
		&t.Name,
		&desc,
		&t.SystemPrompt,
		&t.UserPrompt,
		&contextRaw,
		&model,
		&isDefault,
		&isActive,
		&createdAtRaw,
		&updatedAtRaw,
	); err != nil {
		return ExtractTemplate{}, err
	}
	if desc.Valid {
		t.Description = desc.String
	}
	if model.Valid {
		t.Model = model.String
	}
	t.IsDefault = isDefault == 1
	t.IsActive = isActive == 1
	t.ContextSchema = unmarshalMap(contextRaw)
	createdAt, err := parseDBTime(createdAtRaw)
	if err != nil {
		return ExtractTemplate{}, fmt.Errorf("parse template created_at: %w", err)
	}
	updatedAt, err := parseDBTime(updatedAtRaw)
	if err != nil {
		return ExtractTemplate{}, fmt.Errorf("parse template updated_at: %w", err)
	}
	t.CreatedAt = createdAt
	t.UpdatedAt = updatedAt
	return t, nil
}

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func nullableString(v string) any {
	if v == "" {
		return nil
	}
	return v
}
