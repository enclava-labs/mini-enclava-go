---
phase: 01-application-and-inference
plan: "15"
subsystem: prompts-chatbot
tags: [prompts, chatbot, postgres, auth-principal]
status: complete
requirements: [CORE-05, AUTH-04, AUTH-05]

requires:
  - phase: 01-application-and-inference (plan 01-11/01-14)
    provides: postgres domain 006 tables (prompt_templates, chatbots,
      conversations, messages), auth.Principal context, ported
      rag.Service/connectors.Service surfaces
provides:
  - prompts.Template + chatbot.Instance/Conversation/Message/UsageRecord
    tenant-scoped domain types — zero sqlite.* references remain in either
    service package or their httpx handlers
  - postgres.PromptsRepo (prompt_templates) and postgres.ChatbotRepo
    (chatbots/conversations/messages), every statement tenant-pinned;
    missing, malformed-tenant and cross-tenant lookups all read sql.ErrNoRows
  - prompts.Service and chatbot.Service taking auth.Principal on every
    method; visibility derives from AllowedChatbots/AllowedAgents allowlists
    (empty denies; only explicit all-mode is unrestricted)
  - prompts visibility delegation: chatbot.Service implements
    prompts.VisibleTemplateIDs; prompt list/get filter to templates attached
    to visible chatbots, preserving the previous handler contract
  - httpx prompts/chatbot handlers and dashboard resource sections
    (chatbots, prompt-templates, connectors, rag) on the real principal —
    chatbot.Access, chatbotVisible, chatbotAccess and handler-side
    APIKeyID maps deleted; dashboard chatbots/prompt-templates/actions and
    pricing/billing-audit views no longer reference sqlite types
  - main.go wired to postgres.NewPromptsRepo/NewChatbotRepo; completion
    usage observability bridges to the legacy usage repo via a 20-line
    legacyUsageRecorder adapter until the billing port (01-17)

affects: [01-17 billing/pricing dashboard + usage repo port, 01-22 sqlite removal]

actuals:
  tokens: ~
  tasks: 2
  commits: 2

tech-stack:
  added: []
  patterns:
    - "Conversation resume is owner-or-admin: conversations.owner_actor_id matched against principal ActorID (legacy apikey:<id> fallback), platform.admin scope resumes any tenant conversation"
    - "Per-tenant prompt defaults: PromptsRepo.EnsureDefaults(tenant, templates) is idempotent; chatbot falls back to the built-in template when a tenant has none seeded"
    - "Chatbot RAG retrieval passes the request principal to rag.Search verbatim — no context lookup, no synthesized identity"

key-files:
  created:
    - internal/store/postgres/prompts_repo.go
    - internal/store/postgres/chatbot_repo.go
    - internal/store/postgres/prompts_chatbot_repo_test.go
  modified:
    - internal/prompts/{service,types,defaults,service_test}.go
    - internal/chatbot/{service,types,service_test}.go
    - internal/httpx/{prompts,chatbot,chatbot_test,dashboard_resources}.go
    - cmd/enclava/main.go
  deleted: []

key-decisions:
  - "chatbot.Access/APIKeyID-keyed visibility deleted entirely; auth.ChatbotAllowed + auth.AgentAllowed cover the dual-allowlist contract"
  - "Message rows carry no token columns (schema has none); token accounting stays on the usage record view"
  - "Dashboard pricing/billing-audit render over local view structs identical to the legacy sqlite rows — the repos themselves port in 01-17"
  - "Missing/foreign ids uniformly return sql.ErrNoRows from services so unknown and forbidden share the public outcome"

requirements-completed: [CORE-05, AUTH-04, AUTH-05]

coverage:
  - id: D1
    description: "Prompts/chatbot tenant-scoped CRUD on postgres with principal plumbing"
    requirement: CORE-05
    verification:
      - kind: integration
        ref: "internal/prompts/service_test.go#TestServiceCRUDTenantScoped, internal/chatbot/service_test.go#TestServiceTenantIsolation, internal/store/postgres/prompts_chatbot_repo_test.go#TestPromptsChatbotReposTenantScoped"
        status: pass
    human_judgment: false
  - id: D2
    description: "Allowlist visibility: empty denies, explicit all-mode allows; chatbots+agents allowlists honored; same-outcome 404s"
    requirement: AUTH-04
    verification:
      - kind: integration
        ref: "internal/chatbot/service_test.go#TestPrepareChatCompletionEnforcesAllowedChatbots, internal/httpx/chatbot_test.go#TestChatbotAndPromptReadsHonorAllowlist"
        status: pass
    human_judgment: false
  - id: D3
    description: "Conversation resume owner-or-admin; cross-tenant and cross-actor deny as ErrNoRows; RAG retrieval passes real principal without fabricated admin"
    requirement: AUTH-05
    verification:
      - kind: integration
        ref: "internal/chatbot/service_test.go#TestResumeConversationRequiresOwnerOrAdmin, TestRetrieveRAGUsesBotAttachmentAccess"
        status: pass
    human_judgment: false
  - id: D4
    description: "Provider errors never leak storage/internal details; usage recorded on success and failure"
    verification:
      - kind: integration
        ref: "internal/httpx/chatbot_test.go#TestExternalChatbotCompletionHidesServiceErrors, TestWriteChatbotCompletionErrorClassifiesFailures"
        status: pass
    human_judgment: false
---

# 01-15 Summary — prompts + chatbot on postgres, principals, and ported callers

**Both consumer services now run tenant-scoped on the 006 domain tables with `auth.Principal` end-to-end; every caller (handlers, dashboard resources, main) compiles in one boundary.**

## Accomplishments
- `postgres.PromptsRepo` / `postgres.ChatbotRepo`: tenant-pinned CRUD over prompt_templates, chatbots, conversations, messages; all not-found/cross-tenant/malformed-tenant paths collapse to `sql.ErrNoRows`
- `prompts.Service`: `Template` domain type replaces `sqlite.PromptTemplate`; every method takes `auth.Principal`; list/get visibility delegates to `chatbot.Service.VisiblePromptTemplateIDs`
- `chatbot.Service`: `Instance`/`Conversation`/`Message`/`UsageRecord` domain types; `chatbot.Access` deleted — allowlist checks go through `auth.ChatbotAllowed`/`auth.AgentAllowed`; conversation resume is owner-or-admin via `owner_actor_id`; RAG retrieval passes the request principal to `rag.Search` (no fabricated identity)
- httpx: prompts.go and chatbot.go pass `principal` to all services; `chatbotAccess`, `chatbotVisible`, `chatbotAllowlistOpen`, `visiblePromptTemplates`, `promptTemplateVisible`, `allowedPromptTemplateIDs` removed
- dashboard_resources: chatbot/prompt-template data structs on `chatbot.Instance`/`prompts.Template`; both action handlers take the extracted principal; pricing/billing-audit rows mapped into local view structs so no `sqlite.` reference remains
- main.go: `postgres.NewPromptsRepo`/`NewChatbotRepo` wired; prompt seed moved to per-tenant `EnsureDefaults` (callable at first use); `legacyUsageRecorder` bridges completion usage rows to the legacy sqlite usage stats repo until 01-17
- Tests rebuilt on `testdb` + real services + fake provider/searcher/usage recorder; no sqlite fixtures, no skipped cases

## Task Commits
1. `feat(01-15): prompts + chatbot services on postgres repos and principals`
2. `feat(01-15): port prompts/chatbot callers — handlers, dashboard resources, main wiring`

## Verification
- `go build ./...` ✓, `go vet ./...` ✓
- `ENCLAVA_TEST_DATABASE_URL=postgres://enclava:enclava@localhost:5432/enclava go test ./internal/prompts/ ./internal/chatbot/ ./internal/httpx/ ./internal/store/postgres/ -count=1` → all pass
- `grep -rn 'sqlite\.\|IsAdmin' internal/httpx/prompts.go internal/httpx/chatbot.go internal/httpx/dashboard_resources.go internal/prompts internal/chatbot` → empty

## Deviations
- `prompts.NewService` takes a `VisibleTemplateIDs` arg (implemented by `chatbot.Service`) so visibility lives in the service layer without a package cycle; `nil` keeps restricted principals denied
- `chatbot.UsageRecord` is a minimal observability view (no pricing columns); `main.go` adapts it to `sqlite.UsageRepo` until the billing port
- `sqlite.PromptTemplatesRepo`/`ChatbotRepo` code remains in the repo but is unreferenced by production wiring; 01-22 removes the package
- `Message` drops legacy token/metadata columns — the 006 messages table carries role/content/refs/restrictions only; token accounting lives on `UsageRecord`
