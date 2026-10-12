ALTER TABLE usage_records ADD COLUMN chatbot_id TEXT NULL;
CREATE INDEX IF NOT EXISTS idx_usage_records_chatbot_created_at ON usage_records(chatbot_id, created_at DESC);
