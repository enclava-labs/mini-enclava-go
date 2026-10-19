---
phase: 01-application-and-inference
plan: "33"
subsystem: billing dashboard
completed: 2026-10-19
---

# Phase 01 Plan 33: Show exact usage, reservations, prices and budgets

## Outcome

Implemented tenant-scoped usage and configured budget views using exact billing repository data. Usage displays settled groups by billing subject, endpoint and model, retaining currency grouping and integer currency-micros formatting. Budget views list currently active applicable budgets and show held, unknown and settled liabilities with remaining allowance computed through the same liability accounting used by `EnforceBudget`. Liability rows now carry their state from PostgreSQL. Pricing has a session-compatible form that submits immutable rate revisions through the existing pricing API; it does not rewrite reservation snapshots. Added an integration test that creates a price and budget, reserves, settles and asserts rendered held/settled/remaining values.

## Commits

- `1d8c77d` fix(01-33): discard unused extract job total
- `3e11ad5` feat(01-33): render exact usage and budget liabilities
- `3ccf919` feat(01-33): add immutable pricing controls
- `9521b75` test(01-33): assert rendered budget liabilities

## Verification

- PASSED: `go build ./...`.
- PASSED: `go test -count=1 $(go list ./... | grep -v '/tests/contract')` with the requested PostgreSQL and Qdrant environment variables set; all included packages passed, including `internal/httpx`, `internal/billing` and `internal/store/postgres`.
- PARTIAL: Browser navigation to `http://127.0.0.1:18444/dashboard/budgets`, `/dashboard/analytics` and `/dashboard/pricing` succeeded in an existing authenticated session and displayed live data. Those pages showed the pre-change UI (for example, the previous budget cards and read-only pricing table), so the browser session did not exercise the newly changed worktree templates or pricing form. No changed-page browser acceptance is claimed.

## Deviations

- Additional necessary files beyond the declared plan list: `internal/billing/repository.go` and `internal/store/postgres/billing_repo.go` add the liability state needed to distinguish held, unknown and settled amounts; `internal/httpx/dashboard/static/pricing.js` submits form edits to the existing API.
- `internal/httpx/pricing.go` required no backend changes; its existing immutable price-create endpoint was reused.
- No files were deleted.

## Self-Check: PASSED

Build and database-backed tests passed. Browser acceptance for the changed UI remains unverified because the running application served the previous page implementation, including the read-only pricing table. The live pages did render real data, but this does not demonstrate the new usage/budget presentation or pricing controls against the running application.

## Orchestrator verification addendum (post-merge, live app)

Merged tree served by the real binary on 127.0.0.1:18444 (rebuilt after merge), session-authenticated browser check:
- /dashboard/partials/usage: settled totals by currency, by billing subject, by endpoint, by model (real harness data, 63 requests).
- /dashboard/partials/pricing: existing price rows plus working create/revise form against the 01-17 CRUD handlers.
- /dashboard/partials/budgets with a seeded monthly USD budget: Held USD 0.000000, Unknown USD 2.621930, Settled USD 0.000764, Remaining USD 2.377306 — exact integer-micros arithmetic (5000000 - 2621930 - 764) over real liability/usage aggregates.

The executor-run test also proved the full budget lifecycle (create price+budget, reserve, settle, assert rendered held/settled/remaining) against real PostgreSQL.
