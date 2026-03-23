CREATE TABLE IF NOT EXISTS usage_records (
    id TEXT PRIMARY KEY,
    api_key_id INTEGER NOT NULL,
    endpoint TEXT NOT NULL,
    model TEXT NULL,
    prompt_tokens INTEGER NOT NULL DEFAULT 0,
    completion_tokens INTEGER NOT NULL DEFAULT 0,
    total_tokens INTEGER NOT NULL DEFAULT 0,
    cost_cents INTEGER NOT NULL DEFAULT 0,
    status_code INTEGER NOT NULL DEFAULT 200,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    FOREIGN KEY(api_key_id) REFERENCES api_keys(id)
);
CREATE INDEX IF NOT EXISTS idx_usage_records_api_key_created_at ON usage_records(api_key_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_usage_records_created_at ON usage_records(created_at DESC);
