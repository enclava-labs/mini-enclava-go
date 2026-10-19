---
phase: 01-application-and-inference
plan: "34"
subsystem: inference
tags: [llm, transport, passthrough, security]

requires:
  - phase: 01-33
    provides: Usage, exact pricing and budget screens
provides:
  - Removed generic passthrough interfaces, implementations, routing and credential-forwarding helpers.
  - Kept the selected explicit model, chat, embedding and Responses transports and fail-closed unsupported routes.
affects: [01-35, 01-36]
actuals:
  tokens: 7937
  tasks: 2
  commits: 2
tech-stack:
  added: []
  patterns:
    - Explicit transport operations only; unsupported products have no generic upstream dispatch path.
key-files:
  created: []
  modified:
    - internal/llm/provider.go
    - internal/llm/openai_adapter.go
    - internal/llm/tinfoil_provider.go
    - internal/llm/model_cache.go
    - internal/llm/openai_adapter_test.go
    - internal/httpx/openai_passthrough.go (deleted)
    - internal/httpx/app.go
    - internal/httpx/chatbot_test.go
key-decisions:
  - "Retain selected operation-specific transports and remove generic passthrough completely rather than preserving a renamed compatibility path."
  - "Existing HTTP contract tests assert rejected unsupported routes do not dispatch any upstream request."
requirements-completed: [CORE-05, INF-03, INF-04, INF-06]
coverage:
  - id: D1
    description: "Selected explicit model/chat/embedding/Responses transport implementations remain, including verified Tinfoil transport behavior."
    requirement: INF-03
    verification:
      - kind: unit
        ref: "go test -count=1 ./internal/llm"
        status: pass
    human_judgment: false
  - id: D2
    description: "Unsupported products and legacy routes fail closed without an upstream request."
    requirement: INF-06
    verification:
      - kind: integration
        ref: "internal/httpx TestUnselectedAndUnsupportedRoutesFailClosed; go test -count=1 ./internal/httpx"
        status: pass
    human_judgment: false
  - id: D3
    description: "Generic passthrough interfaces, implementations, registrations and helper paths are removed."
    requirement: CORE-05
    verification:
      - kind: other
        ref: "go build ./..."
        status: pass
    human_judgment: false
duration: not recorded
completed: 2026-10-19
status: complete
---

# Phase 01 Plan 34: Retire the unrestricted passthrough transport surface

**Generic OpenAI-product forwarding is deleted while the selected explicit transports and route-denial contracts remain.**

## Performance

- **Duration:** Not recorded
- **Started:** Not recorded
- **Completed:** 2026-10-19T17:57:00Z
- **Tasks:** 2
- **Files modified:** 8 (including one planned deletion)

## Accomplishments
- Removed generic passthrough provider API, adapter/client implementations, request routing and forwarding helpers; deleted the authorized `internal/httpx/openai_passthrough.go` target.
- Retained explicit operation transport methods, including Tinfoil verification, body/stream limits, cancellation and safe error handling.
- Removed passthrough-only tests while retaining consumer-visible unsupported-route coverage, which verifies rejected requests produce no upstream calls.

## Task Commits

1. **Task 1: Remove unused generic interfaces and transport methods with their callers** - `a69ece6` (refactor)
2. **Task 2: Remove obsolete passthrough contracts and retain real protocol regressions** - `2e008d3` (test)

**Plan metadata:** to be committed (docs: complete plan)

## Files Created/Modified
- `internal/llm/provider.go` - Removed passthrough interface and response type.
- `internal/llm/openai_adapter.go` - Removed catch-all request implementation and generic path/header/stream helpers.
- `internal/llm/tinfoil_provider.go` - Removed catch-all method without changing verified explicit operations.
- `internal/llm/model_cache.go` - Removed passthrough forwarding method.
- `internal/llm/openai_adapter_test.go` - Removed generic forwarding/helper tests; retained endpoint security, response bound, redirect, mutation replay and stream behavior tests.
- `internal/httpx/openai_passthrough.go` - Deleted as authorized in plan `files_deleted`.
- `internal/httpx/app.go` - Removed stale comment claiming handlers were retained for this plan.
- `internal/httpx/chatbot_test.go` - Corrected stale test-provider comment.

## Decisions Made
- No renamed alias or wrapper replaces the removed generic transport surface.
- Kept ordinary operation-specific helpers and the existing route behavior tests unchanged.

## Deviations from Plan

None - plan executed exactly as written. The only deleted file was the planned `internal/httpx/openai_passthrough.go`.

**Total deviations:** 0 auto-fixed. **Impact:** No scope change.

## Issues Encountered
- First verification attempt stopped because the shell did not inherit `ENCLAVA_TEST_DATABASE_URL`; reran the required HTTPX suite after exporting the supplied test DSN and it passed.
- Live configured inference endpoint smoke (core request plus owned Responses follow-up) was not exercised; only the required build and package test suites were run. No live upstream credentials/endpoints were part of this dispatch context.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- Passthrough retirement is complete; proceed to plan 01-35.
- Live configured-upstream smoke remains unverified and should not be inferred from deterministic package tests.

---
*Phase: 01-application-and-inference*
*Completed: 2026-10-19*
