---
phase: 01-application-and-inference
plan: "10"
subsystem: infra
tags: [jobs, postgres, leases, fencing, idempotency, workers]
status: complete
requirements: [JOB-01, JOB-02]

requires:
  - phase: 01-application-and-inference (plan 01-04)
    provides: canonical jobs/types.go contract types + auth.Principal
  - phase: 01-application-and-inference (plan 01-06)
    provides: jobs/job_attempts schema (migration 004)
  - phase: 01-application-and-inference (plan 01-08)
    provides: internal/testdb shared PG fixture convention
provides:
  - jobs.Service — Enqueue (idempotent, owner-bound, frozen principal+policy),
    Lease (atomic fenced claim incl. expired-dispatch sweep to unknown),
    Cancel/Delete/Get/List (tenant+owner scoped)
  - jobs.Claim — worker handle: Heartbeat, Progress, MarkDispatched,
    Complete, Fail, MarkUnknown, Release — every write fence-checked
  - jobs.Runner — bounded multi-worker loop: poll/lease, heartbeat watchdog,
    owner-cancel propagation, retryable-error backoff, graceful drain
  - postgres.JobsRepo — concrete JobsRepository over migration 004
affects: [extract (01-13), connectors/rag (01-14), startup wiring (01-22), billing consumers (JOB-03/04)]

actuals:
  tokens: ~5600
  tasks: 2
  commits: 2

tech-stack:
  added: []
  patterns:
    - "Conditional UPDATE on (lease_owner, fence_token) → ErrStaleFence for every worker publish"
    - "lease_expires_at doubles as not-before timestamp for queued backoff"
    - "jobs.dispatched_attempt_id + unfinished attempt ⇒ reclaim sweeps to unknown, never silent re-dispatch"
    - "context.WithoutCancel for terminal fenced writes during runner shutdown"

key-files:
  created:
    - internal/jobs/repository.go
    - internal/jobs/service.go
    - internal/jobs/runner.go
    - internal/jobs/service_test.go
    - internal/jobs/runner_test.go
    - internal/store/postgres/jobs_repo.go
    - internal/store/postgres/jobs_repo_test.go
  modified: []

key-decisions:
  - "submitter_key = sha256 of (tenant, actor, credential kind+id, session, delegation, billing subject) — idempotency replay/conflict and all reads are owner-bound, never tenant-wide"
  - "Policy equality compared as canonical JSONB in SQL (policy = $n::jsonb); key ordering canonicalized server-side, never a client-supplied hash"
  - "Claim returns *jobs.Claim embedding Lease — runner translates handler outcome via Complete/Fail/MarkUnknown/Release; the in-memory 'dispatched' flag is set only by MarkDispatched on the live claim"
  - "InputResolver is Read(ctx, artifacts.Ref) so artifacts.Store satisfies it literally; input_ref stores the artifact ID, TenantID comes from the job row"
  - "Queued-job cancel transitions immediately; running-job cancel flips on the worker's next fenced write (cancelled wins over succeeded)"
  - "Graceful shutdown releases undispatched leases (requeue); dispatched attempts keep their lease and resolve to unknown via expiry"

patterns-established:
  - "Fenced write pattern: UPDATE ... WHERE lease_owner AND fence_token, RowsAffected=0 → ErrStaleFence"
  - "Atomic claim: expired-dispatch sweep + SKIP LOCKED pick + attempt insert in one tx"
  - "Owner-scoped job reads via submitter_key — another actor's job reads as not-found"

requirements-completed: [JOB-01, JOB-02]

coverage:
  - id: D1
    description: "Enqueue freezes principal+policy before ID return; idempotent replay/conflict matrix incl. different-owner and narrowed-policy conflicts; A→B→A distinct jobs"
    requirement: JOB-01
    verification:
      - kind: integration
        ref: "internal/jobs/service_test.go#TestEnqueueExactReplayReturnsSameJob,TestEnqueueDifferentOwnerSameKeyConflicts,TestEnqueueNarrowedPolicyConflicts,TestABAObservedRevisionsAreDistinctJobs,TestEnqueueFreezesPrincipalBeforeAck"
        status: pass
      - kind: integration
        ref: "internal/store/postgres/jobs_repo_test.go#TestJobsRepoInsertIdempotentMatch"
        status: pass
    human_judgment: false
  - id: D2
    description: "Fenced lease claims: racing Lease hands job to exactly one worker; expired lease reclaim bumps fence; stale writes fail ErrStaleFence"
    requirement: JOB-02
    verification:
      - kind: integration
        ref: "internal/jobs/service_test.go#TestRacingLeaseHandsJobToExactlyOneWorker,TestExpiredLeaseReclaimedOldFenceRejected"
        status: pass
      - kind: integration
        ref: "internal/store/postgres/jobs_repo_test.go#TestJobsRepoClaimRaces,TestJobsRepoUndispatchedReclaimReleases"
        status: pass
    human_judgment: false
  - id: D3
    description: "Unknown-outcome path: expired lease with dispatched unfinished attempt sweeps to unknown and is never re-leased; undispatched expiry re-leases"
    requirement: JOB-02
    verification:
      - kind: integration
        ref: "internal/jobs/service_test.go#TestExpiredLeaseWithDispatchedAttemptBecomesUnknown,TestUndispatchedExpiredLeaseReleases"
        status: pass
      - kind: integration
        ref: "internal/jobs/runner_test.go#TestRunnerReclaimsLostWorkerUnknown"
        status: pass
    human_judgment: false
  - id: D4
    description: "Cancel before dispatch prevents lease; cancel mid-run observed by worker and flips complete to cancelled; tombstone denies stale completion and owner reads"
    requirement: JOB-02
    verification:
      - kind: integration
        ref: "internal/jobs/service_test.go#TestCancelBeforeDispatchPreventsLease,TestCancelDuringRunFlipsOnFencedComplete,TestTombstonedJobCannotBeCompletedByStaleWorker,TestCancelTerminalDeniedAndOwnershipScoping"
        status: pass
    human_judgment: false
  - id: D5
    description: "Runner: input resolution via InputResolver, retryable-error backoff requeue, fatal fail, cancel propagation into handler ctx, concurrency bound, graceful-shutdown release"
    requirement: JOB-02
    verification:
      - kind: integration
        ref: "internal/jobs/runner_test.go#TestRunnerExecutesAndCompletes,TestRunnerRequeuesRetryableThenSucceeds,TestRunnerFatalErrorFailsJob,TestRunnerCancelPropagatesToHandler,TestRunnerConcurrencyBound,TestRunnerShutdownReleasesUndispatchedLease"
        status: pass
    human_judgment: false

duration: ~55min
completed: 2026-10-19
---

# 01-10 Summary — Durable jobs service + multi-worker runner on Postgres

**Idempotent, owner-bound durable jobs with fence-tokened leases, heartbeat
watchdog, cancel/tombstone fencing and the dispatched-but-unobserved →
unknown reclaim path — proven on real PostgreSQL.**

## Performance

- **Duration:** ~55 min
- **Tasks:** 2
- **Files modified:** 7 created, 0 modified

## Accomplishments

- `jobs.Service` + `postgres.JobsRepo`: enqueue persists frozen full
  `auth.Principal` JSONB + `submitter_key` binding before the job ID is
  returned; replay requires submitter+input+canonical-policy+subject match,
  every other same-key reuse is `ErrIdempotencyConflict` (JOB-01).
- Atomic `Claim` in one tx: expired dispatched-attempt jobs sweep to
  `unknown` (never silently replayed), next runnable row claimed via
  `FOR UPDATE SKIP LOCKED` with `fence_token+1`, lease stamp and attempt row.
- `Claim` worker handle: Heartbeat/Progress/CancelRequested/MarkDispatched/
  Complete/Fail(retryable→backoff requeue via lease_expires_at not-before)/
  MarkUnknown/Release — every publish a conditional fenced update
  (`ErrStaleFence` on loss). Queued cancel transitions immediately; running
  cancel flips the worker's fenced complete to `cancelled`. Delete tombstones.
- `jobs.Runner`: bounded concurrent handler pool, heartbeat + owner-cancel
  watchdog cancelling the handler ctx, `RetryableError` classification,
  `context.WithoutCancel` terminal writes during drain, undispatched-lease
  release on shutdown. `artifacts.Store` satisfies `InputResolver` directly
  (`Read(ctx, artifacts.Ref)`; input_ref = artifact ID).

## Task Commits

1. **Task 1: jobs.Service + JobsRepo** — `c90dabf` feat(01-10)
2. **Task 2: jobs.Runner** — `52cf4ad` feat(01-10)

## Verification

- `ENCLAVA_TEST_DATABASE_URL=postgres://enclava:enclava@127.0.0.1:5432/enclava
  go test ./internal/jobs/ ./internal/store/postgres/ -count=1` — **PASS,
  zero skips** (20 jobs tests incl. all acceptance criteria + 4 repo tests;
  full postgres suite green).
- `go build ./...`, `gofmt -l`, `go vet` — clean on touched packages.
- Stale-fence, expired-lease, cancel, tombstone and idempotency matrix all
  exercised (tests listed under coverage refs).

## Decisions Made

See `key-decisions` frontmatter: owner-bound submitter_key hashing, canonical
JSONB policy comparison in SQL, `Claim` handle embedding `Lease`, not-before
requeue via `lease_expires_at`, detached-context terminal writes on drain.

## Deviations from Plan

**Auto-fixed Issues**

**1. [Rule 1 - Bug] Claim SQL rewrite during test**
- **Found during:** Task 1 test run
- **Issue:** `UPDATE ... FROM pick ... RETURNING <cols>` hit SQLSTATE 42702
  (ambiguous `id` between jobs and the CTE) and `scanJobRow` lacked
  `tenant_id` in the projected column list (empty tenant → 22P02 downstream)
- **Fix:** switched to the `(tenant_id,id) = (SELECT ... FOR UPDATE SKIP
  LOCKED)` scalar-subquery claim idiom; added `tenant_id::text` to
  `jobColumns`
- **Files:** internal/store/postgres/jobs_repo.go
- **Verification:** TestJobsRepoClaimRaces and reclaim tests pass
- **Committed in:** c90dabf

**2. [Rule 1 - Bug] Param typing in FailJob**
- **Found during:** Task 1 test run
- **Issue:** unused `$5` placeholder in the jobs UPDATE made pgx send
  untyped params (SQLSTATE 42P18/42804)
- **Fix:** renumbered parameters; `$6::timestamptz` cast on the not-before
  value
- **Files:** internal/store/postgres/jobs_repo.go
- **Verification:** TestJobsRepoRetryableRequeueBackoff passes
- **Committed in:** c90dabf

**3. [Rule 1 - Bug] Shutdown terminal writes used cancelled ctx**
- **Found during:** Task 2 test run
- **Issue:** runner `translate` issued Release on the runner ctx — already
  cancelled at shutdown — leaving the job stuck `running`
- **Fix:** `context.WithoutCancel` on the translate context; documented
- **Files:** internal/jobs/runner.go
- **Verification:** TestRunnerShutdownReleasesUndispatchedLease passes
- **Committed in:** 52cf4ad

**API shaping (within plan latitude):** `Lease` returns `*Claim` (embeds
`Lease` + job row + attempt id) so the runner never re-queries; handler
signature `func(ctx, *Claim, InputFunc) (resultRef string, err error)` —
input resolved via the bound `InputFunc`, result ref returned for the
fenced Complete write; `MarkDispatched(ctx)` takes no arg (attempt identity
is bound at claim time).

**Total deviations:** 3 auto-fixed (all Rule 1 bugs caught by the plan's own
tests). **Impact:** none beyond correctness — API surface stays minimal.

## Issues Encountered

None beyond the auto-fixed items.

## User Setup Required

None — tests need `ENCLAVA_TEST_DATABASE_URL` (real PostgreSQL), consistent
with all prior plans in this phase.

## Next Phase Readiness

- 01-13 (extract), 01-14 (rag/connectors): `jobs.Enqueue` + `Runner` handler
  registration ready; `artifacts.Store` satisfies `jobs.InputResolver`.
- 01-22: runner wiring at startup (`handlers` map may be empty), drain on
  shutdown via ctx cancel + `Run` return.
- JOB-03/JOB-04 consumers can rely on `unknown` retaining liability.

## Self-Check: PASSED

- All 7 files exist and compile; commits c90dabf/52cf4ad on
  `worktree-agent-p10-1792404100` (base c966ed3).
- Every `<acceptance_criteria>` re-verified green in the final run above.

---
*Phase: 01-application-and-inference*
*Completed: 2026-10-19*
