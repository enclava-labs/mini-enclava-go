package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

type ConnectorsRepo struct {
	db *sql.DB
}

func NewConnectorsRepo(db *sql.DB) *ConnectorsRepo {
	return &ConnectorsRepo{db: db}
}

func (r *ConnectorsRepo) CreateSource(ctx context.Context, source ConnectorSource) error {
	configJSON, err := marshalJSON(nonNilSettings(source.Config))
	if err != nil {
		return fmt.Errorf("marshal connector config: %w", err)
	}
	checkpointJSON, err := marshalJSON(nonNilSettings(source.Checkpoint))
	if err != nil {
		return fmt.Errorf("marshal connector checkpoint: %w", err)
	}
	_, err = r.db.ExecContext(ctx, `
		INSERT INTO connector_sources(
			id, type, name, target_collection_id, config_json,
			credential_ref, checkpoint_json, is_enabled, last_sync_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, source.ID, source.Type, source.Name, nullableString(source.TargetCollectionID),
		configJSON, nullableString(source.CredentialRef), checkpointJSON, boolToInt(source.IsEnabled),
		nullableTime(source.LastSyncAt))
	if err != nil {
		return fmt.Errorf("create connector source: %w", err)
	}
	return nil
}

func (r *ConnectorsRepo) ListSources(ctx context.Context, sourceType string, limit, offset int) ([]ConnectorSource, error) {
	limit, offset = normalizeLimitOffset(limit, offset)
	where := []string{"1=1"}
	args := []any{}
	if strings.TrimSpace(sourceType) != "" {
		where = append(where, "type = ?")
		args = append(args, strings.TrimSpace(sourceType))
	}
	args = append(args, limit, offset)
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, type, name, target_collection_id, config_json, credential_ref,
		       checkpoint_json, is_enabled, created_at, updated_at, last_sync_at
		FROM connector_sources
		WHERE `+strings.Join(where, " AND ")+`
		ORDER BY created_at DESC
		LIMIT ? OFFSET ?
	`, args...)
	if err != nil {
		return nil, fmt.Errorf("list connector sources: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := []ConnectorSource{}
	for rows.Next() {
		source, err := scanConnectorSource(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, source)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate connector sources: %w", err)
	}
	return out, nil
}

func (r *ConnectorsRepo) GetSource(ctx context.Context, id string) (ConnectorSource, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, type, name, target_collection_id, config_json, credential_ref,
		       checkpoint_json, is_enabled, created_at, updated_at, last_sync_at
		FROM connector_sources
		WHERE id = ?
	`, id)
	return scanConnectorSource(row)
}

func (r *ConnectorsRepo) UpdateSource(ctx context.Context, source ConnectorSource) error {
	configJSON, err := marshalJSON(nonNilSettings(source.Config))
	if err != nil {
		return fmt.Errorf("marshal connector config: %w", err)
	}
	checkpointJSON, err := marshalJSON(nonNilSettings(source.Checkpoint))
	if err != nil {
		return fmt.Errorf("marshal connector checkpoint: %w", err)
	}
	res, err := r.db.ExecContext(ctx, `
		UPDATE connector_sources
		SET type = ?,
		    name = ?,
		    target_collection_id = ?,
		    config_json = ?,
		    credential_ref = ?,
		    checkpoint_json = ?,
		    is_enabled = ?,
		    updated_at = datetime('now'),
		    last_sync_at = ?
		WHERE id = ?
	`, source.Type, source.Name, nullableString(source.TargetCollectionID),
		configJSON, nullableString(source.CredentialRef), checkpointJSON, boolToInt(source.IsEnabled),
		nullableTime(source.LastSyncAt), source.ID)
	if err != nil {
		return fmt.Errorf("update connector source: %w", err)
	}
	if rows, err := res.RowsAffected(); err == nil && rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *ConnectorsRepo) DeleteSource(ctx context.Context, id string) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM connector_sources WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete connector source: %w", err)
	}
	if rows, err := res.RowsAffected(); err == nil && rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *ConnectorsRepo) CreateSyncJob(ctx context.Context, job ConnectorSyncJob) error {
	statsJSON, err := marshalJSON(nonNilSettings(job.Stats))
	if err != nil {
		return fmt.Errorf("marshal connector sync stats: %w", err)
	}
	_, err = r.db.ExecContext(ctx, `
		INSERT INTO connector_sync_jobs(
			id, connector_source_id, status, started_at, completed_at,
			error_message, stats_json
		) VALUES (?, ?, ?, ?, ?, ?, ?)
	`, job.ID, job.ConnectorSourceID, job.Status, nullableTime(job.StartedAt), nullableTime(job.CompletedAt),
		nullableString(job.ErrorMessage), statsJSON)
	if err != nil {
		return fmt.Errorf("create connector sync job: %w", err)
	}
	return nil
}

func (r *ConnectorsRepo) UpdateSyncJob(ctx context.Context, job ConnectorSyncJob) error {
	statsJSON, err := marshalJSON(nonNilSettings(job.Stats))
	if err != nil {
		return fmt.Errorf("marshal connector sync stats: %w", err)
	}
	res, err := r.db.ExecContext(ctx, `
		UPDATE connector_sync_jobs
		SET status = ?,
		    started_at = ?,
		    completed_at = ?,
		    error_message = ?,
		    stats_json = ?
		WHERE id = ?
	`, job.Status, nullableTime(job.StartedAt), nullableTime(job.CompletedAt),
		nullableString(job.ErrorMessage), statsJSON, job.ID)
	if err != nil {
		return fmt.Errorf("update connector sync job: %w", err)
	}
	if rows, err := res.RowsAffected(); err == nil && rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *ConnectorsRepo) ListSyncJobs(ctx context.Context, sourceID, status string, limit, offset int) ([]ConnectorSyncJob, error) {
	limit, offset = normalizeLimitOffset(limit, offset)
	where := []string{"1=1"}
	args := []any{}
	if strings.TrimSpace(sourceID) != "" {
		where = append(where, "connector_source_id = ?")
		args = append(args, strings.TrimSpace(sourceID))
	}
	if strings.TrimSpace(status) != "" {
		where = append(where, "status = ?")
		args = append(args, strings.TrimSpace(status))
	}
	args = append(args, limit, offset)
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, connector_source_id, status, started_at, completed_at,
		       error_message, stats_json, created_at
		FROM connector_sync_jobs
		WHERE `+strings.Join(where, " AND ")+`
		ORDER BY created_at DESC
		LIMIT ? OFFSET ?
	`, args...)
	if err != nil {
		return nil, fmt.Errorf("list connector sync jobs: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := []ConnectorSyncJob{}
	for rows.Next() {
		job, err := scanConnectorSyncJob(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, job)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate connector sync jobs: %w", err)
	}
	return out, nil
}

func (r *ConnectorsRepo) GetSyncJob(ctx context.Context, id string) (ConnectorSyncJob, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, connector_source_id, status, started_at, completed_at,
		       error_message, stats_json, created_at
		FROM connector_sync_jobs
		WHERE id = ?
	`, id)
	return scanConnectorSyncJob(row)
}

type connectorScanner interface {
	Scan(dest ...any) error
}

func scanConnectorSource(s connectorScanner) (ConnectorSource, error) {
	var source ConnectorSource
	var targetCollectionID, credentialRef, lastSyncRaw sql.NullString
	var configRaw, checkpointRaw string
	var enabled int
	var createdAtRaw, updatedAtRaw string
	if err := s.Scan(
		&source.ID,
		&source.Type,
		&source.Name,
		&targetCollectionID,
		&configRaw,
		&credentialRef,
		&checkpointRaw,
		&enabled,
		&createdAtRaw,
		&updatedAtRaw,
		&lastSyncRaw,
	); err != nil {
		return ConnectorSource{}, err
	}
	if targetCollectionID.Valid {
		source.TargetCollectionID = targetCollectionID.String
	}
	if credentialRef.Valid {
		source.CredentialRef = credentialRef.String
	}
	source.Config = unmarshalMap(configRaw)
	source.Checkpoint = unmarshalMap(checkpointRaw)
	source.IsEnabled = enabled == 1
	createdAt, err := parseDBTime(createdAtRaw)
	if err != nil {
		return ConnectorSource{}, fmt.Errorf("parse connector source created_at: %w", err)
	}
	updatedAt, err := parseDBTime(updatedAtRaw)
	if err != nil {
		return ConnectorSource{}, fmt.Errorf("parse connector source updated_at: %w", err)
	}
	source.CreatedAt = createdAt
	source.UpdatedAt = updatedAt
	if lastSyncRaw.Valid && lastSyncRaw.String != "" {
		lastSyncAt, err := parseDBTime(lastSyncRaw.String)
		if err != nil {
			return ConnectorSource{}, fmt.Errorf("parse connector source last_sync_at: %w", err)
		}
		source.LastSyncAt = &lastSyncAt
	}
	return source, nil
}

func scanConnectorSyncJob(s connectorScanner) (ConnectorSyncJob, error) {
	var job ConnectorSyncJob
	var startedRaw, completedRaw, errorMessage sql.NullString
	var statsRaw, createdAtRaw string
	if err := s.Scan(
		&job.ID,
		&job.ConnectorSourceID,
		&job.Status,
		&startedRaw,
		&completedRaw,
		&errorMessage,
		&statsRaw,
		&createdAtRaw,
	); err != nil {
		return ConnectorSyncJob{}, err
	}
	if startedRaw.Valid && startedRaw.String != "" {
		startedAt, err := parseDBTime(startedRaw.String)
		if err != nil {
			return ConnectorSyncJob{}, fmt.Errorf("parse connector job started_at: %w", err)
		}
		job.StartedAt = &startedAt
	}
	if completedRaw.Valid && completedRaw.String != "" {
		completedAt, err := parseDBTime(completedRaw.String)
		if err != nil {
			return ConnectorSyncJob{}, fmt.Errorf("parse connector job completed_at: %w", err)
		}
		job.CompletedAt = &completedAt
	}
	if errorMessage.Valid {
		job.ErrorMessage = errorMessage.String
	}
	job.Stats = unmarshalMap(statsRaw)
	createdAt, err := parseDBTime(createdAtRaw)
	if err != nil {
		return ConnectorSyncJob{}, fmt.Errorf("parse connector job created_at: %w", err)
	}
	job.CreatedAt = createdAt
	return job, nil
}
