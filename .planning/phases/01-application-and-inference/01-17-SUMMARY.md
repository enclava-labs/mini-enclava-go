---
phase: 01-application-and-inference
plan: "17"
subsystem: billing-httpx
tags: [postgres, billing, budgets, pricing, usage-aggregates]
status: complete
requirements: [CORE-05, JOB-03, JOB-04]

requires:
  - phase: 01-application-and-inference (plan 01-11)
    provides: postgres.BillingRepo (reservations/usage/prices/budgets over
      migration 005) + billing.Service admission/settlement authority
  - phase: 01-application-and-inference (plan 01-16)
    provides: IdentityRepo-backed keys and dashboardTenantID convention the
      billing dashboard sections now use
provides:
  - httpx.App billing group on postgres — pricingRepo/budgetsRepo/
    billingAuditRepo fields and every sqlite type gone; NewApp keeps its
    positional shape (usageRepo stays for the responses.go write path)
  - EnforceBudget reimplemented on stored billing budgets — resolves the
    principal's billing subject + credential subject + tenant, evaluates
    held+settled liability inside each budget's period window
  - Admin pricing CRUD on rational billing.Price rows (one unit kind per
    row, validated against the UnitKind const set) with billing_price_audit
    rows written in the PutPrice transaction
  - BillingRepo read methods: ListPrices/ListPriceAudit/ListBillingAudit/
    CreateBillingAudit/ListActiveBudgets/Liabilities/UsageTotals/
    UsageBySubject/UsageByEndpoint/UsageByModel/SubjectUsageSince
  - Dashboard usage/stats/budgets pages on tenant-scoped usage_records
    aggregates; pricing + billing-audit partials carry billing types
  - budgets_test rebuilt on testdb + BillingRepo: denies at stored amount/
    token limits, admits below them, period windows select correctly

affects: [01-18 providers cutover, 01-20 dashboard session, 01-34 responses removal, 01-35 legacy field removal]

actuals:
  tokens: ~
  tasks: 2
  commits: 1

tech-stack:
  added: []
  patterns:
    - "BillingRepo non-transactional reads share SQL with the tx-scoped paths (queryLiabilities, budgetSelectSQL/scanBudgets) — the read twin never reimplements liability semantics"
    - "Dashboard usage keys on billing subject_id (stored key UUID), not the legacy numeric handle"

key-files:
  created: []
  modified:
    - internal/httpx/app.go
    - internal/httpx/budgets.go
    - internal/httpx/pricing.go
    - internal/httpx/admin.go
    - internal/httpx/dashboard.go
    - internal/httpx/dashboard_resources.go
    - internal/httpx/budgets_test.go
    - internal/httpx/root_test.go
    - internal/httpx/openai_contract_test.go
    - internal/httpx/dashboard/templates/partial_pricing.tmpl
    - internal/httpx/dashboard/templates/partial_billing_audit.tmpl
    - internal/httpx/dashboard/templates/partial_usage.tmpl
    - internal/store/postgres/billing_repo.go
    - internal/billing/types.go
    - internal/modules/extract/handlers_test.go
    - cmd/enclava/main.go

key-decisions:
  - "billing.Budget gains a Name column field (the budgets table already stored it); CreateBudget writes it, LockActiveBudgets/ListActiveBudgets scan it"
  - "EnforceBudget checks the credential subject in addition to the billing subject: owner-delegated keys bill the user subject but still enforce api_key-scoped budgets — matches the budget subject semantics used by Reserve"
  - "Cost display converts billing currency-micros to cents (microsToCents, 10_000 micros/cent); the old cents/microcents helpers are deleted"
  - "usage_records aggregates sum units via a jsonb_each_text per-kind total — truthful for any advertised unit kind, not just tokens"

requirements-completed: [CORE-05, JOB-03, JOB-04]

coverage:
  - id: D1
    description: "Billing group cut over to postgres: pricing CRUD, budget enforcement, billing audit and usage aggregates all run on BillingRepo"
    requirement: JOB-03
    verification:
      - kind: unit
        ref: "go build ./... + grep pricingRepo|budgetsRepo|billingAuditRepo empty; ENCLAVA_TEST_DATABASE_URL go test ./internal/httpx"
        status: pass
    human_judgment: false
  - id: D2
    description: "EnforceBudget denies at stored budget limits and admits below them on real billing rows, period windows select correctly"
    requirement: JOB-04
    verification:
      - kind: integration
        ref: "internal/httpx/budgets_test.go#TestEnforceBudgetDeniesAtStoredAmountLimit TestEnforceBudgetDeniesOnTokenBudget TestEnforceBudgetPeriodWindow TestUsedForPeriodReadsSettledAggregates; openai_contract_test.go#TestOpenAIChatCompletionBudgetExceededBeforeProviderCall TestResponsesBudgetAppliesOnlyToCreate"
        status: pass
    human_judgment: false
---

# 01-17 Summary — billing group on postgres (pricing, budgets, audit, usage reads)

**`pricingRepo`/`budgetsRepo`/`billingAuditRepo` are gone from httpx and cmd: price CRUD, budget enforcement, billing audit and every usage read run on `postgres.BillingRepo` + `billing.Service`. `usageRepo` survives solely for the responses.go legacy write path (dies with the route in 01-34).**

## Accomplishments
- `app.go`: `billingSvc *billing.Service` + `billingRepo *postgres.BillingRepo` replace the three sqlite billing repos in the positional constructor; every NewApp caller updated (main.go, root_test, openai_contract_test, extract handlers_test)
- `budgets.go`: EnforceBudget keeps signature/denial surface (`budgetExceededError` → 429 budget_exceeded) but lists active billing budgets for {billing subject, credential subject, tenant} and evaluates held+settled liability via `BillingRepo.Liabilities` inside each budget's period window; `usedForPeriod` reads `SubjectUsageSince`; legacy principal budget limits evaluate the same aggregate
- `pricing.go`: `billing.Price` admin CRUD — one unit kind per row validated against the UnitKind const set, negative rates and non-positive unit_quantity rejected via `errInvalidPricing`, writes through PutPrice with its audit row; billing-audit list filters by subject_type/subject_id strings
- `admin.go`: usage summary → `UsageBySubject` (tenant-scoped); `recordAPIKeyBillingAudit` → `CreateBillingAudit` keyed by the stored key UUID (no LegacyAPIKeyID)
- `dashboard.go`: stats/usage/budgets blocks on `UsageTotals`/`UsageBySubject`/`UsageByEndpoint`/`UsageByModel`; dashboard usage rows key on `subject_id` (key UUID), cost converted micros→cents
- `dashboard_resources.go`: pricing + billing-audit partials carry `postgres.PriceRow`/`PriceAuditRow`/`BillingAuditRow` shapes, tenant-scoped via `dashboardTenantID`; templates updated to billing columns
- `billing_repo.go`: non-transactional reads (ListPrices, ListPriceAudit, ListBillingAudit, CreateBillingAudit, ListActiveBudgets, Liabilities, usage aggregates, SubjectUsageSince); liability SQL shared with the tx path via `queryLiabilities`
- `billing/types.go`: `Budget.Name` field; `main.go`: single `postgres.NewBillingRepo` shared by `billing.NewService` and the app

## Task Commits
1. `feat(01-17): billing repo cutover for pricing, budgets, audit, usage reads`

## Verification
- `go build ./...` — clean
- `gofmt -l internal/httpx cmd/enclava` — empty (two pre-existing unformatted files under internal/store/postgres untouched and unrelated)
- `ENCLAVA_TEST_DATABASE_URL=… go test ./internal/httpx/ -count=1` — pass
- `ENCLAVA_TEST_DATABASE_URL=… go test ./internal/billing ./internal/store/postgres ./internal/modules/extract -count=1` — pass
- `grep -rn 'pricingRepo\|budgetsRepo\|billingAuditRepo' internal/httpx/ cmd/` — empty
- `go vet ./...` — clean

## Deviations
- EnforceBudget checks the credential subject in addition to the billing subject: `auth.Service` sets `BillingSubjectKind=user` for owner-delegated keys, so an api_key-scoped budget must still bind via `CredentialID` — without this the two contract budget tests could not observe a deny
- Admin billing-audit `api_key_id` (legacy int64) filter replaced by `subject_id` string — the billing schema stores UUID subject IDs
- Contract-test budget seeds rewritten from `sqlite.BudgetsRepo.Upsert` to `BillingRepo.CreateBudget` (kind=tokens, limit 0) — the intent (deny on exhausted budget) is preserved on the new store
- `billingSvc` field is stored but unused inside httpx — retained per the positional param swap; service consumers live outside httpx
- internal/rag integration tests require ENCLAVA_TEST_QDRANT_URL — pre-existing env gate, unrelated

## Self-Check: PASSED
