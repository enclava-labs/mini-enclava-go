package sqlite

import (
	"context"
	"database/sql"
	"fmt"
)

type PromptTemplatesRepo struct {
	db *sql.DB
}

func NewPromptTemplatesRepo(db *sql.DB) *PromptTemplatesRepo {
	return &PromptTemplatesRepo{db: db}
}

func (r *PromptTemplatesRepo) Create(ctx context.Context, t PromptTemplate) error {
	if t.VariablesSchema == nil {
		t.VariablesSchema = map[string]any{}
	}
	schemaJSON, err := marshalJSON(t.VariablesSchema)
	if err != nil {
		return fmt.Errorf("marshal prompt variables_schema: %w", err)
	}
	_, err = r.db.ExecContext(ctx, `
		INSERT INTO prompt_templates(
			id, name, description, system_prompt, user_prompt,
			variables_schema, is_default, is_active
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, t.ID, t.Name, nullableString(t.Description), t.SystemPrompt, t.UserPrompt,
		schemaJSON, boolToInt(t.IsDefault), boolToInt(t.IsActive))
	if err != nil {
		return fmt.Errorf("create prompt template: %w", err)
	}
	return nil
}

func (r *PromptTemplatesRepo) Upsert(ctx context.Context, t PromptTemplate) error {
	if t.VariablesSchema == nil {
		t.VariablesSchema = map[string]any{}
	}
	schemaJSON, err := marshalJSON(t.VariablesSchema)
	if err != nil {
		return fmt.Errorf("marshal prompt variables_schema: %w", err)
	}
	_, err = r.db.ExecContext(ctx, `
		INSERT INTO prompt_templates(
			id, name, description, system_prompt, user_prompt,
			variables_schema, is_default, is_active, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, datetime('now'), datetime('now'))
		ON CONFLICT(id) DO UPDATE SET
			name = excluded.name,
			description = excluded.description,
			system_prompt = excluded.system_prompt,
			user_prompt = excluded.user_prompt,
			variables_schema = excluded.variables_schema,
			is_default = excluded.is_default,
			is_active = excluded.is_active,
			updated_at = datetime('now')
	`, t.ID, t.Name, nullableString(t.Description), t.SystemPrompt, t.UserPrompt,
		schemaJSON, boolToInt(t.IsDefault), boolToInt(t.IsActive))
	if err != nil {
		return fmt.Errorf("upsert prompt template: %w", err)
	}
	return nil
}

func (r *PromptTemplatesRepo) EnsureDefaults(ctx context.Context, templates []PromptTemplate) error {
	var count int
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM prompt_templates`).Scan(&count); err != nil {
		return fmt.Errorf("count prompt templates: %w", err)
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

func (r *PromptTemplatesRepo) List(ctx context.Context) ([]PromptTemplate, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, name, description, system_prompt, user_prompt,
		       variables_schema, is_default, is_active, created_at, updated_at
		FROM prompt_templates
		ORDER BY is_default DESC, name ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("list prompt templates: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := []PromptTemplate{}
	for rows.Next() {
		t, err := scanPromptTemplate(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate prompt templates: %w", err)
	}
	return out, nil
}

func (r *PromptTemplatesRepo) GetByID(ctx context.Context, id string) (PromptTemplate, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, name, description, system_prompt, user_prompt,
		       variables_schema, is_default, is_active, created_at, updated_at
		FROM prompt_templates
		WHERE id = ?
	`, id)
	return scanPromptTemplate(row)
}

func (r *PromptTemplatesRepo) GetDefault(ctx context.Context) (PromptTemplate, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, name, description, system_prompt, user_prompt,
		       variables_schema, is_default, is_active, created_at, updated_at
		FROM prompt_templates
		WHERE is_default = 1 AND is_active = 1
		ORDER BY updated_at DESC
		LIMIT 1
	`)
	return scanPromptTemplate(row)
}

func (r *PromptTemplatesRepo) Update(ctx context.Context, t PromptTemplate) error {
	if t.VariablesSchema == nil {
		t.VariablesSchema = map[string]any{}
	}
	schemaJSON, err := marshalJSON(t.VariablesSchema)
	if err != nil {
		return fmt.Errorf("marshal prompt variables_schema: %w", err)
	}
	res, err := r.db.ExecContext(ctx, `
		UPDATE prompt_templates
		SET name = ?,
		    description = ?,
		    system_prompt = ?,
		    user_prompt = ?,
		    variables_schema = ?,
		    is_default = ?,
		    is_active = ?,
		    updated_at = datetime('now')
		WHERE id = ?
	`, t.Name, nullableString(t.Description), t.SystemPrompt, t.UserPrompt,
		schemaJSON, boolToInt(t.IsDefault), boolToInt(t.IsActive), t.ID)
	if err != nil {
		return fmt.Errorf("update prompt template: %w", err)
	}
	if rows, err := res.RowsAffected(); err == nil && rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *PromptTemplatesRepo) Delete(ctx context.Context, id string) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM prompt_templates WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete prompt template: %w", err)
	}
	if rows, err := res.RowsAffected(); err == nil && rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}

type promptTemplateScanner interface {
	Scan(dest ...any) error
}

func scanPromptTemplate(s promptTemplateScanner) (PromptTemplate, error) {
	var t PromptTemplate
	var desc sql.NullString
	var schemaRaw string
	var isDefault, isActive int
	var createdAtRaw, updatedAtRaw string
	if err := s.Scan(
		&t.ID,
		&t.Name,
		&desc,
		&t.SystemPrompt,
		&t.UserPrompt,
		&schemaRaw,
		&isDefault,
		&isActive,
		&createdAtRaw,
		&updatedAtRaw,
	); err != nil {
		return PromptTemplate{}, err
	}
	if desc.Valid {
		t.Description = desc.String
	}
	t.VariablesSchema = unmarshalMap(schemaRaw)
	t.IsDefault = isDefault == 1
	t.IsActive = isActive == 1
	createdAt, err := parseDBTime(createdAtRaw)
	if err != nil {
		return PromptTemplate{}, fmt.Errorf("parse prompt created_at: %w", err)
	}
	updatedAt, err := parseDBTime(updatedAtRaw)
	if err != nil {
		return PromptTemplate{}, fmt.Errorf("parse prompt updated_at: %w", err)
	}
	t.CreatedAt = createdAt
	t.UpdatedAt = updatedAt
	return t, nil
}
