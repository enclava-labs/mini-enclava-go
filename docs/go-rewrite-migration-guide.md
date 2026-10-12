# Go Rewrite Migration Import

The repo includes a best-effort Postgres JSONL exporter and an offline Go
importer for cutover data:

```bash
DATABASE_URL=postgres://... ./scripts/export-postgres-jsonl.sh ./export
enclava migrate-import -input-dir ./export -database ./enclava.db -dry-run
enclava migrate-import -input-dir ./export -database ./enclava.db
```

The export script writes one JSON object per line and skips missing tables. The
import command validates JSONL during dry-run without opening or writing the
database. A real import applies migrations first, writes all rows in one
transaction, and recomputes API-key usage totals from imported usage records.

Supported optional files:

- `users.jsonl`
- `roles.jsonl`
- `api_keys.jsonl`
- `budgets.jsonl`
- `provider_pricing.jsonl`
- `usage_records.jsonl`
- `extract_templates.jsonl`
- `prompt_templates.jsonl`
- `chatbots.jsonl`
- `chatbot_conversations.jsonl`
- `rag_collections.jsonl`
- `rag_documents.jsonl`
- `connector_sources.jsonl`

Plugin-era tables should not be exported. API keys are imported as existing
hash metadata (`key_prefix`, `key_hash`, `key_hash_algo`); plaintext keys are
not required or stored. API-key allowlists can include `allowed_models`,
`allowed_endpoints`, `allowed_extract_templates`, `allowed_chatbots`,
`allowed_agents`, and `allowed_ips`. RAG vector data should keep the same
Qdrant collection names via each collection's `qdrant_collection` field.

Extract templates are imported from `extract_templates.jsonl`, including
`context_schema` and `output_schema` when present. Missing schema fields default
to `{}` for compatibility with older exports.

Connector sources imported with `config.sync_interval_seconds` can participate
in scheduled sync when the process is started with `CONNECTOR_SYNC_INTERVAL`
greater than zero. Source credentials should be represented as `credential_ref`
values that point to environment variables available to the Go process.
