CREATE TABLE IF NOT EXISTS schema_migrations (
    version TEXT PRIMARY KEY,
    applied_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS api_keys (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    key_prefix TEXT NOT NULL,
    key_hash TEXT NOT NULL,
    is_active INTEGER NOT NULL DEFAULT 1,
    expires_at TEXT NULL,
    scopes TEXT NOT NULL DEFAULT '[]',
    allowed_models TEXT NOT NULL DEFAULT '[]',
    allowed_endpoints TEXT NOT NULL DEFAULT '[]',
    allowed_extract_templates TEXT NOT NULL DEFAULT '[]',
    total_requests INTEGER NOT NULL DEFAULT 0,
    total_tokens INTEGER NOT NULL DEFAULT 0,
    total_cost_cents INTEGER NOT NULL DEFAULT 0,
    last_used_at TEXT NULL,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_api_keys_hash ON api_keys(key_hash);
CREATE INDEX IF NOT EXISTS idx_api_keys_prefix ON api_keys(key_prefix);

CREATE TABLE IF NOT EXISTS extract_templates (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    description TEXT,
    system_prompt TEXT NOT NULL,
    user_prompt TEXT NOT NULL,
    context_schema TEXT NOT NULL DEFAULT '{}',
    model TEXT NULL,
    is_default INTEGER NOT NULL DEFAULT 0,
    is_active INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS extract_settings (
    id INTEGER PRIMARY KEY,
    default_model TEXT NULL,
    max_file_size_mb INTEGER NOT NULL DEFAULT 10,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now')),
    CHECK (id = 1)
);

CREATE TABLE IF NOT EXISTS extract_jobs (
    id TEXT PRIMARY KEY,
    template_id TEXT NOT NULL,
    status TEXT NOT NULL,
    file_name TEXT NOT NULL,
    file_mime_type TEXT NOT NULL,
    file_size_bytes INTEGER NOT NULL,
    num_pages INTEGER NOT NULL DEFAULT 1,
    model_used TEXT NULL,
    error_message TEXT NULL,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now')),
    completed_at TEXT NULL,
    FOREIGN KEY(template_id) REFERENCES extract_templates(id)
);
CREATE INDEX IF NOT EXISTS idx_extract_jobs_created_at ON extract_jobs(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_extract_jobs_status ON extract_jobs(status);

CREATE TABLE IF NOT EXISTS extract_results (
    id TEXT PRIMARY KEY,
    job_id TEXT NOT NULL UNIQUE,
    data_json TEXT NOT NULL,
    raw_response TEXT NOT NULL,
    validation_errors TEXT NOT NULL DEFAULT '[]',
    validation_warnings TEXT NOT NULL DEFAULT '[]',
    tokens_used INTEGER NOT NULL DEFAULT 0,
    cost_cents INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    FOREIGN KEY(job_id) REFERENCES extract_jobs(id)
);
