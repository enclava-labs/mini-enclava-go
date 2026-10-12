ALTER TABLE usage_records ADD COLUMN provider_id TEXT NULL;
ALTER TABLE usage_records ADD COLUMN provider_pricing_id INTEGER NULL;
ALTER TABLE usage_records ADD COLUMN pricing_currency TEXT NULL;
ALTER TABLE usage_records ADD COLUMN input_token_cents_per_1k REAL NOT NULL DEFAULT 0;
ALTER TABLE usage_records ADD COLUMN output_token_cents_per_1k REAL NOT NULL DEFAULT 0;
ALTER TABLE usage_records ADD COLUMN embedding_token_cents_per_1k REAL NOT NULL DEFAULT 0;
CREATE INDEX IF NOT EXISTS idx_usage_records_provider_model_created_at ON usage_records(provider_id, model, created_at DESC);
