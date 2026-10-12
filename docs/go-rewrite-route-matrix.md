# Enclava Go Rewrite Route Matrix

Status values:

- `keep`: Preserve this route contract in Go.
- `replace`: Replace old frontend/internal behavior with a Go dashboard route.
- `defer`: In scope for the rewrite but not implemented in the current Go app.
- `drop`: Explicitly excluded from rewrite parity.

## Core And Health

| Route | Methods | Status | Scope/Auth | Notes |
| --- | --- | --- | --- | --- |
| `/health` | `GET` | keep | public | Liveness-style JSON plus redacted provider/attestation state. |
| `/livez` | `GET` | keep | public | Process liveness. |
| `/readyz` | `GET` | keep | public | Provider readiness gate. |

## OpenAI-Compatible Public API

OpenAI-compatible routes are mounted under `/api/v1` and also under `/v1` for
SDKs that expect the standard OpenAI base path. Endpoint allowlists treat the
two prefixes as equivalent.

| Route | Methods | Status | Scope/Auth | Notes |
| --- | --- | --- | --- | --- |
| `/api/v1/models` | `GET` | keep | `models.list` | Filters by API key allowed models. |
| `/api/v1/models/cache/invalidate` | `POST` | keep | `models.list` | Explicit runtime model cache invalidation. |
| `/api/v1/models/{model_id}` | `GET` | keep | `models.list` | Rejects disallowed models. |
| `/api/v1/chat/completions` | `POST` | keep | `chat.completions` | Supports non-streaming and SSE streaming proxy. |
| `/api/v1/embeddings` | `POST` | keep | `embeddings.create` | Proxies OpenAI-compatible embeddings. |
| `/api/v1/responses` | `POST` | keep | `responses` | OpenAI Responses API passthrough with non-streaming and SSE streaming responses, budget checks, model restrictions, and usage parsing. |
| `/api/v1/responses/{response_id}` | `GET`, `DELETE` | keep | `responses` | Retrieve or delete an upstream response. |
| `/api/v1/responses/{response_id}/input_items` | `GET` | keep | `responses` | Proxy documented response input-items list route. |
| `/api/v1/responses/{response_id}/cancel` | `POST` | keep | `responses` | Proxy documented response cancel route. |
| `/api/v1/responses/input_tokens` | `POST` | keep | `responses` | Proxy documented response input-token counting route. |
| `/api/v1/responses/compact` | `POST` | keep | `responses` | Proxy documented response compaction route. |
| `/api/v1/{openai_route}` | `GET`, `POST`, `PUT`, `PATCH`, `DELETE` | keep | `openai.proxy` | Generic passthrough for additional OpenAI API families such as legacy completions, assistants, files, uploads, audio, images, moderations, batches, fine-tuning, conversations, vector stores, evals, containers, skills, realtime client secrets, and deprecated Threads routes. Enclava-owned routes still use their typed handlers. |

## Provider And Attestation

| Route | Methods | Status | Scope/Auth | Notes |
| --- | --- | --- | --- | --- |
| `/api/v1/attestation/status` | `GET` | keep | `models.list` | Tinfoil status when configured. |
| `/api/v1/attestation/verify` | `POST` | keep | `models.list` | Explicit attestation verification. |
| `/api/v1/providers` | `GET`, `POST` | keep | `pricing.manage` | Provider registry list/create with runtime readiness context and non-secret config metadata. |
| `/api/v1/providers/{id}` | `GET`, `PUT`, `DELETE` | keep | `pricing.manage` | Provider registry detail/update/delete; active runtime provider remains env/config controlled. |

## Admin Platform API

| Route | Methods | Status | Scope/Auth | Notes |
| --- | --- | --- | --- | --- |
| `/api/v1/admin/api-keys` | `GET`, `POST` | keep | `platform.admin` | Current API key list/create. |
| `/api/v1/admin/api-keys/{id}` | `GET`, `PUT`, `DELETE` | keep | `platform.admin` | Current detail/update/soft-delete. |
| `/api/v1/admin/api-keys/{id}/regenerate` | `POST` | keep | `platform.admin` | Current secret rotation. |
| `/api/v1/admin/api-keys/{id}/restore` | `POST` | keep | `platform.admin` | Current soft-delete restore. |
| `/api/v1/admin/usage/summary` | `GET` | keep | `platform.admin` | Current summary endpoint. |
| `/api/v1/admin/pricing` | `GET`, `POST` | keep | `pricing.manage` | Provider/model pricing list/create with pricing audit snapshots. |
| `/api/v1/admin/billing-audit` | `GET` | keep | `platform.admin` | Billing audit list with API-key and subject filters. |

## Extract API

| Route | Methods | Status | Scope/Auth | Notes |
| --- | --- | --- | --- | --- |
| `/api/v1/extract/process` | `POST` | keep | `extract` | Multipart processing; accepts `template_id`, `template`, or `extract_template_id`. |
| `/api/v1/extract/jobs` | `GET` | keep | `extract` | API-key scoped job list. |
| `/api/v1/extract/jobs/{job_id}` | `GET` | keep | `extract` | API-key scoped job detail/result. |
| `/api/v1/extract/templates` | `GET`, `POST` | keep | `extract`; `extract.manage` for create | Template CRUD. |
| `/api/v1/extract/templates/{template_id}` | `GET`, `PUT`, `DELETE` | keep | `extract`; `extract.manage` for write | Template detail/update/delete. |
| `/api/v1/extract/templates/reset-defaults` | `POST` | keep | `extract.manage` | Default template reset. |
| `/api/v1/extract/models` | `GET` | keep | `extract` | Extract-capable model list. |
| `/api/v1/extract/settings` | `GET`, `PUT` | keep | `extract`; `extract.manage` for write | Extract settings. |
| `/api/v1/extract/health` | `GET` | keep | `extract` | Module/provider health. |

## RAG API

| Route | Methods | Status | Scope/Auth | Notes |
| --- | --- | --- | --- | --- |
| `/api/v1/rag/collections` | `GET`, `POST` | keep | `rag`, `rag.manage` | Collection list/create with ACL filtering. |
| `/api/v1/rag/collections/{collection_id}` | `GET`, `PUT`, `DELETE` | keep | `rag`, `rag.manage` | Collection detail/update/delete with owner/admin management checks. |
| `/api/v1/rag/collections/{collection_id}/documents` | `GET`, `POST` | keep | `rag`, `rag.manage` | Multipart upload/list documents. |
| `/api/v1/rag/documents/{document_id}` | `GET`, `DELETE` | keep | `rag`, `rag.manage` | Download original file and delete DB metadata plus Qdrant vectors. |
| `/api/v1/rag/documents/{document_id}/reprocess` | `POST` | keep | `rag.manage` | Re-extract, rechunk, re-embed, and replace Qdrant vectors. |
| `/api/v1/rag/search` | `POST` | keep | `rag` | Qdrant-backed vector search using provider embeddings. |

## Chatbot API

| Route | Methods | Status | Scope/Auth | Notes |
| --- | --- | --- | --- | --- |
| `/api/v1/prompt-templates` | `GET`, `POST` | keep | `chatbot`, `chatbot.manage` | Prompt template list/create; mutations require `chatbot.manage`. |
| `/api/v1/prompt-templates/{template_id}` | `GET`, `PUT`, `DELETE` | keep | `chatbot`, `chatbot.manage` | Prompt template detail/update/delete; mutations require `chatbot.manage`. |
| `/api/v1/chatbots` | `GET`, `POST` | keep | `chatbot`, `chatbot.manage` | Chatbot config list/create; mutations require `chatbot.manage`. |
| `/api/v1/chatbots/{chatbot_id}` | `GET`, `PUT`, `DELETE` | keep | `chatbot`, `chatbot.manage` | Chatbot config detail/update/delete; mutations require `chatbot.manage`. |
| `/api/v1/chatbot/external/{chatbot_id}/chat/completions` | `POST` | keep | `chatbot` | OpenAI-compatible chatbot endpoint with non-streaming and SSE streaming responses, prompt rendering, optional RAG context, conversation persistence, and usage attribution. |
| `/api/v1/chatbot/external/{chatbot_id}/v1/models` | `GET` | keep | `chatbot` | External model list for the configured chatbot model. |

## Connector API

| Route | Methods | Status | Scope/Auth | Notes |
| --- | --- | --- | --- | --- |
| `/api/v1/connectors/sources` | `GET`, `POST` | keep | `connectors.manage` | Connector source list/create for GitHub, Slack, Notion, and Linear source types. |
| `/api/v1/connectors/sources/{source_id}` | `GET`, `PUT`, `DELETE` | keep | `connectors.manage` | Source detail/update/delete. |
| `/api/v1/connectors/sources/{source_id}/sync` | `POST` | keep | `connectors.manage` | Manual sync into target RAG collection with fixture/static documents or API-backed GitHub, Slack, Notion, and Linear fetchers. Stores per-document hash checkpoints and Slack timestamp cursors to skip unchanged incremental runs. |
| `/api/v1/connectors/sync-jobs` | `GET` | keep | `connectors.manage` | Sync history with source/status filters. |
| `/api/v1/connectors/sync-jobs/{job_id}` | `GET` | keep | `connectors.manage` | Sync detail/errors/stats. |

## Go Admin Dashboard

| Route | Methods | Status | Scope/Auth | Notes |
| --- | --- | --- | --- | --- |
| `/dashboard` | `GET` | replace | dashboard session or API key fallback | Replaces primary operational frontend shell. |
| `/dashboard/` | `GET` | replace | dashboard session or API key fallback | Same as `/dashboard`. |
| `/dashboard/login` | `GET`, `POST` | replace | dashboard session | Password login when configured. |
| `/dashboard/logout` | `POST` | replace | dashboard session | Clears session. |
| `/dashboard/keys` | `GET` | replace | dashboard session or API key fallback | API key page. |
| `/dashboard/budgets` | `GET` | replace | dashboard session or API key fallback | Current budget page. |
| `/dashboard/analytics` | `GET` | replace | dashboard session or API key fallback | Current usage analytics. |
| `/dashboard/extract` | `GET` | replace | dashboard session or API key fallback | Current Extract page. |
| `/dashboard/partials/stats` | `GET` | replace | dashboard session or `platform.admin` | HTMX stats partial. |
| `/dashboard/partials/keys` | `GET`, `POST` | replace | dashboard session or `platform.admin` | HTMX API key workflows. |
| `/dashboard/partials/usage` | `GET` | replace | dashboard session or `platform.admin` | HTMX usage summary. |
| `/dashboard/partials/budgets` | `GET`, `POST` | replace | dashboard session or `platform.admin` | HTMX budget workflows. |
| `/dashboard/partials/extract` | `GET` | replace | dashboard session or `platform.admin` | HTMX Extract overview. |
| `/dashboard/partials/extract/templates` | `GET`, `POST` | replace | dashboard session or `platform.admin` | HTMX Extract template list/create. |
| `/dashboard/partials/extract/templates/{template_id}` | `PUT`, `DELETE` | replace | dashboard session or `platform.admin` | HTMX Extract template update/delete. |
| `/dashboard/partials/extract/reset-defaults` | `POST` | replace | dashboard session or `platform.admin` | HTMX default reset. |
| `/dashboard/partials/extract/settings` | `GET`, `PUT` | replace | dashboard session or `platform.admin` | HTMX Extract settings. |
| `/dashboard/partials/extract/process` | `POST` | replace | dashboard session or `platform.admin` | HTMX document processing. |
| `/dashboard/partials/extract/jobs` | `GET` | replace | dashboard session or `platform.admin` | HTMX job list. |
| `/dashboard/partials/extract/models` | `GET` | replace | dashboard session or `platform.admin` | HTMX model options. |
| `/dashboard/partials/providers` | `GET` | replace | dashboard session or `platform.admin` | HTMX provider readiness, models, and attestation. |
| `/dashboard/partials/pricing` | `GET` | replace | dashboard session or `platform.admin` | HTMX provider pricing and pricing audit. |
| `/dashboard/partials/billing-audit` | `GET` | replace | dashboard session or `platform.admin` | HTMX billing audit. |
| `/dashboard/partials/rag` | `GET`, `POST` | replace | dashboard session or `platform.admin` | HTMX RAG collection overview, collection create/delete, document list/upload/reprocess/delete. |
| `/dashboard/rag/documents/{document_id}/download` | `GET` | replace | dashboard session or `platform.admin` | Dashboard-authenticated document download. |
| `/dashboard/partials/chatbots` | `GET`, `POST` | replace | dashboard session or `platform.admin` | HTMX chatbot overview, endpoint snippets, chatbot create, enable/disable, and delete actions. |
| `/dashboard/partials/prompt-templates` | `GET`, `POST` | replace | dashboard session or `platform.admin` | HTMX prompt template overview and create. |
| `/dashboard/partials/connectors` | `GET`, `POST` | replace | dashboard session or `platform.admin` | HTMX connector source overview, source create, sync, enable/disable, and delete actions. |
| `/dashboard/static/*` | `GET` | replace | public static | Embedded dashboard assets. |
| `/dashboard/providers` | `GET` | replace | dashboard session or API key fallback | Provider readiness/model/attestation page. |
| `/dashboard/pricing` | `GET` | replace | dashboard session or API key fallback | Provider pricing page. |
| `/dashboard/billing-audit` | `GET` | replace | dashboard session or API key fallback | Billing audit page. |
| `/dashboard/rag` | `GET` | replace | dashboard session or API key fallback | RAG collection overview page. |
| `/dashboard/chatbots` | `GET` | replace | dashboard session or API key fallback | Chatbot overview page. |
| `/dashboard/prompt-templates` | `GET` | replace | dashboard session or API key fallback | Prompt template overview page. |
| `/dashboard/connectors` | `GET` | replace | dashboard session or API key fallback | Connector source and sync job overview page. |

## Dropped Plugin Routes

All plugin routes/pages from the Python/Next.js implementation are `drop` and
must not be included in Go parity requirements:

| Route Family | Status | Notes |
| --- | --- | --- |
| `/api/v1/plugins*` | drop | Registry, list, install, upload, config, instances, execution. |
| `/api-internal/v1/plugins*` | drop | Next.js plugin management compatibility. |
| `/plugins*` dashboard/pages | drop | Marketplace and plugin UI. |
| Plugin gateway routes | drop | Dynamic plugin gateway is out of scope. |
| Plugin cron/job routes | drop | Replaced by first-party connector sync jobs where needed. |
| Zammad plugin/module routes | drop | Reintroduce only as first-party connector/feature later. |
