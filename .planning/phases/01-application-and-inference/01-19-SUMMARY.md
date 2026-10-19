---
phase: 01-application-and-inference
plan: "19"
subsystem: httpx-extract
tags: [postgres, extract, dashboard, cutover]
status: complete
requirements: [CORE-05, AUTH-05]

requires:
  - phase: 01-application-and-inference (plan 01-13)
    provides: extract.Service port — Process/ListJobs/JobStats-ready JobReader,
      SeedDefaults, GetSettings first-touch row, and the dashboard_extract
      handlers already on extractSvc with PrincipalFromContext
  - phase: 01-application-and-inference (plan 01-16)
    provides: dashboardTenantID/extractPrincipal convention — real request
      principal or truthful 401, never fabricated authority

provides:
  - httpx.App extract group fully on extract.Service — the
    extractJobs/extractTemplates/extractSettings sqlite fields, NewApp
    params and assignments are deleted; zero sqlite extract repo references
    remain in httpx or cmd
  - Dashboard extract stats (partial-stats + partial-extract) read
    extractSvc.JobStats — owner-scoped via jobs.SubmitterKey on
    postgres.JobsRepo.CountOwnedJobs + new CountOwnedJobsSince (submitted_at
    >= since window)
  - partial-extract template table renders extract.Template
    (DefaultModel/Enabled); the sqlite.ExtractTemplate shape is gone from
    the view
  - main.go seeds the bootstrap tenant's default templates
    (extractSvc.SeedDefaults) and materializes the default settings row
    (extractSvc.GetSettings first-touch upsert) — legacy sqlite
    EnsureDefaults/EnsureDefaultRow seeding is gone with the repos

affects: [01-20 dashboard session, 01-22 bootstrap CLI]

actuals:
  tokens: ~
  tasks: 1
  commits: 1

tech-stack:
  added: []
  patterns:
    - "Dashboard stat reads consume the owned service's tenant/principal-
      scoped view (JobStats) instead of global sqlite counts — no
      cross-tenant leakage"

key-files:
  created:
    - .planning/phases/01-application-and-inference/01-19-SUMMARY.md
  modified:
    - internal/httpx/app.go
    - internal/httpx/dashboard.go
    - internal/httpx/dashboard/templates/partial_extract.tmpl
    - internal/extract/service.go
    - internal/store/postgres/jobs_repo.go
    - cmd/enclava/main.go
    - internal/httpx/root_test.go
    - internal/httpx/openai_contract_test.go
    - internal/modules/extract/handlers_test.go

key-decisions:
  - "extract.Service.JobStats(p, since) returns (recent, total) owner-bound
    via jobs.SubmitterKey; postgres.JobsRepo.CountOwnedJobsSince added to
    the JobReader interface — dashboard keeps 24h/total semantics without
    cross-tenant counts"
  - "Dashboard extract partial now resolves extractPrincipal — password-
    cookie requests get truthful 401 until session principals land (01-20)"
  - "Bootstrap seeding uses a minimal auth.Principal{TenantID, ActorID} —
    SeedDefaults/Settings are tenant-keyed writes needing no scopes"

requirements-completed: [CORE-05, AUTH-05]

verification:
  - "go build ./... — clean"
  - "gofmt -l internal/httpx cmd/enclava — clean"
  - "ENCLAVA_TEST_DATABASE_URL go test ./internal/httpx/ -count=1 — pass"
  - "ENCLAVA_TEST_DATABASE_URL go test ./internal/modules/extract/ -count=1 — pass"
---
