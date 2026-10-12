#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 1 ]]; then
  echo "usage: DATABASE_URL=postgres://... $0 <output-dir>" >&2
  exit 2
fi

if [[ -z "${DATABASE_URL:-}" ]]; then
  echo "DATABASE_URL is required" >&2
  exit 2
fi

out_dir="$1"
mkdir -p "$out_dir"

psql_base=(psql "$DATABASE_URL" -v ON_ERROR_STOP=1 -q)

table_exists() {
  local table="$1"
  local exists
  exists="$("${psql_base[@]}" -Atc "SELECT to_regclass('public.${table}') IS NOT NULL")"
  [[ "$exists" == "t" ]]
}

column_exists() {
  local table="$1"
  local column="$2"
  local exists
  exists="$("${psql_base[@]}" -Atc "SELECT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema = 'public' AND table_name = '${table}' AND column_name = '${column}')")"
  [[ "$exists" == "t" ]]
}

export_table() {
  local table="$1"
  local file="$2"
  local query="$3"
  if ! table_exists "$table"; then
    printf 'skip %s: table not found\n' "$table" >&2
    return 0
  fi
  printf 'export %s -> %s\n' "$table" "$file" >&2
  "${psql_base[@]}" -c "\\copy (${query}) TO '${out_dir}/${file}'"
}

export_table "users" "users.jsonl" \
  "SELECT row_to_json(t)::text FROM (SELECT id, email, display_name, password_hash, is_active, created_at, updated_at FROM users) t"

export_table "roles" "roles.jsonl" \
  "SELECT row_to_json(t)::text FROM (SELECT id, name, description, created_at, updated_at FROM roles) t"

api_key_allowed_agents_expr="'[]'::jsonb AS allowed_agents"
if table_exists "api_keys" && column_exists "api_keys" "allowed_agents"; then
  api_key_allowed_agents_expr="COALESCE(allowed_agents, '[]'::jsonb) AS allowed_agents"
fi

export_table "api_keys" "api_keys.jsonl" \
  "SELECT row_to_json(t)::text FROM (SELECT id, name, description, key_prefix, key_hash, COALESCE(key_hash_algo, 'sha256') AS key_hash_algo, is_active, is_deleted, deleted_at, expires_at, scopes, allowed_models, allowed_endpoints, allowed_extract_templates, COALESCE(allowed_chatbots, '[]'::jsonb) AS allowed_chatbots, ${api_key_allowed_agents_expr}, allowed_ips, rate_limit_per_minute, rate_limit_per_hour, rate_limit_per_day, is_unlimited, budget_limit_tokens, budget_limit_cents, budget_period, tags, total_requests, total_tokens, total_cost_cents, last_used_at, created_at, updated_at FROM api_keys) t"

export_table "budgets" "budgets.jsonl" \
  "SELECT row_to_json(t)::text FROM (SELECT id, name, subject_type, subject_id, api_key_id, limit_tokens, limit_cents, period, starts_at, ends_at, is_active, created_at, updated_at FROM budgets) t"

export_table "provider_pricing" "provider_pricing.jsonl" \
  "SELECT row_to_json(t)::text FROM (SELECT id, provider_id, model_id, currency, input_token_cents_per_1k, output_token_cents_per_1k, embedding_token_cents_per_1k, effective_at, created_at FROM provider_pricing) t"

export_table "usage_records" "usage_records.jsonl" \
  "SELECT row_to_json(t)::text FROM (SELECT id, api_key_id, endpoint, model, chatbot_id, provider_id, provider_pricing_id, pricing_currency, input_token_cents_per_1k, output_token_cents_per_1k, embedding_token_cents_per_1k, prompt_tokens, completion_tokens, total_tokens, cost_cents, status_code, created_at FROM usage_records) t"

extract_output_schema_expr="'{}'::jsonb AS output_schema"
if table_exists "extract_templates" && column_exists "extract_templates" "output_schema"; then
  extract_output_schema_expr="COALESCE(output_schema, '{}'::jsonb) AS output_schema"
fi
export_table "extract_templates" "extract_templates.jsonl" \
  "SELECT row_to_json(t)::text FROM (SELECT id, name, description, system_prompt, user_prompt, context_schema, ${extract_output_schema_expr}, model, is_default, is_active, created_at, updated_at FROM extract_templates) t"

export_table "prompt_templates" "prompt_templates.jsonl" \
  "SELECT row_to_json(t)::text FROM (SELECT id, name, description, system_prompt, user_prompt, variables_schema, is_default, is_active, created_at, updated_at FROM prompt_templates) t"

export_table "chatbot_instances" "chatbots.jsonl" \
  "SELECT row_to_json(t)::text FROM (SELECT id, name, description, prompt_template_id, provider_id, model, rag_collection_ids, settings_json, external_id, is_enabled, created_at, updated_at FROM chatbot_instances) t"

export_table "chatbot_conversations" "chatbot_conversations.jsonl" \
  "SELECT row_to_json(t)::text FROM (SELECT id, chatbot_id, api_key_id, external_user_id, title, metadata_json, created_at, updated_at FROM chatbot_conversations) t"

export_table "rag_collections" "rag_collections.jsonl" \
  "SELECT row_to_json(t)::text FROM (SELECT id, name, description, owner_api_key_id, qdrant_collection, embedding_provider, embedding_model, chunk_size, chunk_overlap, acl_json, metadata_json, is_active, created_at, updated_at FROM rag_collections) t"

export_table "rag_documents" "rag_documents.jsonl" \
  "SELECT row_to_json(t)::text FROM (SELECT id, collection_id, source_type, source_id, source_uri, file_name, mime_type, status, text_hash, metadata_json, qdrant_point_count, error_message, created_at, updated_at, processed_at FROM rag_documents) t"

export_table "connector_sources" "connector_sources.jsonl" \
  "SELECT row_to_json(t)::text FROM (SELECT id, type, name, target_collection_id, config_json, credential_ref, checkpoint_json, is_enabled, created_at, updated_at, last_sync_at FROM connector_sources) t"
