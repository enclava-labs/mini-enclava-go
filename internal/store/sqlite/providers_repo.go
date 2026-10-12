package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

type ProvidersRepo struct {
	db *sql.DB
}

func NewProvidersRepo(db *sql.DB) *ProvidersRepo {
	return &ProvidersRepo{db: db}
}

func (r *ProvidersRepo) EnsureDefault(ctx context.Context, provider ProviderConfig) error {
	if strings.TrimSpace(provider.ID) == "" {
		return fmt.Errorf("provider id is required")
	}
	if strings.TrimSpace(provider.Type) == "" {
		return fmt.Errorf("provider type is required")
	}
	if strings.TrimSpace(provider.Name) == "" {
		provider.Name = provider.ID
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin provider default tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var existing int
	err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM provider_configs WHERE id = ?`, provider.ID).Scan(&existing)
	if err != nil {
		return fmt.Errorf("lookup provider config: %w", err)
	}
	if existing > 0 {
		return nil
	}

	if provider.Config == nil {
		provider.Config = map[string]any{}
	}
	configJSON, err := marshalJSON(provider.Config)
	if err != nil {
		return fmt.Errorf("marshal provider config: %w", err)
	}
	if provider.IsDefault {
		if err := clearProviderDefault(ctx, tx, provider.ID); err != nil {
			return err
		}
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO provider_configs(
			id, type, name, base_url, credential_ref, config_json, is_enabled, is_default
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, provider.ID, provider.Type, provider.Name, nullableString(provider.BaseURL),
		nullableString(provider.CredentialRef), configJSON, boolToInt(provider.IsEnabled), boolToInt(provider.IsDefault))
	if err != nil {
		return fmt.Errorf("ensure default provider: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit provider default tx: %w", err)
	}
	return nil
}

func (r *ProvidersRepo) Create(ctx context.Context, provider ProviderConfig) error {
	configJSON, err := marshalJSON(nonNilSettings(provider.Config))
	if err != nil {
		return fmt.Errorf("marshal provider config: %w", err)
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin provider create tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if provider.IsDefault {
		if err := clearProviderDefault(ctx, tx, provider.ID); err != nil {
			return err
		}
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO provider_configs(
			id, type, name, base_url, credential_ref, config_json, is_enabled, is_default
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, provider.ID, provider.Type, provider.Name, nullableString(provider.BaseURL),
		nullableString(provider.CredentialRef), configJSON, boolToInt(provider.IsEnabled), boolToInt(provider.IsDefault))
	if err != nil {
		return fmt.Errorf("create provider config: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit provider create tx: %w", err)
	}
	return nil
}

func (r *ProvidersRepo) List(ctx context.Context, providerType string, includeDisabled bool) ([]ProviderConfig, error) {
	where := []string{"1=1"}
	args := []any{}
	if strings.TrimSpace(providerType) != "" {
		where = append(where, "type = ?")
		args = append(args, strings.TrimSpace(providerType))
	}
	if !includeDisabled {
		where = append(where, "is_enabled = 1")
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, type, name, base_url, credential_ref, config_json,
		       is_enabled, is_default, created_at, updated_at
		FROM provider_configs
		WHERE `+strings.Join(where, " AND ")+`
		ORDER BY is_default DESC, type ASC, name ASC
	`, args...)
	if err != nil {
		return nil, fmt.Errorf("list provider configs: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := []ProviderConfig{}
	for rows.Next() {
		provider, err := scanProviderConfig(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, provider)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate provider configs: %w", err)
	}
	return out, nil
}

func (r *ProvidersRepo) Get(ctx context.Context, id string) (ProviderConfig, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, type, name, base_url, credential_ref, config_json,
		       is_enabled, is_default, created_at, updated_at
		FROM provider_configs
		WHERE id = ?
	`, strings.TrimSpace(id))
	return scanProviderConfig(row)
}

func (r *ProvidersRepo) Update(ctx context.Context, provider ProviderConfig) error {
	configJSON, err := marshalJSON(nonNilSettings(provider.Config))
	if err != nil {
		return fmt.Errorf("marshal provider config: %w", err)
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin provider update tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if provider.IsDefault {
		if err := clearProviderDefault(ctx, tx, provider.ID); err != nil {
			return err
		}
	}
	res, err := tx.ExecContext(ctx, `
		UPDATE provider_configs
		SET type = ?,
		    name = ?,
		    base_url = ?,
		    credential_ref = ?,
		    config_json = ?,
		    is_enabled = ?,
		    is_default = ?,
		    updated_at = datetime('now')
		WHERE id = ?
	`, provider.Type, provider.Name, nullableString(provider.BaseURL), nullableString(provider.CredentialRef),
		configJSON, boolToInt(provider.IsEnabled), boolToInt(provider.IsDefault), provider.ID)
	if err != nil {
		return fmt.Errorf("update provider config: %w", err)
	}
	if rows, err := res.RowsAffected(); err == nil && rows == 0 {
		return sql.ErrNoRows
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit provider update tx: %w", err)
	}
	return nil
}

func (r *ProvidersRepo) Delete(ctx context.Context, id string) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM provider_configs WHERE id = ?`, strings.TrimSpace(id))
	if err != nil {
		return fmt.Errorf("delete provider config: %w", err)
	}
	if rows, err := res.RowsAffected(); err == nil && rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}

type providerConfigScanner interface {
	Scan(dest ...any) error
}

func clearProviderDefault(ctx context.Context, tx *sql.Tx, exceptID string) error {
	_, err := tx.ExecContext(ctx, `
		UPDATE provider_configs
		SET is_default = 0,
		    updated_at = datetime('now')
		WHERE is_default = 1
		  AND id <> ?
	`, exceptID)
	if err != nil {
		return fmt.Errorf("clear provider default: %w", err)
	}
	return nil
}

func scanProviderConfig(s providerConfigScanner) (ProviderConfig, error) {
	var provider ProviderConfig
	var baseURL, credentialRef sql.NullString
	var configRaw string
	var enabled, def int
	var createdAtRaw, updatedAtRaw string
	if err := s.Scan(
		&provider.ID,
		&provider.Type,
		&provider.Name,
		&baseURL,
		&credentialRef,
		&configRaw,
		&enabled,
		&def,
		&createdAtRaw,
		&updatedAtRaw,
	); err != nil {
		return ProviderConfig{}, err
	}
	provider.BaseURL = baseURL.String
	provider.CredentialRef = credentialRef.String
	provider.IsEnabled = enabled == 1
	provider.IsDefault = def == 1
	provider.Config = unmarshalMap(configRaw)
	createdAt, err := parseDBTime(createdAtRaw)
	if err != nil {
		return ProviderConfig{}, fmt.Errorf("parse provider created_at: %w", err)
	}
	updatedAt, err := parseDBTime(updatedAtRaw)
	if err != nil {
		return ProviderConfig{}, fmt.Errorf("parse provider updated_at: %w", err)
	}
	provider.CreatedAt = createdAt
	provider.UpdatedAt = updatedAt
	return provider, nil
}
