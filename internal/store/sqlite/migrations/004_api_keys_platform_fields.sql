ALTER TABLE api_keys ADD COLUMN description TEXT NULL;
ALTER TABLE api_keys ADD COLUMN tags TEXT NOT NULL DEFAULT '[]';
ALTER TABLE api_keys ADD COLUMN allowed_ips TEXT NOT NULL DEFAULT '[]';
ALTER TABLE api_keys ADD COLUMN rate_limit_per_minute INTEGER NULL;
ALTER TABLE api_keys ADD COLUMN rate_limit_per_hour INTEGER NULL;
ALTER TABLE api_keys ADD COLUMN rate_limit_per_day INTEGER NULL;
ALTER TABLE api_keys ADD COLUMN is_unlimited INTEGER NOT NULL DEFAULT 1;
ALTER TABLE api_keys ADD COLUMN budget_limit_tokens INTEGER NULL;
ALTER TABLE api_keys ADD COLUMN budget_limit_cents INTEGER NULL;
ALTER TABLE api_keys ADD COLUMN budget_period TEXT NULL; -- 'total' or 'monthly'
ALTER TABLE api_keys ADD COLUMN is_deleted INTEGER NOT NULL DEFAULT 0;
ALTER TABLE api_keys ADD COLUMN deleted_at TEXT NULL;
