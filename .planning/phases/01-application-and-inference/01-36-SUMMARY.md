---
phase: 01-application-and-inference
plan: "36"
subsystem: application-runtime
tags: [go, postgres, inference, billing, sqlite-removal]
requires:
  - phase: 01-35
    provides: Legacy principal and duplicate usage/accounting glue removal
provides:
  - Removed SQLite metadata runtime, all SQLite repositories/migrations, duplicate usage parser and modernc SQLite dependency
  - Migrated NewApp callers to the single PostgreSQL/executor composition
  - Replaced stale application API and route references with current selected runtime boundaries
  - Verified the two-process PostgreSQL admission/accounting scenario
affects: [application-runtime, inference, billing, identity, documentation]
actuals:
  tokens: 3000
  tasks: 2
  commits: 3
  plan_head_before: e427a4a49d57e5c879c8453efe635264bc550db0
  plan_head_after: e55eb0f6b7b2b60f8a385ba0a061837f69dcb43b
tech-stack:
  added: []
  patterns: [single PostgreSQL metadata store, shared inference executor]
key-files:
  created: []
  modified:
    - cmd/enclava/main.go
    - internal/httpx/app.go
    - internal/httpx/root_test.go
    - internal/httpx/identities_test.go
    - internal/httpx/openai_contract_test.go
    - internal/httpx/responses_contract_test.go
    - internal/modules/extract/handlers_test.go
    - internal/store/sqlite/
    - internal/httpx/usage_parse.go
    - go.mod
    - go.sum
    - docs/go-rewrite-contract.md
    - docs/go-rewrite-route-matrix.md
key-decisions:
  - "Remove the final typed SQLite usage-repository constructor slot rather than preserve a compatibility argument."
  - "Keep provider/prober fields because current dashboard, chatbot and attestation paths still use them."
  - "Document unexercised external OIDC, inference and browser acceptance as blockers, not passes."
patterns-established: []
requirements-completed:
  - CORE-01
  - CORE-02
  - CORE-03
  - CORE-04
  - CORE-05
  - AUTH-01
  - AUTH-02
  - AUTH-03
  - AUTH-04
  - AUTH-05
  - AUTH-06
  - JOB-01
  - JOB-02
  - JOB-03
  - JOB-04
  - INF-01
  - INF-02
  - INF-03
  - INF-04
  - INF-05
  - INF-06
  - CON-01
  - CON-02
  - UI-01
  - UI-02
coverage:
  - id: D1
    description: "The active runtime no longer imports or ships the SQLite metadata backend; constructor callers use PostgreSQL and shared-executor contracts."
    requirement: CORE-05
    verification:
      - kind: automated
        ref: "go build ./..."
        status: pass
      - kind: automated
        ref: "ENCLAVA_TEST_DATABASE_URL=... ENCLAVA_TEST_QDRANT_URL=... QDRANT_INTEGRATION_URL=... go test -count=1 ./..."
        status: fail
    human_judgment: true
    rationale: "The package tests passed, but the full suite exits non-zero because tests/contract requires external app URL, API key and model names; full-plan behavioral acceptance is not implied by the build."
  - id: D2
    description: "Two-process shared PostgreSQL admission/accounting works with the controlled upstream integration fixture."
    requirement: JOB-04
    verification:
      - kind: integration
        ref: "ENCLAVA_PROXY_INTEGRATION=1 go test -count=1 ./tests -run TestProxyTwoProcess"
        status: pass
    human_judgment: true
    rationale: "The controlled upstream proves the repository integration scenario, not a real provider contract or interactive application/browser behavior."
  - id: D3
    description: "Current application reference documents selected routes, ownership, accounting units and per-process versus shared bounds without deployment/compliance claims."
    verification: []
    human_judgment: true
    rationale: "Routes and contracts were reconciled to source, but the plan requires live API, OIDC, cancellation/restart, provider and browser scenarios that could not be exercised before merge."
# Metrics
duration: unknown
completed: 2026-10-19
status: halted
---

# Phase 01 Plan 36: Single-store runtime cutover and core application proof

**The Go application drops its final SQLite runtime path and records the selected PostgreSQL/executor routes and accounting boundaries.**

## Performance

- **Duration:** Not recorded.
- **Started:** Not recorded.
- **Completed:** 2026-10-19T18:35:53Z
- **Tasks:** Task 1 implementation complete; Task 2 documentation complete, live acceptance blocked.
- **Files modified:** 41.

## Accomplishments

- Removed every `internal/store/sqlite` repository and migration, `internal/httpx/usage_parse.go`, and modernc SQLite dependencies; removed the final `NewApp` usage-repository parameter and migrated all actual callers.
- Updated the two existing application-reference documents to match mounted routes, PostgreSQL identity/ownership, exact billing units, local-vs-shared bounds and explicit capability/assurance limits.
- Ran the real two-process admission/accounting integration test against its controlled HTTP upstream and PostgreSQL fixture.

## Task Commits

1. **Task 1: Complete composition and delete the obsolete store/usage paths** — `9eaae596a23e617903a37e13f482579e42328ccd` (`feat(01-36)`).
2. **Task 2: Exercise the real core and update the existing application reference** — `e55eb0f6b7b2b60f8a385ba0a061837f69dcb43b` (`docs(01-36)`). Documentation and available automated integration completed; required live acceptance remains blocked.
3. **Plan metadata:** this SUMMARY commit.

## Verification

- `go build ./...` — PASS.
- `ENCLAVA_TEST_DATABASE_URL=... ENCLAVA_TEST_QDRANT_URL=http://127.0.0.1:6333 QDRANT_INTEGRATION_URL=http://127.0.0.1:6333 go test -count=1 ./...` — FAIL only at `tests/contract`: required `OPENAI_CONTRACT_BASE_URL`, `OPENAI_CONTRACT_API_KEY`, and chat/embedding/Responses model names were not configured. PostgreSQL, Qdrant, and all other reported packages passed.
- `ENCLAVA_PROXY_INTEGRATION=1 go test -count=1 ./tests -run TestProxyTwoProcess` — PASS (real application processes and shared PostgreSQL fixture; controlled upstream).
- `node /home/user/.agents/gsd-core/bin/gsd-tools.cjs check evaluation-scope --plan 01-36 --commits-only --raw` — PASS; both task commits resolved on this worktree branch.
- Not exercised: post-merge application/browser scenarios, live OIDC login/linking, real external inference compatibility, live user/key/role lifecycle, and full restart/cancel/revocation behavior. The post-merge harness is explicitly unavailable to this pre-merge worktree; no mocks or test skips are represented as proof.

## Deviations from Plan

None in implementation scope. Required real-service proof is incomplete because the OpenAI contract environment and post-merge application/browser harness are not available here; this is recorded as a blocker, not a successful verification.

**Total deviations:** 0 auto-fixed. **Impact:** SQLite removal and documentation changes are committed; the plan's end-to-end acceptance remains unverified and must not be treated as complete evidence for live identity/provider/UI behavior.

## Issues Encountered

- The full Go suite initially reported missing Qdrant integration configuration; rerunning with the supplied Qdrant URL passed those packages.
- The OpenAI SDK contract test fails closed when live application URL, credential and model configuration are absent. The post-merge-only app harness could not be used from this worktree.

## User Setup Required

For the remaining acceptance checks, configure `OPENAI_CONTRACT_BASE_URL`, `OPENAI_CONTRACT_API_KEY`, `OPENAI_CONTRACT_CHAT_MODEL`, `OPENAI_CONTRACT_EMBEDDING_MODEL`, and `OPENAI_CONTRACT_RESPONSES_MODEL`; configure a real OIDC provider if OIDC is to be exercised; then run the application/API/browser/restart scenarios using the post-merge harness.

## Self-Check: PASSED

The commit-scope check found both implementation/documentation task commits. The build and available PostgreSQL/Qdrant/two-process checks passed; the full acceptance gate remains incomplete because live OpenAI contract configuration and the post-merge OIDC/browser harness were not available. The missing scenarios are documented above and are not counted as passed.

## Next Phase Readiness

The source cutover and available automated integration checks are complete. Do not claim the plan's live-core acceptance until the external OpenAI contract, real OIDC, identity/provider/usage browser flows, and post-merge application scenarios have been exercised. The final app integration and user acceptance checks remain blocked by their configured service prerequisites.

---
*Phase: 01-application-and-inference*
*Completed: 2026-10-19*

## Orchestrator verification addendum (post-merge, live single-store binary)

Merged tree rebuilt and served by the real binary on 127.0.0.1:18444 against PostgreSQL (sqlite store fully deleted from the tree):
- Full gate: go build ./... green; go test ./... green over real PostgreSQL + live Qdrant, INCLUDING the official OpenAI SDK contract suite against this live binary (models, chat unary+stream, embeddings, full owned Responses lifecycle with bindings) — the credentials the executor lacked are harness-provided post-merge.
- Browser: session login (local account), dashboard renders, budgets partial shows exact integer-micros aggregates over real liability/usage rows (Held 0.000000 / Unknown 3.146342 / Settled 0.000944 / Remaining 1.852714 against the USD 5.000000 budget).
- Live chat completion through the shared executor returned a real upstream response with usage accounting.
Remaining honest limitation (inherited from 01-32): no confidential/attested upstream exists in this environment, so the confidential-route acceptance criterion stays environment-blocked, not code-blocked.
