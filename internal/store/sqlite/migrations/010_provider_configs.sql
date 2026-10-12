CREATE TABLE IF NOT EXISTS provider_configs (
    id TEXT PRIMARY KEY,
    type TEXT NOT NULL,
    name TEXT NOT NULL,
    base_url TEXT NULL,
    credential_ref TEXT NULL,
    config_json TEXT NOT NULL DEFAULT '{}',
    is_enabled INTEGER NOT NULL DEFAULT 1,
    is_default INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_provider_configs_type_enabled ON provider_configs(type, is_enabled);
CREATE UNIQUE INDEX IF NOT EXISTS idx_provider_configs_default ON provider_configs(is_default) WHERE is_default = 1;
