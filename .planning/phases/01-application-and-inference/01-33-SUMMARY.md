---
phase: 01-application-and-inference
plan: "33"
subsystem: billing dashboard
completed: 2026-10-19
---

# Phase 01 Plan 33: Show exact usage, reservations, prices and budgets

## Outcome

Partial implementation only. Removed the hardcoded dashboard allowance and float/cents presentation from the home and usage views. Settled currency totals now remain separated by currency and display integer currency-micros at six decimal places. The budget page reads the authenticated billing subject's `Liabilities` rows, aggregates per currency and preserves raw unit totals with overflow saturation. Pricing rows label exact `rate_micros / unit_quantity`; immutable price writes remain unchanged.

## Tasks

- Task 1: Partially completed. Changed dashboard totals to avoid the fictitious default cap and lossy currency rounding; added checked budget-liability aggregation and an overflow helper test. Commit: `bb21e53`.
- Task 2: Partially completed. Updated usage, budget and pricing labels to avoid claiming cents or settled-only cost for liability. Commit: `bb21e53`.

## Verification

- NOT RUN: `test -n "$ENCLAVA_TEST_DATABASE_URL" && go test -count=1 ./internal/httpx ./internal/billing` — `ENCLAVA_TEST_DATABASE_URL` is unset in this environment. No database-backed or live/browser acceptance criteria were exercised.

## Deviations from Plan

**[Rule 3 - Missing live prerequisite] PostgreSQL verification unavailable** — Found during: Tasks 1–2 | The required PostgreSQL DSN is absent. The mandatory verification commands and actual two-request/cancelled-stream observation could not be exercised. No test is reported as passing.

**[Rule 3 - Missing repository contract] Budget administration and per-state liability views unavailable through current HTTP repository API** — Found during: Tasks 1–2 | `BillingRepo` has concrete `Liabilities` and `ListActiveBudgets` methods, but the public `BillingRepository` contract does not expose these reads; it also has no budget listing or reservation-state enumeration method. `LiabilityRow` contains amounts/currencies/units but no state, so the dashboard cannot truthfully distinguish settled, held and unknown rows or calculate configured remaining allowance for each subject. The actual pricing API remains immutable and tenant authenticated, but the existing pricing partial is read-only. Adding ad-hoc SQL or fabricated controls would violate the single-authority/no-fakes contract. These acceptance criteria remain incomplete.

**[Rule 3 - Missing browser/upstream prerequisites] Live UI acceptance unavailable** — Found during: Task 2 | No running application/provider configuration was supplied for observing concurrent requests, stream cancellation, changed-rate effects, keyboard behavior or browser totals against service responses. No smoke result is claimed.

No files were deleted.

**Total deviations:** 3 blockers. **Impact:** Source changes improve exact presentation and remove the fake allowance, but the plan is not complete: state-specific reservations, real configured budget views/edits, browser acceptance and PostgreSQL tests remain unverified/unimplemented.

## Self-Check: FAILED
