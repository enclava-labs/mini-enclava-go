# Enclava Go Rewrite Implementation Plan

## Purpose

This plan describes how to expand `mini-enclava-go` into the primary Enclava
implementation while intentionally dropping the Python plugin subsystem. The
goal is not a line-for-line port of the FastAPI and Next.js application. The
goal is a smaller, maintainable Go appliance that preserves the important
product contracts: confidential LLM access, OpenAI-compatible APIs, API key
management, usage and budgets, Extract, RAG, chatbots, connectors, and an admin
dashboard.

## Scope

### Keep

- OpenAI-compatible public API:
  - `GET /api/v1/models`
  - `GET /api/v1/models/{model_id}`
  - `POST /api/v1/chat/completions`
  - `POST /api/v1/embeddings`
  - `POST /api/v1/responses` and documented Responses subroutes
  - SDK-compatible `/v1/...` aliases for the public OpenAI route family
  - Authenticated passthrough for additional OpenAI API families outside the
    typed local handlers, including files, uploads, audio, images,
    moderations, batches, fine-tuning, conversations, vector stores, realtime,
    Assistants/Threads, and legacy/deprecated compatibility routes where
    supported upstream.
- Provider abstraction for OpenAI-compatible upstreams, PrivateMode, Tinfoil,
  and RedPill-style confidential providers.
- API key authentication with scopes, endpoint restrictions, model restrictions,
  source IP restrictions, expiration, activation, regeneration, and soft delete.
- Usage recording, model pricing, budget enforcement, and billing audit.
- Extract document processing, templates, settings, job history, and usage
  attribution.
- RAG collections, document upload, document processing, Qdrant indexing, Qdrant
  search, ACLs, and connector-ingested sources.
- Chatbot configuration, prompt templates, external chatbot endpoints, and
  conversation history.
- Connectors for GitHub, Slack, Notion, and Linear.
- Admin dashboard for operational workflows.
- Data migration from the Python/Postgres application where practical.

### Drop

- Plugin registry APIs.
- Plugin marketplace discovery and installation.
- Plugin upload endpoints.
- Plugin configuration, plugin instances, plugin audit logs, plugin cron jobs,
  and plugin gateway tables.
- Python plugin sandboxing and plugin execution.
- Dynamic module hot reload as a product feature.
- Zammad plugin/module support unless it is reintroduced later as a normal
  connector or first-party Go feature.

## Target Architecture

Use `mini-enclava-go` as the implementation base.

```text
cmd/enclava/
internal/config/
internal/server/
internal/httpx/
internal/auth/
internal/store/sqlite/
internal/store/migrations/
internal/llm/
internal/providers/
internal/billing/
internal/usage/
internal/budgets/
internal/extract/
internal/rag/
internal/chatbot/
internal/prompts/
internal/connectors/
internal/adminui/
internal/jobs/
internal/audit/
```

Default deployment should be one Go binary plus SQLite and Qdrant. Keep Qdrant
as the vector store. Do not require Redis unless a later multi-node deployment
needs distributed cache or rate limiting. Prefer embedded admin assets and
server-rendered UI with HTMX over keeping the large Next.js frontend long term.

## Implementation Phases

### Phase 0: Contract And Scope Freeze

Objective: define exactly what the Go application must preserve before
implementation starts.

Tasks:

- Build a route inventory from the Python app and classify each route as
  `keep`, `replace`, `defer`, or `drop`.
- Explicitly mark every plugin route and plugin UI page as `drop`.
- Define whether `/api-internal/v1` remains temporarily for frontend
  compatibility or is replaced immediately by Go dashboard routes.
- Capture external API contracts for OpenAI-compatible chat, models, embeddings,
  Extract, API key management, usage, budgets, and RAG.
- Convert the most important Python tests into black-box contract tests that can
  run against both the Python app and the Go app.

Deliverables:

- `docs/go-rewrite-contract.md`
- `docs/go-rewrite-route-matrix.md`
- Contract test harness for public API compatibility.

Acceptance gates:

- Every Python route has a disposition.
- Plugin subsystem is fully excluded from future parity requirements.
- Contract tests run against the current Python app before Go parity work.

### Phase 1: Harden The Go Core

Objective: turn the current Go prototype into a durable application shell.

Tasks:

- Keep or replace `http.ServeMux` with `chi`; if route complexity grows, prefer
  `chi` for path parameters and middleware composition.
- Add request IDs and structured logging.
- Add log redaction for API keys, JWTs, auth headers, provider credentials, and
  user-provided document content.
- Add panic recovery with safe error responses.
- Add CORS configuration.
- Add secure response headers.
- Add request body limits and per-route upload limits.
- Preserve `/health`, `/livez`, and `/readyz`.
- Add a consistent OpenAI-compatible error writer for public API failures.
- Add config validation for production secrets, bootstrap keys, provider
  credentials, and timeout settings.
- Keep graceful shutdown behavior.

Deliverables:

- `internal/server` or hardened `internal/httpx` middleware stack.
- Config validation tests.
- Core route smoke tests.

Acceptance gates:

- `go test ./...` passes.
- Invalid production config fails at startup.
- API key and provider secrets are redacted from logs.
- Public API errors do not echo unsafe user input.

### Phase 2: Database Schema Expansion

Objective: add the durable data model needed for the kept Enclava features.

Tasks:

- Keep SQLite as the default database.
- Continue using embedded SQL migrations.
- Add or expand these tables:
  - `users`
  - `roles`
  - `dashboard_sessions` if needed
  - `api_keys`
  - `usage_records`
  - `provider_pricing`
  - `pricing_audit_log`
  - `billing_audit_log`
  - `budgets`
  - `rag_collections`
  - `rag_documents`
  - `prompt_templates`
  - `chatbot_instances`
  - `chatbot_conversations`
  - `chatbot_messages`
  - `connector_sources`
  - `connector_sync_jobs`
  - `responses` and `conversations` only if Responses API is kept
- Do not add plugin tables.
- Add migration tests for empty DB and upgrade from the current
  `mini-enclava-go` schema.

Deliverables:

- New migrations under `internal/store/sqlite/migrations`.
- Repository packages for new tables.
- Schema documentation.

Acceptance gates:

- Empty DB migrates successfully.
- Current mini DB migrates successfully.
- Foreign keys and key indexes are present for auth, usage, RAG, and connector
  hot paths.

### Phase 3: Auth, API Keys, Budgets, And Usage

Objective: finish the platform control plane before adding more product
features.

Tasks:

- Keep API-key-first authentication for public API routes.
- Keep admin dashboard session auth for operator UI.
- Add users and roles only where needed for admin management parity.
- Support API key scopes:
  - `models.list`
  - `chat.completions`
  - `embeddings.create`
  - `responses`
  - `extract`
  - `extract.manage`
  - `rag`
  - `rag.manage`
  - `chatbot`
  - `chatbot.manage`
  - `connectors.manage`
  - `pricing.manage`
  - `platform.admin`
- Implement allowed models, endpoints, IPs, chatbots, agents, and extract
  templates.
- Implement budget checks before provider calls.
- Implement post-call usage recording with actual token counts.
- Store pricing snapshots in usage records.
- Add billing audit events for API key, budget, and pricing changes.

Deliverables:

- Expanded `internal/auth`.
- `internal/budgets`.
- `internal/usage`.
- `internal/billing`.
- Admin API key and usage endpoints.

Acceptance gates:

- API key lifecycle tests pass.
- Endpoint, model, IP, and scope restriction tests pass.
- Budget exceeded requests fail before provider calls.
- Usage totals are updated after successful provider calls.
- Usage is recorded for failed provider calls where useful for audit.

### Phase 4: Provider Layer

Objective: make provider routing, health, attestation, and model metadata a
first-class Go subsystem.

Tasks:

- Define a provider interface for:
  - list models
  - get model
  - chat completion
  - streaming chat completion
  - embeddings
  - health
  - attestation status
  - attestation verification
- Keep the existing OpenAI-compatible adapter.
- Keep the existing Tinfoil provider.
- Add PrivateMode configuration using OpenAI-compatible upstream behavior where
  possible.
- Add RedPill/OpenAI-compatible confidential provider support.
- Add model metadata normalization:
  - provider ID
  - model ID
  - context length
  - input modalities
  - output modalities
  - streaming support
  - function calling support
  - vision/document support
- Add a model cache with explicit invalidation.
- Track provider health and readiness.
- Parse final streaming usage chunks when providers emit usage.

Deliverables:

- `internal/providers`.
- Provider health endpoints.
- Attestation endpoints.
- Model cache implementation.

Acceptance gates:

- Mocked provider tests cover normal, streaming, timeout, and upstream error
  behavior.
- `/readyz` reflects provider readiness.
- Model filtering works with API key allowed models.
- Streaming does not use a normal request timeout that kills long responses.

### Phase 5: OpenAI-Compatible API Parity

Objective: complete the core external API contract.

Tasks:

- Implement:
  - `GET /api/v1/models`
  - `GET /api/v1/models/{model_id}`
  - `POST /api/v1/chat/completions`
  - `POST /api/v1/embeddings`
  - `POST /api/v1/responses` and documented Responses subroutes
  - equivalent `/v1/...` aliases for OpenAI SDK compatibility
- Proxy other OpenAI API route families through the configured upstream with
  preserved method, query string, body, selected OpenAI headers, status code,
  response headers, binary bodies, SSE streams, and request tracing headers such
  as `X-Client-Request-Id`.
- Support streaming SSE for chat completions.
- Preserve OpenAI-style auth errors and request errors.
- Apply endpoint, scope, model, IP, and budget restrictions.
- Record usage and pricing snapshots.
- Support common OpenAI request fields:
  - `model`
  - `messages`
  - `temperature`
  - `max_tokens`
  - `top_p`
  - `frequency_penalty`
  - `presence_penalty`
  - `stop`
  - `stream`
  - `response_format`
  - `tools`
  - `tool_choice`
- Keep Responses API as an OpenAI-compatible passthrough, including streaming
  and usage parsing where providers emit usage.

Deliverables:

- Public API handlers.
- OpenAI client compatibility tests.

Acceptance gates:

- Official OpenAI client can call the Go server with a custom base URL.
- Non-streaming and streaming chat tests pass.
- Embedding tests pass.
- Responses API tests pass.
- Error shape tests pass.
- Budget and usage integration tests pass.

### Phase 6: Extract Parity

Objective: make the existing Go Extract implementation production-ready.

Tasks:

- Keep the current Extract service as the base.
- Add `output_schema` support to templates.
- Keep `context_schema`.
- Accept Python-compatible multipart fields:
  - `template`
  - `template_id`
  - `context`
  - `buyer_context`
- Add template wizard only if it remains part of the desired product.
- Add richer job detail responses.
- Add result retrieval and result validation details.
- Add model selection using provider model capability metadata.
- Add usage and pricing integration.
- Add dashboard pages for templates, settings, document processing, and job
  history.

Deliverables:

- Expanded `internal/extract`.
- Extract dashboard pages.
- Extract API contract tests.

Acceptance gates:

- Multipart upload tests pass.
- Template CRUD tests pass.
- Disallowed template/model tests pass.
- Provider-mocked extraction tests pass.
- Extract usage records contain model, tokens, endpoint, and cost.

### Phase 7: RAG

Objective: port RAG as a Go service backed by Qdrant, not as a dynamic module.

Tasks:

- Add Qdrant client abstraction.
- Implement collection CRUD.
- Implement collection ACLs.
- Implement document upload.
- Implement document metadata persistence.
- Implement document download.
- Implement document delete and Qdrant vector cleanup.
- Implement reprocessing.
- Implement text extraction for:
  - plain text
  - Markdown
  - CSV
  - JSON/JSONL
  - HTML
  - DOCX
  - PDF
- Implement chunking with overlap.
- Generate embeddings through the provider `/embeddings` endpoint first.
- Store vectors in Qdrant with document and chunk metadata.
- Implement vector search.
- Implement a RAG search tool function for chatbot/agent use.
- Avoid local Python ML dependencies in the Go process. If local embeddings are
  required later, use ONNX or a sidecar service.

Deliverables:

- `internal/rag`.
- Qdrant integration tests.
- RAG dashboard pages.

Acceptance gates:

- Upload-to-search integration test passes.
- Delete removes DB metadata and Qdrant vectors.
- ACL tests prevent unauthorized collection access.
- Connector-ingested documents appear in search.

### Phase 8: Chatbots And Prompt Templates

Objective: rebuild the chatbot product on top of the Go provider and RAG
services.

Tasks:

- Implement prompt template CRUD.
- Seed default prompt templates.
- Implement chatbot config CRUD.
- Support bound RAG collections.
- Support provider and model preferences.
- Implement external chatbot OpenAI-compatible endpoint:
  - `POST /api/v1/chatbot/external/{chatbot_id}/chat/completions`
  - `GET /api/v1/chatbot/external/{chatbot_id}/v1/models`
- Store conversations and messages.
- Attribute usage by chatbot ID.
- Add dashboard pages for chatbot management and integration snippets.

Deliverables:

- `internal/prompts`.
- `internal/chatbot`.
- Chatbot dashboard pages.

Acceptance gates:

- Chatbot can be created, updated, disabled, and deleted.
- External endpoint works with API key auth.
- Chatbot RAG retrieval affects prompt context.
- Conversations and usage records are persisted.

### Phase 9: Connectors

Objective: replace plugin-based extensibility with first-party connector sync.

Tasks:

- Implement connector source CRUD.
- Implement encrypted credential storage or a documented local secret strategy.
- Implement connector sync jobs.
- Implement manual sync.
- Implement scheduled sync.
- Implement checkpoint support.
- Port GitHub connector behavior.
- Port Slack connector behavior.
- Port Notion connector behavior.
- Port Linear connector behavior.
- Feed connector documents into RAG collections.
- Store sync errors and per-run stats.

Deliverables:

- `internal/connectors`.
- `internal/jobs`.
- Connector dashboard pages.

Acceptance gates:

- Each connector has mocked API tests.
- Each connector has at least one fixture-based sync test.
- Sync updates RAG document metadata.
- Incremental sync respects checkpoints.

### Phase 10: Admin UI

Objective: replace the operational parts of the Next.js frontend with a Go
admin dashboard.

Tasks:

- Keep the existing mini dashboard direction: server-rendered pages plus HTMX.
- Add pages:
  - dashboard overview
  - API keys
  - usage
  - budgets
  - providers
  - pricing
  - billing audit
  - Extract
  - RAG collections
  - RAG documents
  - chatbots
  - prompt templates
  - connectors
- Keep pages dense and operational.
- Avoid rebuilding dropped plugin UI.
- Add Playwright smoke tests.

Deliverables:

- `internal/adminui`.
- Embedded static assets.
- Dashboard smoke test suite.

Acceptance gates:

- Login works.
- API key create/regenerate/delete flows work.
- Provider status page loads.
- Extract processing flow works.
- RAG collection and upload flow works.
- Chatbot create flow works.
- Connector create and sync flow works with mocked connectors.

### Phase 11: Migration And Cutover

Objective: move useful data from the Python/Postgres system into the Go system
without carrying forward plugin state.

Tasks:

- Build a Postgres export script from the Python app.
- Export JSONL files for:
  - users
  - roles
  - API keys metadata
  - budgets
  - provider pricing
  - usage records
  - prompt templates
  - chatbots
  - chatbot conversations
  - RAG collections
  - RAG documents metadata
  - connector sources
- Do not export plugin tables.
- Build `enclava migrate-import`.
- Preserve Qdrant collection names where possible.
- Validate row counts and sampled records.
- Provide dry-run mode.

Deliverables:

- `cmd/enclava migrate-import`.
- Migration guide.
- Dry-run report.

Acceptance gates:

- Dry-run import completes without writes.
- Real import maps expected row counts.
- A migrated API key can authenticate.
- A migrated chatbot can be called.
- A migrated RAG collection can be searched.

## Recommended Execution Order

1. Phase 0: contract and route matrix.
2. Phase 1: Go core hardening.
3. Phase 2: database schema expansion.
4. Phase 3: auth, API keys, budgets, usage, billing.
5. Phase 4: provider layer.
6. Phase 5: OpenAI-compatible API parity.
7. Phase 6: Extract parity.
8. Phase 10 subset: dashboard for API keys, usage, providers, Extract.
9. Phase 7: RAG.
10. Phase 8: chatbots and prompt templates.
11. Phase 9: connectors.
12. Remaining Phase 10 dashboard pages.
13. Phase 11: migration and cutover.

This order gets the core API stable before moving higher-level workflows.

## Testing Strategy

Use four layers of tests:

- Unit tests for auth, budget math, pricing math, provider adapters, request
  validation, and repository behavior.
- Integration tests with SQLite and Qdrant.
- Contract tests for public HTTP APIs.
- Playwright tests for admin dashboard workflows.

Minimum launch test suite:

- `go test ./...`
- OpenAI client compatibility test.
- API key lifecycle test.
- Budget exceeded test.
- Usage recording test.
- Extract process test with mocked provider.
- RAG upload-to-search test with Qdrant.
- Chatbot external endpoint test.
- Connector sync fixture tests.
- Dashboard smoke tests.

## Major Risks

- RAG is the largest technical port because Python currently relies on document
  processing and embedding libraries that do not map cleanly to Go.
- The old Next.js frontend is tightly coupled to `/api-internal/v1`. Either keep
  compatibility temporarily or replace workflows with Go dashboard pages as they
  are implemented.
- Streaming usage accounting depends on provider response behavior. Some
  providers may not emit final usage in streams.
- Data migration must avoid copying plugin-era assumptions into the new schema.
- Connector API behavior can be rate limited and inconsistent; fixture tests are
  mandatory.

## Out Of Scope For The Rewrite

- Plugin install/upload/marketplace.
- Python plugin sandbox.
- Dynamic module reload as a user-facing capability.
- Redis-backed distributed rate limiting.
- Multi-node deployment.
- Local in-process Python ML.
- Stateful local Responses storage beyond upstream passthrough.

## First Implementation Milestone

The first concrete milestone should be:

1. Add `docs/go-rewrite-contract.md`.
2. Add `docs/go-rewrite-route-matrix.md`.
3. Harden core middleware and config validation.
4. Expand SQLite schema for auth, usage, pricing, budgets, RAG, chatbots, and
   connectors.
5. Keep `go test ./...` green after every step.

This milestone creates the foundation needed for all later feature work without
starting with the hardest product surfaces.
