ALTER TABLE usage_records ADD COLUMN cost_microcents INTEGER NOT NULL DEFAULT 0;
ALTER TABLE api_keys ADD COLUMN total_cost_microcents INTEGER NOT NULL DEFAULT 0;

UPDATE usage_records
SET cost_microcents = cost_cents * 1000000
WHERE cost_microcents = 0
  AND cost_cents != 0;

UPDATE api_keys
SET total_cost_microcents = total_cost_cents * 1000000
WHERE total_cost_microcents = 0
  AND total_cost_cents != 0;
