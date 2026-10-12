# Enclava Go Rewrite Schema Notes

SQLite remains the default durable store for the Go appliance. Migrations are
embedded from `internal/store/sqlite/migrations` and are applied in filename
order.

## Existing Mini Schema

The current mini application already has:

- `api_keys`
- `extract_templates` (`output_schema` is added by migration `012_extract_output_schema.sql`)
- `extract_settings`
- `extract_jobs`
- `extract_results`
- `usage_records`

These tables support API key auth, Extract, and usage accounting.

## Foundation Schema Added For Rewrite

Migration `006_foundation_schema.sql` adds the remaining foundation tables for
the implementation plan.

### Users And Roles

- `users`
- `roles`
- `user_roles`
- `dashboard_sessions`

These support future dashboard/operator management. Current dashboard session
cookies remain stateless; `dashboard_sessions` is present for a durable session
store if needed.

### Pricing, Budgets, And Audit

- `provider_pricing`
- `pricing_audit_log`
- `billing_audit_log`
- `budgets`

`provider_pricing` stores effective-dated pricing snapshots. Audit tables record
pricing and billing-sensitive control plane changes. `budgets` supports
platform/API-key/chatbot style subjects while keeping the existing API-key
budget columns for compatibility. API-key restriction columns include model,
endpoint, extract-template, chatbot, agent, and source-IP allowlists.

### RAG

- `rag_collections`
- `rag_documents`
- `rag_document_chunks`

Collections map to Qdrant collection names and carry embedding configuration,
chunking defaults, ACL JSON, and metadata. Documents and chunks track processing
status, source metadata, and Qdrant point IDs/counts so deletes can clean up both
SQLite metadata and vectors.

### Prompt Templates And Chatbots

- `prompt_templates`
- `chatbot_instances`
- `chatbot_conversations`
- `chatbot_messages`

Prompt templates are reusable system/user prompt definitions. Chatbot instances
bind prompt templates, provider/model preferences, settings, and RAG collection
IDs. Conversations and messages store external chatbot history and usage
metadata.

### Connectors

- `connector_sources`
- `connector_sync_jobs`

Connector sources store first-party connector configuration, target RAG
collection, credential reference, and checkpoint JSON. Sync jobs store status,
errors, and per-run stats.

## Index And Foreign Key Requirements

Hot-path indexes exist for:

- API key lookup by hash and prefix.
- Usage by API key and created time.
- Budgets by API key/subject and active state.
- RAG collections by owner and name.
- RAG documents by collection/status and source.
- RAG chunks by collection and Qdrant point ID.
- Chatbot conversations/messages by parent and created time.
- Connector sources by type/enabled state.
- Connector sync jobs by source/status and created time.

SQLite foreign key enforcement is enabled at connection open with
`PRAGMA foreign_keys = ON`. Migration tests verify empty database migration,
upgrade from the current mini schema, representative indexes, and representative
foreign keys.
