---
phase: 01-application-and-inference
plan: "28"
subsystem: api
tags: [inference, executor, chatbot, assistant, billing, postgres]

requires:
  - phase: 01-application-and-inference
    provides: "single authorized priced inference executor (inference.Service) plus the real RAG conversation store it now consumes"
provides:
  - "Assistant chat completions dispatch only through inference.Service.Execute — policy, reservation, dispatch and settlement all live there"
  - "chatbot.Executor seam; PrepareChatCompletion produces an inference.Request (payload + source-object inputs + estimate); ExecutePrepared dispatches it"
  - "PersistAssistantMessage is the only retained assistant-side write; usage accounting is exclusively the executor's settlement record"
  - "postgres.ChatbotConversationStore adapter wiring retained conversations into inference.Deps.Conversations"
  - "httpx stream handler runs over the executor's accounting stream and propagates client disconnect by closing the owned body"
affects: [05-agents-and-approved-tools, phase-5 tool/run engine]

actuals:
  tokens: 0
  tasks: 2
  commits: 1

tech-stack:
  added: []
  patterns:
    - "Assistant model calls share the inference.Service executor — no separate provider or usage-recorder path remains"
    - "PreparedCompletion carries a fully-formed inference.Request (capability, model alias, endpoint pin, payload, stream flag, evidence inputs, estimate); the HTTP layer only dispatches and persists"
    - "Restricted RAG sources flow into Request.Inputs as source.ObjectRef and merge into the effective route restriction before dispatch"

key-files:
  created: []
  modified:
    - "internal/chatbot/service.go"
    - "internal/chatbot/service_test.go"
    - "internal/chatbot/types.go"
    - "internal/httpx/chatbot.go"
    - "internal/httpx/chatbot_test.go"
    - "internal/store/postgres/chatbot_repo.go"
    - "cmd/enclava/main.go"

key-decisions:
  - "PreparedCompletion now embeds inference.Request instead of a raw UpstreamRequest body — dispatch is executor-only by construction"
  - "chatbot.UsageRecord / UsageRecorder / RecordCompletion / RecordFailure removed entirely; the executor's settlement record is the single accounting artifact"
  - "RAG search hits become Request.Inputs (source.ObjectRef, deduplicated by DocumentID) so evidence restrictions gate routing before any upstream write"
  - "chatbot.Instance.ProviderID maps onto inference.Request.EndpointID — the legacy pin targets a persisted endpoint id only"

patterns-established:
  - "One execution path: assistant completions, streams and any later agent/tool work all inherit inference.Service.Execute"
  - "Postgres adapter type (ChatbotConversationStore) bridges tenant-owned rows to executor-visible identity without leaking internals"

requirements-completed:
  - "INF-02"
  - "AUTH-04"
  - "AUTH-06"
  - "JOB-03"
  - "JOB-04"

coverage:
  - id: D1
    description: "Assistant completions/streams execute only through the shared inference executor with caller principal and evidence restrictions"
    requirement: INF-02
    verification:
      - kind: unit
        ref: "go test ./internal/chatbot ./internal/httpx"
        status: pass
    human_judgment: false
  - id: D2
    description: "Restricted RAG source prevents routing to an incompatible endpoint before dispatch"
    requirement: AUTH-04
    verification:
      - kind: unit
        ref: "internal/chatbot/service_test.go#TestRestrictedRAGSourceDeniesIncompatibleEndpoint"
        status: pass
    human_judgment: false
  - id: D3
    description: "No assistant path can fabricate admin RAG access or charge independently; single authoritative settlement record"
    requirement: JOB-03
    verification:
      - kind: unit
        ref: "internal/chatbot/service_test.go#TestCreateChatCompletionInjectsRAGAndPersistsConversation"
        status: pass
    human_judgment: false
  - id: D4
    description: "Stream body stays caller-owned and closes on completion or client disconnect; settlement runs via executor accounting stream"
    requirement: JOB-04
    verification:
      - kind: unit
        ref: "internal/httpx/chatbot_test.go#TestExternalChatbotCompletionStreamUsesExecutor"
        status: pass
    human_judgment: false

duration: 0 min
completed: 2026-10-19
status: complete
---

# Phase 01 Plan 28: Remove assistant and HTTP model-execution bypasses

**Assistant chat completions and streams now dispatch exclusively through `inference.Service.Execute`; the dedicated provider/usage-recorder path is deleted.**

## Performance

- **Duration:** ~35 min
- **Tasks:** 2
- **Files modified:** 7
- **Commits:** 1 (`809f9ff`)

## Accomplishments

- `chatbot.Service` now depends on the shared `chatbot.Executor` seam (`*inference.Service`) instead of `llm.Provider` + `UsageRecorder`; `NewService(repo, promptRepo, ragSvc, executor)`.
- `PrepareChatCompletion` returns a `PreparedCompletion` carrying a fully formed `inference.Request` — capability `chat/completions`, the bot's model alias, `EndpointID` pinned from `ProviderID`, the rendered payload, the stream flag, deduplicated `source.ObjectRef` inputs from RAG results, and an input/output unit estimate.
- `CreateChatCompletion` executes the request, reads the bounded body, persists the assistant message, stamps chatbot/conversation metadata and returns the executor's settlement record; upstream errors propagate unrewritten.
- `ExecutePrepared` is the only dispatch helper; the HTTP stream handler streams the executor's accounting stream directly (no `prepared.UpstreamRequest` → provider reconstruction), propagates client disconnect via `ctx.Err()` and closes the owned body so settlement/liability run once.
- `PersistAssistantMessage` replaces `RecordCompletion`/`RecordFailure`/`usageRecord`/`recordUsage`/`chatbot.UsageRecord` — repository failures return real errors instead of fabricated success.
- RAG evidence now enters `Request.Inputs` as `source.ObjectRef`s, so the executor's input-resolution merges source restrictions and denies routing to an incompatible endpoint (new test proves a `RequiredAssurance: "high"` object never dispatches).
- `postgres.ChatbotConversationStore` adapts `ChatbotRepo` to `inference.ConversationStore` and is wired into `inference.Deps.Conversations` in `cmd/enclava/main.go`.
- Public error mapping extended: `inference.ErrDenied`/`ErrNoRoute` → 403, `ErrUnbounded`/`ErrUnsupportedCapability` → 400, real `llm.UpstreamError` still surfaces through `WriteProviderError`; SQL/credential details never reach the client.

## Verification

- `ENCLAVA_TEST_DATABASE_URL=postgres://enclava:enclava@localhost:5432/enclava go test -count=1 ./internal/chatbot ./internal/httpx ./cmd/enclava` — **PASS** (cmd/enclava has no test files; package builds)
- `go vet ./internal/... ./cmd/...` — clean
- `gofmt -l` on touched files — clean
- Full `go test ./...` — all packages pass except `internal/rag` Qdrant integration tests, which require `QDRANT_INTEGRATION_URL` (pre-existing external prerequisite, unrelated to this change)

## Deviations from Plan

- **[Rule 1 - auto-fix] `ProviderID` endpoint pin.** The plan said "remove … direct provider calls" but did not explicitly state where `ProviderID` went. Decision: map it to `Request.EndpointID` — a persisted endpoint id is a narrowing input, never a bypass.
- **[Rule 1 - auto-fix] `ExternalModels` no longer calls `provider.ListModels`.** The endpoint reports only the bot's configured alias (the caller-visible contract); a live catalog lookup was the last provider dependency and was removed rather than emulated.
- **[Rule 1 - auto-fix] Streaming bookkeeping simplified.** `applyChatbotStreamEvent`/`usageParsed` bookkeeping on the stream path was dropped because usage accounting moved into the executor's accounting stream; the handler still forwards SSE verbatim and persists the assistant message after the stream ends.

## Issues Encountered

- `ENCLAVA_TEST_DATABASE_URL` was unset in the environment; the established DSN `postgres://enclava:enclava@localhost:5432/enclava` (used by earlier phases) works against the running local PostgreSQL.
- `cmd/enclava` has no test files — the listed `go test` target exits 0 with `[no test files]`; coverage for the composition root is provided by `go build ./cmd/enclava` plus the package suites.

## Next Phase Readiness

Phase 5's bounded tool/run engine inherits this single execution path — `ExecutePrepared` and `inference.Request` are the extension points, and `Conversations` is now a live executor store.
