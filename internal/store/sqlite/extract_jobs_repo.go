package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

type ExtractJobsRepo struct {
	db *sql.DB
}

func NewExtractJobsRepo(db *sql.DB) *ExtractJobsRepo {
	return &ExtractJobsRepo{db: db}
}

func (r *ExtractJobsRepo) Create(ctx context.Context, job ExtractJob) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO extract_jobs(id, api_key_id, template_id, status, file_name, file_mime_type, file_size_bytes, num_pages, model_used, error_message, created_at, updated_at, completed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, datetime('now'), datetime('now'), NULL)
	`, job.ID, job.APIKeyID, job.TemplateID, job.Status, job.FileName, job.FileMimeType, job.FileSizeBytes, job.NumPages, nullableString(job.ModelUsed), nullableString(job.ErrorMessage))
	if err != nil {
		return fmt.Errorf("create extract job: %w", err)
	}
	return nil
}

func (r *ExtractJobsRepo) UpdateStatus(ctx context.Context, jobID, status, modelUsed, errorMessage string, markCompleted bool) error {
	query := `
		UPDATE extract_jobs
		SET status = ?,
		    model_used = ?,
		    error_message = ?,
		    updated_at = datetime('now'),
		    completed_at = NULL
		WHERE id = ?
	`
	if markCompleted {
		query = `
			UPDATE extract_jobs
			SET status = ?,
			    model_used = ?,
			    error_message = ?,
			    updated_at = datetime('now'),
			    completed_at = datetime('now')
			WHERE id = ?
		`
	}
	_, err := r.db.ExecContext(ctx, query, status, nullableString(modelUsed), nullableString(errorMessage), jobID)
	if err != nil {
		return fmt.Errorf("update extract job status: %w", err)
	}
	return nil
}

func (r *ExtractJobsRepo) ListAll(ctx context.Context, limit, offset int) ([]ExtractJob, int, error) {
	var total int
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM extract_jobs`).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count all extract jobs: %w", err)
	}

	rows, err := r.db.QueryContext(ctx, `
		SELECT id, api_key_id, template_id, status, file_name, file_mime_type, file_size_bytes, num_pages, model_used, error_message, created_at, updated_at, completed_at
		FROM extract_jobs
		ORDER BY created_at DESC
		LIMIT ? OFFSET ?
	`, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list all extract jobs: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	jobs := []ExtractJob{}
	for rows.Next() {
		job, err := scanExtractJob(rows)
		if err != nil {
			return nil, 0, err
		}
		jobs = append(jobs, job)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate rows: %w", err)
	}
	return jobs, total, nil
}

func (r *ExtractJobsRepo) List(ctx context.Context, apiKeyID int64, limit, offset int) ([]ExtractJob, int, error) {
	var total int
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM extract_jobs WHERE api_key_id = ?`, apiKeyID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count extract jobs: %w", err)
	}

	rows, err := r.db.QueryContext(ctx, `
		SELECT id, api_key_id, template_id, status, file_name, file_mime_type, file_size_bytes, num_pages, model_used, error_message, created_at, updated_at, completed_at
		FROM extract_jobs
		WHERE api_key_id = ?
		ORDER BY created_at DESC
		LIMIT ? OFFSET ?
	`, apiKeyID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list extract jobs: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	jobs := []ExtractJob{}
	for rows.Next() {
		job, err := scanExtractJob(rows)
		if err != nil {
			return nil, 0, err
		}
		jobs = append(jobs, job)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate rows: %w", err)
	}
	return jobs, total, nil
}

func (r *ExtractJobsRepo) Get(ctx context.Context, apiKeyID int64, jobID string) (ExtractJob, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, api_key_id, template_id, status, file_name, file_mime_type, file_size_bytes, num_pages, model_used, error_message, created_at, updated_at, completed_at
		FROM extract_jobs
		WHERE api_key_id = ? AND id = ?
	`, apiKeyID, jobID)
	return scanExtractJob(row)
}

func (r *ExtractJobsRepo) SaveResult(ctx context.Context, result ExtractResult) error {
	if result.Data == nil {
		result.Data = map[string]any{}
	}
	dataJSON, err := marshalJSON(result.Data)
	if err != nil {
		return fmt.Errorf("marshal result data: %w", err)
	}
	errorsJSON, err := marshalJSON(result.ValidationErrors)
	if err != nil {
		return fmt.Errorf("marshal validation errors: %w", err)
	}
	warningsJSON, err := marshalJSON(result.ValidationWarnings)
	if err != nil {
		return fmt.Errorf("marshal validation warnings: %w", err)
	}
	_, err = r.db.ExecContext(ctx, `
		INSERT INTO extract_results(id, job_id, data_json, raw_response, validation_errors, validation_warnings, tokens_used, cost_cents, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, datetime('now'))
	`, result.ID, result.JobID, dataJSON, result.RawResponse, errorsJSON, warningsJSON, result.TokensUsed, result.CostCents)
	if err != nil {
		return fmt.Errorf("save extract result: %w", err)
	}
	return nil
}

func (r *ExtractJobsRepo) GetResultByJobID(ctx context.Context, jobID string) (ExtractResult, error) {
	var result ExtractResult
	var dataRaw, errorsRaw, warningsRaw string
	var createdAtRaw string
	err := r.db.QueryRowContext(ctx, `
		SELECT id, job_id, data_json, raw_response, validation_errors, validation_warnings, tokens_used, cost_cents, created_at
		FROM extract_results
		WHERE job_id = ?
	`, jobID).Scan(
		&result.ID,
		&result.JobID,
		&dataRaw,
		&result.RawResponse,
		&errorsRaw,
		&warningsRaw,
		&result.TokensUsed,
		&result.CostCents,
		&createdAtRaw,
	)
	if err != nil {
		return ExtractResult{}, err
	}

	result.Data = unmarshalMap(dataRaw)
	result.ValidationErrors = unmarshalStringList(errorsRaw)
	result.ValidationWarnings = unmarshalStringList(warningsRaw)
	createdAt, err := parseDBTime(createdAtRaw)
	if err != nil {
		return ExtractResult{}, fmt.Errorf("parse result created_at: %w", err)
	}
	result.CreatedAt = createdAt
	return result, nil
}

func (r *ExtractJobsRepo) CountAll(ctx context.Context) (int64, error) {
	var total int64
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM extract_jobs`).Scan(&total); err != nil {
		return 0, fmt.Errorf("count all extract jobs: %w", err)
	}
	return total, nil
}

func (r *ExtractJobsRepo) CountSince(ctx context.Context, since time.Time) (int64, error) {
	var total int64
	if err := r.db.QueryRowContext(ctx, `
		SELECT COUNT(1)
		FROM extract_jobs
		WHERE created_at >= ?
	`, since.UTC().Format("2006-01-02 15:04:05")).Scan(&total); err != nil {
		return 0, fmt.Errorf("count extract jobs since: %w", err)
	}
	return total, nil
}

func scanExtractJob(s scanner) (ExtractJob, error) {
	var job ExtractJob
	var modelUsed, errMsg, completedRaw sql.NullString
	var createdRaw, updatedRaw string
	if err := s.Scan(
		&job.ID,
		&job.APIKeyID,
		&job.TemplateID,
		&job.Status,
		&job.FileName,
		&job.FileMimeType,
		&job.FileSizeBytes,
		&job.NumPages,
		&modelUsed,
		&errMsg,
		&createdRaw,
		&updatedRaw,
		&completedRaw,
	); err != nil {
		return ExtractJob{}, fmt.Errorf("scan extract job: %w", err)
	}
	if modelUsed.Valid {
		job.ModelUsed = modelUsed.String
	}
	if errMsg.Valid {
		job.ErrorMessage = errMsg.String
	}
	createdAt, err := parseDBTime(createdRaw)
	if err != nil {
		return ExtractJob{}, fmt.Errorf("parse job created_at: %w", err)
	}
	updatedAt, err := parseDBTime(updatedRaw)
	if err != nil {
		return ExtractJob{}, fmt.Errorf("parse job updated_at: %w", err)
	}
	job.CreatedAt = createdAt
	job.UpdatedAt = updatedAt
	if completedRaw.Valid && completedRaw.String != "" {
		completedAt, err := parseDBTime(completedRaw.String)
		if err == nil {
			job.CompletedAt = &completedAt
		}
	}
	return job, nil
}
