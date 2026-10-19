---
phase: 01-application-and-inference
plan: "35"
subsystem: auth
tags: [go, auth, principal, billing]
requires:
  - phase: 01-34
    provides: Responses and inference consumers migrated to owned executor
provides:
  - Principal no longer exposes legacy numeric key, name, budget, or retained-marker fields
  - HTTP legacy usage recorder and duplicate budget enforcement removed; shared billing display/error helpers retained
  - Chatbot ownership uses canonical ActorID only
 affects: [authentication, inference, chatbot, billing dashboard]
actuals:
  tokens: 300
  tasks: 2
  commits: 2
  plan_head_before: 4f0850945a5a61cdf7a821b879e6db27bf887e2a
  plan_head_after: fb1355b4c092a93f6f3ce8481c39e81ec2e978f5
tech-stack:
  added: []
  patterns: [canonical ActorID ownership, authoritative billing admission]
key-files:
  created: []
  modified:
    - internal/auth/principal.go
    - internal/auth/service.go
    - internal/httpx/app.go
    - internal/httpx/budgets.go
    - internal/httpx/platform_api.go
    - internal/httpx/chatbot.go
    - internal/chatbot/service.go
    - internal/chatbot/service_test.go
    - internal/httpx/budgets_test.go
    - internal/httpx/chatbot_test.go
key-decisions:
  - "Keep the typed SQLite usage-repository slot through plan 01-36 as directed; remove its original RecordUsage helper now."
  - "Retain shared budget liability, display, and error helpers used by dashboard/RAG while deleting the duplicate pre-inference enforcement path."
patterns-established: []
requirements-completed: [AUTH-04, CORE-05, INF-03]
coverage:
  - id: D1
    description: "Legacy Principal authority/accounting fields and HTTP RecordUsage/EnforceBudget paths removed; shared executor admission remains authoritative."
    requirement: AUTH-04
    verification:
      - kind: automated
        ref: "go build ./..."
        status: pass
    human_judgment: false
  - id: D2
    description: "Authentication, chatbot ownership, inference authorization and responses contracts remain covered after the Principal cutover."
    requirement: CORE-05
    verification:
      - kind: integration
        ref: "go test -count=1 $(go list ./... | grep -v '/tests/contract')"
        status: fail
    human_judgment: true
    rationale: "The suite reached a pre-existing randomized item-ID assertion failure in TestResponsesChainedFollowUpPinsOrigin; integration behavior was not fully green."
# Metrics
duration: 14min
completed: 2026-10-19
status: complete
---

# Phase 01 Plan 35: Remove the last legacy authority and accounting fields

**Canonical authenticated principals and shared billing admission replace the final HTTP legacy authority/accounting paths.**

## Performance

- **Duration:** 14 min
- **Started:** 2026-10-19
- **Completed:** 2026-10-19
- **Tasks:** 2/2
- **Files modified:** 10

## Accomplishments
- Removed legacy fields from `auth.Principal`, stopped populating them in key authentication, and removed the numeric `LegacyAPIKeyID` bridge.
- Removed duplicate HTTP budget enforcement and usage recording while retaining shared billing helpers used by dashboard and RAG paths.
- Removed chatbot ownership fallback to a numeric legacy key ID and obsolete test assertions.

## Task Commits
1. **Task 1: Remove legacy principal and HTTP accounting consumers together** — `2ab80cb` (`refactor(01-35)`)
2. **Task 2: Keep behavioral identity regressions and remove obsolete field assertions** — `fb1355b` (`test(01-35)`)

## Verification
- `go build ./...` — PASS.
- `go test -count=1 $(go list ./... | grep -v '/tests/contract')` — FAIL: `TestResponsesChainedFollowUpPinsOrigin` reported `item id must be a bound public id` for a generated `item_...` identifier containing `UP`. Other packages reported PASS; HTTP suite did not pass completely.

## Deviations from Plan

- **[Rule 3 - Missing caller] Chatbot principal consumer migration** — Found during Task 1 | `chatbot/service.go` still fell back to removed numeric `APIKeyID`; switched ownership to `ActorID` and updated its service tests. Files: `internal/chatbot/service.go`, `internal/chatbot/service_test.go`. Verification: build passed; chatbot tests passed in requested suite. Commit: `2ab80cb`, `fb1355b`.
- **[Rule 3 - Missing caller] HTTP chatbot enforcement caller and usage helper** — The generic chatbot HTTP path still called `EnforceBudget`, and `platform_api.go` still exposed `RecordUsage`; removed those obsolete entry points and migrated the related test fixture. Files: `internal/httpx/chatbot.go`, `internal/httpx/platform_api.go`, `internal/httpx/chatbot_test.go`. Commit: `2ab80cb`, `fb1355b`.
- Retained legacy fields on `APIKeyRecord` and its SQLite producer because that store is explicitly deferred to plan 01-36; these are storage-record fields, not fields on `Principal`.

**Total deviations:** 2 caller-scope extensions. **Impact:** All compiled callers were migrated; no APIKeyID bridge remains in active auth/chatbot/HTTP code.

## Self-Check: FAILED

The build passed and both task commits are on this worktree branch. The full requested test command failed in the unrelated randomized Responses item-ID assertion documented above. The failed verification remains unresolved.

## Next Phase Readiness

Plan 01-36 remains responsible for removing the legacy SQLite backend and its still-typed constructor fixture slot.
