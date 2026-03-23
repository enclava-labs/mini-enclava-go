ALTER TABLE extract_jobs ADD COLUMN api_key_id INTEGER NOT NULL DEFAULT 0;
CREATE INDEX IF NOT EXISTS idx_extract_jobs_api_key_id_created_at ON extract_jobs(api_key_id, created_at DESC);
