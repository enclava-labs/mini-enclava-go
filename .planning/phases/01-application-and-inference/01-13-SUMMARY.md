---
phase: 01-application-and-inference
plan: "13"
subsystem: extract
tags: [extract, postgres, jobs, artifacts, billing, auth-principal]
status: complete
requirements: [CORE-05, JOB-01, JOB-04]

requires:
  - phase: 01-application-and-inference (plan 01-07)
    provides: artifacts.Store Begin/Finalize/Read + postgres.ArtifactRepo
  - phase: 01-application-and-inference (plan 01-10)
    provides: jobs.Service Enqueue/Lease/Claim + postgres.JobsRepo
  - phase: 01-application-and-inference (plan 01-11)
    provides: billing.Service Reserve/Settle/MarkUnknown/ReleaseUndispatched + postgres.BillingRepo
provides:
  - extract.Service on durable artifacts+jobs+billing — Process enqueues
    extract.process after persisting the input artifact; JobHandler is the
    fenced, code-complete attempt (reserve → dispatch → provider → result
    artifact → settle)
  - extract.Template/Settings/JobView/ResultView tenant-scoped domain types
  - postgres.ExtractRepo — extract_templates + extract_settings over 006 schema
  - jobs.SubmitterKey + JobsRepo.CountOwnedJobs — owner-bound job reads/paging
  - modules/extract + dashboard_extract + main.go callers on the new surface
affects: [extract executor dispatch (01-25), dashboard stat migration (01-19), httpx session principal (01-16/01-20)]

actuals:
  tokens: ~5800
  tasks: 2
  commits: 1

tech-stack:
  added: []
  patterns:
    - "Async job cutover: Process persists input artifact then Enqueue with subject bound to artifact id+version; the same Submit/Lease/Complete fence path runs the attempt"
    - "Billing inside the attempt: Reserve before MarkDispatched, ReleaseUndispatched on pre-dispatch failure, MarkUnknown on unobserved upstream outcome, Settle after result artifact commits"
    - "JobView projects jobs.Job + immutable policy doc onto the legacy consumer JSON shape; ResultView decodes the result artifact"

key-files:
  created:
    - internal/store/postgres/extract_repo.go
    - internal/store/postgres/extract_repo_test.go
    - internal/extract/service_integration_test.go
  modified:
    - internal/extract/types.go
    - internal/extract/defaults.go
    - internal/extract/service.go
    - internal/extract/service_test.go
    - internal/modules/extract/handlers.go
    - internal/modules/extract/handlers_test.go
    - internal/httpx/dashboard_extract.go
    - internal/jobs/service.go
    - internal/store/postgres/jobs_repo.go
    - cmd/enclava/main.go

key-decisions:
  - "Process is enqueue-only: returns the durable job id; JobHandler() is registered on jobs.NewRunner in main but the loop is not started until the executor plan (01-25) — dispatch wiring is the parent's"
  - "Owner-scoped job reads reuse JobsRepo.GetOwnedJob/ListOwnedJobs keyed by exported jobs.SubmitterKey — foreign and unknown ids both map to extract.ErrNotFound"
  - "Dashboard password mode has no tenant principal: extract partials return 401 rather than fabricating APIKeyID:0; the API-key path supplies the real principal via auth.PrincipalFromContext"
  - "Legacy sqlite extract repo fields stay on httpx.App only for dashboard.go stat reads; no extract code path calls them (01-19 removes)"
  - "extract_templates UNQIUE(tenant_id,name) + PK(tenant_id,id) drive repo SQL; Template.Enabled maps enabled, DefaultModel maps default_model (JSON names kept for consumers)"

patterns-established:
  - "Durable input lifecycle: artifacts.Begin(Finalize) BEFORE jobs.Enqueue; input_ref carries the artifact id, subject carries id+version"
  - "Attempt billing contract: unpriced model => Reserve fails with ErrPriceMissing BEFORE provider invocation; settled usage_records hold raw units and snapshot-computed micros"
  - "Two-part test layout: pure-schema tests in package extract, postgres-backed tests in package extract_test (import cycle with internal/store/postgres)"

requirements-completed: [CORE-05, JOB-01, JOB-04]

coverage:
  - id: D1
    description: "Extract runs end-to-end on durable artifacts+jobs+reservations — no sqlite in the extract path"
    requirement: CORE-05
    verification:
      - kind: integration
        ref: "internal/extract/service_integration_test.go#TestProcessPersistsInputArtifactAndJobRow,TestAttemptPricedModelReservesAndSettles"
        status: pass
      - kind: integration
        ref: "internal/store/postgres/extract_repo_test.go#TestExtractRepoTemplatesTenantScoped,TestExtractRepoReplaceDefaultsKeepsCustom,TestExtractRepoSettingsUpsertRoundTrip"
        status: pass
    human_judgment: false
  - id: D2
    description: "Job row persists before Process returns its id; frozen principal snapshot on the row; idempotent enqueue identity"
    requirement: JOB-01
    verification:
      - kind: integration
        ref: "internal/extract/service_integration_test.go#TestProcessPersistsInputArtifactAndJobRow"
        status: pass
    human_judgment: false
  - id: D3
    description: "Priced model reserves+settles with truthful usage; unpriced model fails BEFORE provider call with no usage row"
    requirement: JOB-04
    verification:
      - kind: integration
        ref: "internal/extract/service_integration_test.go#TestAttemptPricedModelReservesAndSettles,TestAttemptUnpricedModelRefusesBeforeProvider"
        status: pass
      - kind: integration
        ref: "internal/modules/extract/handlers_test.go#TestHandleProcessEnqueuesDurableJobAndSettlesUsage"
        status: pass
    human_judgment: false
  - id: D4
    description: "Cross-tenant/cross-actor job+result reads deny identically to unknown ids; template/settings reads tenant-scoped"
    requirement: CORE-05
    verification:
      - kind: integration
        ref: "internal/extract/service_integration_test.go#TestCrossTenantJobReadsDenyIdentically,TestTemplateCRUDTenantScoped"
        status: pass
      - kind: integration
        ref: "internal/modules/extract/handlers_test.go#TestHandleJobDetailDeniesForeignAndUnknown"
        status: pass
    human_judgment: false

duration: ~35min
completed: 2026-10-19
---

# Phase 01 Plan 13: Extract service cutover to artifacts+jobs+billing

**extract.Service now runs on the durable stack: uploads finalize into the
artifact store before a job row exists, Process returns the durable job id,
and the fenced attempt reserves/settles billing around the provider call —
every caller (module handlers, dashboard partials, main.go) compiles against
the new surface in one boundary.**

## Accomplishments

- `internal/extract`: domain types (`Template`, `Settings`, `JobView`,
  `ResultView`) replace every `sqlite.*` reference; `Process(ctx, auth.Principal,
  ProcessInput)` authorizes via `auth.ExtractTemplateAllowed`/`auth.ModelAllowed`,
  streams input bytes through `artifacts.Begin`/`Finalize`, then
  `jobs.Enqueue`s `extract.process` with subject = input artifact id+version
  and the frozen principal — returning `{job_id, status:"queued"}`.
- Attempt path (`Service.JobHandler`): decodes the frozen principal, rechecks
  template/model allowlists, `billing.Reserve`s a bounded input/output
  estimate (`ErrPriceMissing` refuses before any provider call),
  `MarkDispatched`s both job and reservation, calls the provider, writes the
  result document as a `kind=result` artifact, then `Settle`s. Unobserved
  upstream outcomes `MarkUnknown`; pre-dispatch failures
  `ReleaseUndispatched`.
- `postgres.ExtractRepo`: tenant-scoped `extract_templates`/`extract_settings`
  CRUD; `DeleteTemplate` and `GetTemplate`/`GetSettings` return
  `extract.ErrNotFound` for absent or foreign rows;
  `ReplaceDefaultTemplates` swaps only the `is_default` set in one tx.
- `jobs`: exported `SubmitterKey` (owner-bound read views) and added
  `JobsRepo.CountOwnedJobs` for list totals.
- Callers: module handlers pass the real `auth.Principal`, drop both
  `RecordUsage` call sites (reserve/settle is internal now) and decode
  `extract.Template`; dashboard_extract routes every read/write through
  `a.extractSvc` with the real request principal (`PrincipalFromContext`,
  truthful 401 without one — no API-key-id-0); `main.go` wires
  `postgres.NewExtractRepo`/`NewJobsRepo`/`NewArtifactRepo` +
  `artifacts.NewStore` + `jobs.NewService` + `billing.NewService` and
  registers `extract.process` on a `jobs.Runner` that is constructed but not
  started (dispatch lands with the executor plan).

## Verification

- `go build ./...` — clean; `gofmt -l` — clean; `go vet` — clean on touched
  packages.
- `ENCLAVA_TEST_DATABASE_URL=postgres://enclava:enclava@localhost:5432/enclava_test
  go test ./internal/extract/ ./internal/modules/extract/
  ./internal/store/postgres/ -count=1` — **all pass, zero skips**
  (fixture Fatals without the DSN as required).
- `grep -rn 'sqlite' internal/extract internal/modules/extract
  internal/httpx/dashboard_extract.go` — empty.
- `go test ./...` — entire tree green.
- Acceptance criteria met: no `sqlite.` symbol in the extract path; unpriced
  model fails before the provider is invoked (call count asserted 0); job
  row + frozen principal persist before `Process` returns the id; cross-
  tenant/actor job and result reads return `ErrNotFound` identically to
  unknown ids.

## Decisions Made

- `Process` keeps a synchronous acknowledgement shape (`job_id`, `status`,
  resolved `model_used`); extraction results are read via `GetJob` —
  asynchronous execution is the contract, matching "no provider dispatch
  happens without an executor".
- Reservation estimate bounds input at ~1 token/2 bytes + prompt bytes +
  fixed envelope, output at 8192 tokens; settle records raw actuals
  unclipped (overrun truthful, per JOB-04).
- `internal/extract/service_integration_test.go` uses `package extract_test`
  — `internal/store/postgres` imports `extract` for its domain types, so
  postgres-backed tests cannot live in-package (import cycle).

## Deviations from Plan

**1. [Rule 3 — missing seam] `jobs.SubmitterKey` exported +
`JobsRepo.CountOwnedJobs` added**
- **Found during:** Task 1
- **Issue:** Owner-scoped job reads require the submitter binding that lived
  unexported in `internal/jobs`; job lists need a total for the paged API.
- **Fix:** one-line export wrapper in `internal/jobs/service.go`; a COUNT
  query in `jobs_repo.go` (files beyond the plan list, minimal additions on
  the consumed seam).
- **Committed in:** `1da4483`

**2. [Rule 3 — missing critical] `ProcessOutput`/`JobView` keep legacy JSON
field names**
- **Found during:** Task 2
- **Issue:** dashboard.js renders `job.id`, `job.template_id`, `job.status`,
  `job.model_used`, `data.default_model`, `tokens_used` — the plan demands
  consumer JSON names survive.
- **Fix:** `JobView`/`ResultView`/`Template` carry matching JSON tags while
  Go field names follow the postgres schema (`Enabled`, `DefaultModel`).
- **Committed in:** `1da4483`

**3. Dashboard password mode has no principal**
- **Found during:** Task 2
- **Issue:** `dashboardPartialsAuth` password mode stores no server-side
  tenant principal; the plan defers session-cookie principals to 01-16/01-20.
- **Fix:** extract partials resolve `auth.PrincipalFromContext` (works under
  the api_key auth mode and future session middleware) and return a truthful
  401 when absent — the fabricated `Process(ctx, 0, …)` call is deleted.
- **Committed in:** `1da4483`

**Total deviations:** 3 auto-fixed. **Impact on plan:** additive seam exports
and truthful-denial handling only; no behavior change outside the port.

## Issues Encountered

- `ENCLAVA_TEST_DATABASE_URL` unset in the executor environment; verified
  against the same local test DSN used by 01-06/01-07/01-10/01-11
  (`postgres://enclava:enclava@localhost:5432/enclava_test`).
- `extract` ↔ `postgres` import cycle forced the test split described above.

## Next Phase Readiness

- 01-19: `dashboard.go` stat reads remain on the legacy sqlite extract
  fields (documented in main.go); this plan removed every other sqlite use.
- 01-25: `jobs.Runner` is constructed in main with `extract.process`
  registered — starting the loop is the only remaining dispatch step.
- 01-14/01-15/01-16/01-22: sqlite `templatesRepo`/`settingsRepo`/`jobsRepo`
  constructions in main.go were renamed `extract*Repo` and now serve only
  `httpx.NewApp` stats params.

## Self-Check: PASSED
