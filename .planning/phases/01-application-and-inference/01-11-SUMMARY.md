---
phase: 01-application-and-inference
plan: "11"
subsystem: billing
tags: [postgres, billing, reservations, exact-arithmetic, endpoints]

requires:
  - phase: 01-application-and-inference (plans 01-04, 01-06, 01-08)
    provides: billing/inference contract types, 005 billing schema, tenant-scoped auth principal, postgres store harness
provides:
  - billing.Service Reserve/Settle/MarkDispatched/MarkUnknown/ReleaseUndispatched over BillingRepository
  - billing.BillingRepository/BillingTx seam + postgres BillingRepo (prices/budgets/reservations/usage)
  - postgres EndpointsRepo implementing inference.EndpointRepository incl. ListCapabilities
  - migrations 007 inference_endpoints, 008 reservation dispatch-context columns
affects: [inference executor (01-23), billing admin/EnforceBudget (01-17), endpoint admin (01-18), usage dashboard (01-33)]

actuals:
  tokens: ~5700
  tasks: 2
  commits: 2

tech-stack:
  added: []
  patterns:
    - "One reservation = N subject-attribution rows sharing one reservation id (canonical row + '<id>/<type>:<subject>' rows); transitions address the group by prefix"
    - "InTx seam: service owns policy, BillingTx owns SQL; no transaction spans upstream work"
    - "Exact rational charge: per-kind ceil(rate_micros×units÷unit_quantity) in 128-bit, overflow → ErrArithmeticOverflow"

key-files:
  created:
    - internal/billing/repository.go
    - internal/billing/service.go
    - internal/billing/service_test.go
    - internal/store/postgres/billing_repo.go
    - internal/store/postgres/billing_repo_test.go
    - internal/store/postgres/endpoints_repo.go
    - internal/store/postgres/endpoints_repo_test.go
    - internal/store/postgres/migrations/007_endpoints.sql
    - internal/store/postgres/migrations/008_reservation_context.sql
  modified:
    - internal/billing/types.go
    - internal/inference/endpoint.go
    - internal/testdb/testdb.go

key-decisions:
  - "Multi-subject admission writes one liability row per checked subject so every subject's budgets reconstruct liability across processes; canonical row keeps the reservation id, attribution rows carry '<id>/<type>:<subject>'"
  - "Reservation rows persist capability/model/endpoint_id (migration 008) because usage_records require them NOT NULL and must not trust settle-time caller input"
  - "Tenant-scope budgets count all canonical reservations and usage of the tenant; subject budgets match exact subject rows"
  - "Settle returns the existing usage_records row on re-settle — once-only via held→settled guard + UNIQUE(tenant,reservation_id)"
  - "ListCapabilities added to inference.EndpointRepository (plan named it as the routing seam)"

patterns-established:
  - "Budget admission: lock applicable budgets FOR UPDATE in sorted (subject,id) order, then liabilities, then insert — deterministic serialization, no deadlock order skew"
  - "Held/unknown reservations always count; settled usage counts inside the budget's period window (daily/weekly-ISO/monthly clamped to starts_at)"
  - "Cached input settles disjointly at its own rate — never double-billed as input"

requirements-completed: [JOB-03, JOB-04, INF-01]

coverage:
  - id: D1
    description: "Atomic budget admission: proposed bound counted, two-actor race never over-admits, cross-process token liability reconstructed from persisted reserved_units incl. zero rates"
    requirement: JOB-03
    verification:
      - kind: integration
        ref: "internal/billing/service_test.go#TestReserveDeniesWhenProposedBoundExceedsRemaining/TestConcurrentReserveNeverOverAdmits/TestTokenBudgetDenialReconstructsUnitsAcrossProcesses"
        status: pass
    human_judgment: false
  - id: D2
    description: "Immutable price snapshots: missing rate refuses (ErrPriceMissing), mixed currencies reject, cross-currency vs amount budget denies, later price change does not alter settlement"
    requirement: JOB-03
    verification:
      - kind: integration
        ref: "internal/billing/service_test.go#TestMissingRateRefusesDispatch/TestMixedCurrencyPriceSetRejected/TestCrossCurrencyReservationDeniedByAmountBudget/TestSnapshotHonoredAfterPriceChange"
        status: pass
    human_judgment: false
  - id: D3
    description: "Once-only settlement without clipping: overrun stores true amount, cached_input disjoint, exact fractional rational under ceil, overflow → ErrArithmeticOverflow"
    requirement: JOB-04
    verification:
      - kind: integration
        ref: "internal/billing/service_test.go#TestSettleIsOnceOnly/TestOverrunSettleStoresTrueAmount/TestCachedInputSettlesDisjointly/TestDistinctAndFractionalRatesSettleExactly/TestOverflowEstimateRejected"
        status: pass
    human_judgment: false
  - id: D4
    description: "Unknown outcomes retain liability; ReleaseUndispatched frees only never-dispatched holds"
    requirement: JOB-04
    verification:
      - kind: integration
        ref: "internal/billing/service_test.go#TestMarkUnknownRetainsLiability/TestReleaseUndispatched"
        status: pass
    human_judgment: false
  - id: D5
    description: "Tenant-scoped inference endpoint records: CRUD, cross-tenant isolation, capability routing, assurance/authorized_private independence, origin validation"
    requirement: INF-01
    verification:
      - kind: integration
        ref: "internal/store/postgres/endpoints_repo_test.go#TestEndpointsRepoCRUDAndTenantIsolation/TestEndpointsRepoOriginValidation"
        status: pass
    human_judgment: false

duration: 45 min
completed: 2026-10-19
status: complete
---

# Phase 01 Plan 11: Exact Billing Service + Endpoint Records Summary

**Atomic cross-process billing: budget-locked reservations with immutable rational price snapshots, once-only unclipped settlement, retained unknown liability — plus tenant-scoped inference endpoint records on real PostgreSQL.**

## Performance

- **Duration:** ~45 min
- **Commits:** 2 production commits (c690b84, 0911f52)
- **Files:** 9 created, 3 modified

## Accomplishments

- `internal/billing/service.go` + `repository.go`: `Service.Reserve` runs admission in ONE transaction — budgets `SELECT ... FOR UPDATE` in deterministic `(subject_type, subject_id, id)` order, price set resolved per unit kind at `now()`, every estimated unit kind must have a rate (`ErrPriceMissing`), all selected rates share one currency (`ErrCurrencyMismatch`), admission arithmetic enforces existing-liability + proposed-bound ≤ limit for amount budgets (same currency) and token budgets (persisted `reserved_units`), then inserts held reservation rows before commit. `Settle` guards held→settled, inserts `usage_records` with RAW units and snapshot-computed amount (per-kind ceil under 128-bit checked math — overruns stored truthfully, re-settle returns the existing evidence). `MarkUnknown` retains liability (rows stay counted); `ReleaseUndispatched` frees only `dispatched_at IS NULL` holds; `MarkDispatched` stamps dispatch.
- `internal/store/postgres/billing_repo.go`: `BillingRepo` implements the seam — `InTx`/`LockActiveBudgets`/`SelectPrices` (DISTINCT ON latest effective per unit kind)/`Liabilities` (held+unknown always count, settled windowed; tenant scope is tenant-wide over canonical rows)/`InsertReservations`/`Transition`/`InsertUsageRecord`/`GetUsageRecord`; `PutPrice` writes immutable price + audit row; `CreateBudget` inserts limits.
- `internal/store/postgres/endpoints_repo.go`: `EndpointsRepo` implements `inference.EndpointRepository` (Get/List/Create/Update/SetEnabled) + `ListCapabilities` (enabled endpoints advertising a capability). `base_origin` enforced as absolute https origin; http only with `AuthorizedPrivate`; `assurance_verified_at` persists independently of `authorized_private`.
- Migrations: `007_endpoints.sql` (inference_endpoints table); `008_reservation_context.sql` (capability/model/endpoint_id columns on reservations — required because usage_records demand them NOT NULL and settle must not trust caller input).

## Verification

- `ENCLAVA_TEST_DATABASE_URL=postgres://enclava:enclava@localhost:5432/enclava go test ./internal/billing/ ./internal/store/postgres/ -count=1 -v` → all 41 tests PASS, zero SKIP.
- `go build ./...` clean; `go vet` clean on touched packages; `gofmt -l` empty.
- Every task-1 acceptance criterion has a named passing test (proposed-bound denial, race, cross-process token units with zero rates, cross-currency denial, unknown liability, snapshot honored, overrun unclipped).

## Deviations from Plan

- **[Rule 3 - Missing critical]** `Reservation`/`Budget` contract types needed dispatch-context and window fields (`Capability/Model/EndpointID/DispatchedAt/Currency/JobID/AttemptID` on Reservation; `ID/StartsAt/EndsAt` on Budget) and new sentinels `ErrReservationNotFound`/`ErrReservationState` + `UsageRecord` — added to `internal/billing/types.go` (file outside the plan's list).
- **[Rule 3 - Missing critical]** `usage_records.capability/model` are NOT NULL while the plan's `Settle(ctx, id, Usage)` sketch supplies none — resolved by persisting dispatch context on the reservation via new migration `008_reservation_context.sql` (additive ALTER; no edit to 005).
- **[Rule 3 - Missing critical]** The reservations PK `(tenant_id,id)` allows only one row per reservation id, so multi-subject liability uses per-subject attribution rows (`<id>/<type>:<subject>`) sharing the reservation id; transitions/liability address the group by prefix.
- **[Rule 3 - Missing critical]** `inference.Endpoint` has no Config/CreatedBy fields — migration stores `config '{}'` and `created_by` NULL; `ErrEndpointNotFound` + `ListCapabilities` added to `internal/inference/endpoint.go` (the interface, since the plan named ListCapabilities part of the consumed seam).
- **Scope choice** `testdb.NewWithDSN`/`Secondary` added so the two-process token test uses two pools over one schema.

**Total deviations:** 5 auto-fixed (all Rule 3). **Impact:** additive schema and contract fields only; no existing behavior changed.

## Self-Check: PASSED
