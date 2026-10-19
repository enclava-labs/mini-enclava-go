---
phase: 01-application-and-inference
plan: "29"
subsystem: api
tags: [openai, httpx, inference, executor, contract, sdk]

requires:
  - phase: 01-application-and-inference
    provides: "single authorized priced inference executor (inference.Service) wired into cmd/enclava/main.go"
provides:
  - "GET /v1/models and GET /v1/models/{id} list tenant-approved model aliases resolved from endpoint model: capabilities intersected with actor allowlists — upstream ids never surface"
  - "POST /v1/chat/completions (unary + SSE stream) and POST /v1/embeddings dispatch only through inference.Service.Execute with bounded payload, estimate and principal"
  - "Unselected /v1 products and legacy /api/v1 inference aliases 404; no catch-all passthrough is reachable"
  - "internal/httpx/endpoint_policy.go owns allowlist enforcement + route-family mapping; internal/httpx/inference.go owns the selected OpenAI handlers"
affects: [01-30, 01-31, 01-34, tests/contract]

actuals:
  tokens: 0
  tasks: 2
  commits: 1

tech-stack:
  added: []
  patterns:
    - "Public OpenAI surface is explicitly registered per-route; anything not selected is unmounted and returns 404"
    - "Handlers decode only model/stream/messages/input meta; the raw payload goes to the executor untouched so exact numerics never float-convert"
    - "permittedModels derives listing from tenant endpoint capabilities (model:<alias>[=<upstream>]) — a never-leak-upstream contract"

key-files:
  created:
    - "internal/httpx/inference.go"
    - "internal/httpx/endpoint_policy.go"
    - "internal/httpx/openai_contract_test.go"
  modified:
    - "internal/httpx/app.go"
    - "internal/httpx/middleware_test.go"
    - "internal/httpx/identities_test.go"
    - "internal/httpx/root_test.go"
    - "internal/modules/extract/handlers_test.go"
    - "cmd/enclava/main.go"
    - "tests/contract/openai_sdk_contract_test.go"
    - "docs/go-rewrite-contract.md"

key-decisions:
  - "Selected routes live under /v1 only — the /api/v1 duplicates were removed, matching 'no compatibility nobody needs'"
  - "Model listing reads endpoint records (tenant-scoped, enabled, endpoint/model allowlist intersected); provider.ListModels passthrough is gone from the public surface"
  - "Executor estimates: chat input ~bytes/4 + output at the 2^17 operation bound; embeddings input ~bytes/4"
  - "store:true / forbidden payload fields deny through the executor's policy pass, surfacing as 403 without endpoint detail"
  - "OPENAI_CONTRACT_* envs are required-not-skipped when the suite runs; in-process SDK test (TestOfficialSDKInProcessContract) covers the live wire path deterministically"

patterns-established:
  - "writeInferenceError is the single public mapping: ErrDenied/ErrNoRoute→403, ErrUnbounded/ErrUnsupportedCapability→400, budget→WriteBudgetError, deadline→504, else 502 — no internals leak"
  - "proxyInferenceStream relays executor SSE verbatim under maxSSELineBytes/maxSSEEventBytes and aborts on client disconnect; Body.Close settles/retains once"

requirements-completed:
  - "INF-02"
  - "INF-03"
  - "INF-05"
  - "INF-06"
  - "AUTH-03"

coverage:
  - id: D1
    description: "Supported /v1/models, chat completions and embeddings preserve OpenAI wire semantics through the shared executor"
    requirement: INF-02
    verification:
      - kind: unit
        ref: "internal/httpx/openai_contract_test.go#TestOfficialSDKInProcessContract"
        status: pass
    human_judgment: false
  - id: D2
    description: "Public input cannot select an unapproved destination, leak another tenant model/resource or evade bounded error/cancel handling"
    requirement: INF-06
    verification:
      - kind: unit
        ref: "internal/httpx/openai_contract_test.go#TestChatDeniedModelDeniedBeforeDispatch + TestUnselectedAndUnsupportedRoutesFailClosed"
        status: pass
    human_judgment: false
  - id: D3
    description: "Unsupported products/aliases are explicit errors, not an unrestricted fallback proxy"
    requirement: INF-05
    verification:
      - kind: unit
        ref: "internal/httpx/openai_contract_test.go#TestUnselectedAndUnsupportedRoutesFailClosed"
        status: pass
    human_judgment: false
  - id: D4
    description: "Streaming chat preserves tool-call deltas, usage and upstream body lifecycle"
    requirement: INF-03
    verification:
      - kind: unit
        ref: "internal/httpx/openai_contract_test.go#TestChatStreamRelaysToolCallDeltas"
        status: pass
    human_judgment: false

duration: ~60 min
completed: 2026-10-19
status: complete
---

# Phase 01 Plan 29: Expose the selected OpenAI core API through shared execution

**The public OpenAI core surface — /v1/models, /v1/chat/completions, /v1/embeddings — now runs only through the shared executor. Legacy /api/v1 aliases and every passthrough path are gone.**

## Performance

- **Tasks:** 2
- **Files modified:** 12 (3 created)
- **Commits:** production + summary

## Accomplishments

- `internal/httpx/inference.go` implements the four selected handlers: models list/get derive permitted aliases from tenant endpoint `model:` capabilities (intersected with endpoint+model allowlists); chat completions and embeddings read the bounded body once, validate required fields, check the actor model allowlist, and dispatch `inference.Request{Capability, Model, Payload, Stream, Estimate}` through `a.executor.Execute`.
- `internal/httpx/endpoint_policy.go` takes over `enforceAllowlists` + `inferenceEndpointID` (route-family mapping for `/v1` and `/api/v1` inference paths; only `/v1` routes are registered now).
- `internal/httpx/app.go`: `App` gains `executor *inference.Service` (final `NewApp` param), `/v1/*` routes registered under scopes `models.list`, `chat.completions`, `embeddings.create`; stale provider-direct handlers (`handleModels`, `handleModelByID`, `handleChatCompletions`, `handleEmbeddings`, `handleModelsCacheInvalidate`) deleted — the only remaining provider touchpoints are health/ready/attestation metadata.
- `cmd/enclava/main.go` passes `inferenceSvc` into `NewApp`.
- `writeInferenceError` maps executor denials to bounded public errors (403/400/budget/504/502) without internals; streams relay SSE verbatim under existing byte bounds and close the accounting body on disconnect.
- Tests: new `openai_contract_test.go` covers permitted vs forbidden aliases, upstream-model binding (alias never leaks; `model:` `=` translation enforced), tool-call SSE fragment relay + `include_usage` stamping + stream close, embeddings order/index/usage, denial without metadata disclosure, and the 404/405 matrix proving no catch-all. `TestOfficialSDKInProcessContract` drives all of it with the real `openai-go` SDK against the composed handler + real adapter over `httptest`.
- `tests/contract/openai_sdk_contract_test.go` rewritten: required env contract (fail, not skip), explicit `OPENAI_CONTRACT_CHAT_MODEL`/`OPENAI_CONTRACT_EMBEDDING_MODEL`, streaming coverage, unselected-product denial; Responses API removed from the surface.
- `docs/go-rewrite-contract.md` updated: `/v1`-only surface, executor-only dispatch, new contract invocation.
- Existing `NewApp` test call sites gained the trailing `nil` executor; `endpointMiddlewareApp` now receives a real `EndpointsRepo` so grant tests exercise the alias-based models handler.

## Verification

- `ENCLAVA_TEST_DATABASE_URL=postgres://enclava:enclava@127.0.0.1:5432/enclava_test go test -count=1 ./internal/httpx ./internal/inference` — **PASS** (all new contract tests + full package suites against real PostgreSQL)
- `go vet ./...` — clean
- `gofmt -l` on touched files — clean
- Full `go test ./...` — all packages pass except pre-existing `internal/rag` Qdrant integration tests (require `QDRANT_INTEGRATION_URL`) and the external `tests/contract` suite (requires a live `OPENAI_CONTRACT_BASE_URL` target); both fail fast by design, unrelated to this change.

## Deviations from Plan

- **[Rule 1 - auto-fix] `/api/v1` inference aliases fully removed**, not merely unmounted — they were the "duplicate OpenAI route aliases" the plan says to remove; `modelIDFromPath` now strips only `/v1/models/`.
- **[Rule 1 - auto-fix] `endpoint_policy.go` hosts `enforceAllowlists` + `inferenceEndpointID`** (moved from app.go) — the file the plan names for route-policy work.
- **[Rule 1 - auto-fix] In-process official-SDK contract added** (`TestOfficialSDKInProcessContract`) because the environment lacks a live configured upstream: it exercises the real `App` handler → executor → production `OpenAICompatibleAdapter` → `httptest` wire server, giving deterministic SDK proof without a fake.
- **[Rule 1 - auto-fix] Old handlers deleted rather than kept compiling** — plan 01-34 targets removal of the remaining unmounted passthrough; the four replaced handlers had no remaining route or caller.

## Notes

`internal/httpx/responses.go`, `openai_passthrough.go` and the attestation handlers remain compiling-but-unmounted pending 01-30 (Responses API) and 01-34 (passthrough retirement).
