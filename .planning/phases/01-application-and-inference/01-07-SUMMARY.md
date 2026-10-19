---
phase: 01-application-and-inference
plan: "07"
type: summary
status: complete
subsystem: storage
tags: [artifacts, postgres, lease, fencing, reconciliation, filesystem]
requirements: [CORE-03]
requires:
  - phase: 01-04
    provides: canonical artifact Ref/Meta contract types
  - phase: 01-06
    provides: artifacts table (state enum, lease_owner/lease_expires_at/fence_token, tombstone_reason)
provides:
  - internal/artifacts Store — Begin/Finalize/Read/Delete/ScratchDir/Reconcile over opaque Refs
  - artifacts.MetadataStore narrow persistence boundary
  - postgres.ArtifactRepo — concrete MetadataStore over the artifacts table
affects: [uploads, connectors, extraction results, any service retaining durable bytes]

actuals:
  tokens: ~5000
  tasks: 2
  commits: 3

tech-stack:
  added: []
  patterns:
    - "Writing-row-first: metadata INSERT with lease/fence commits before any byte exists"
    - "Conditional lease/fence UPDATE for finalize and reclaim fencing"
    - "Server-composed paths only: validated components + root-prefix proof; Ref is never a path"

key-files:
  created:
    - internal/artifacts/store.go
    - internal/artifacts/store_test.go
    - internal/artifacts/reconcile.go
    - internal/store/postgres/artifact_repo.go
    - internal/store/postgres/artifact_repo_test.go
  modified: []

key-decisions:
  - "Ref.ID is server-generated (art_<16B hex>); fence_token is a random int64 per Begin"
  - "Begin takes an explicit maxBytes; over-limit streams abort mid-Write and remove temp bytes"
  - "Tombstone reasons: deleted, write_aborted, expired_write_lease, missing_final_bytes — first reason wins on re-tombstone"
  - "UpdateState restricted to deleting|tombstoned transitions; final goes only through fence-checked FinalizeCommit"
  - "Traversal uses name→row lookup: files are deleted only when no live writing row owns them"

patterns-established:
  - "Writer streams to <root>/.tmp/<id>.part, fsync, atomic rename, dir-fsync, then conditional commit"
  - "Reconcile is three-stage: fence expired writers → traverse durable tree → row-driven repair; idempotent"

requirements-completed: [CORE-03]

coverage:
  - id: D1
    description: "Filesystem store: Begin persists writing row before bytes; limited streams to .part; fsync→rename→fence-checked commit under tenant dirs"
    requirement: CORE-03
    verification:
      - kind: unit
        ref: "internal/artifacts/store_test.go#TestBeginWritingRowBeforeBytes"
        status: pass
      - kind: unit
        ref: "internal/artifacts/store_test.go#TestWriteFinalizeRoundTrip"
        status: pass
      - kind: unit
        ref: "internal/artifacts/store_test.go#TestOverLimitLeavesNoFile"
        status: pass
      - kind: unit
        ref: "internal/artifacts/store_test.go#TestConcurrentBeginsDistinctIDs"
        status: pass
      - kind: integration
        ref: "internal/store/postgres/artifact_repo_test.go#TestArtifactRepoRoundTrip"
        status: pass
    human_judgment: false
  - id: D2
    description: "Traversal/cross-tenant denial: Ref fields with '..', '/', '\\' or empty are rejected; tenant B cannot read/delete tenant A's artifact"
    requirement: CORE-03
    verification:
      - kind: unit
        ref: "internal/artifacts/store_test.go#TestRefTraversalRejected"
        status: pass
      - kind: unit
        ref: "internal/artifacts/store_test.go#TestCrossTenantDenied"
        status: pass
      - kind: integration
        ref: "internal/store/postgres/artifact_repo_test.go#TestArtifactRepoRoundTrip"
        status: pass
    human_judgment: false
  - id: D3
    description: "Crash/traversal reconciliation: expired writers fenced before byte reap; live-lease writers survive; rowless and tombstoned files removed; missing final bytes tombstoned; idempotent"
    requirement: CORE-03
    verification:
      - kind: unit
        ref: "internal/artifacts/store_test.go#TestReconcilePureFS"
        status: pass
      - kind: integration
        ref: "internal/store/postgres/artifact_repo_test.go#TestReconcileWriterVsReconcilerBarrier"
        status: pass
      - kind: integration
        ref: "internal/store/postgres/artifact_repo_test.go#TestReconcileCrashWindow"
        status: pass
      - kind: integration
        ref: "internal/store/postgres/artifact_repo_test.go#TestReconcileExpiredWriterFenced"
        status: pass
      - kind: integration
        ref: "internal/store/postgres/artifact_repo_test.go#TestReconcileMissingFinalBytes"
        status: pass
      - kind: integration
        ref: "internal/store/postgres/artifact_repo_test.go#TestReconcileTombstonedFileRemoved"
        status: pass
    human_judgment: false
  - id: D4
    description: "Private bounded scratch directories with cleanup (cancellation/failure path)"
    requirement: CORE-03
    verification:
      - kind: unit
        ref: "internal/artifacts/store_test.go#TestScratchDirLifecycle"
        status: pass
    human_judgment: false

duration: ~25 min
completed: 2026-10-19
---

# Phase 01 Plan 07: Durable filesystem artifact store Summary

**Leased, fenced, tenant-isolated artifact writes: the 'writing' row commits before bytes exist, .part→fsync→rename→conditional-commit finalizes atomically, and Reconcile repairs every crash window without ever orphaning a live writer — backed by a real PostgreSQL ArtifactRepo.**

## Performance

- **Duration:** ~25 min
- **Started:** 2026-10-19T09:30:00Z (approx)
- **Completed:** 2026-10-19T09:56:58Z
- **Tasks:** 2 completed
- **Files modified:** 5 created, 0 modified

## Accomplishments

- `internal/artifacts` `Store`: `Begin` commits the `state=writing` row (lease_owner/lease_expires_at/fence_token) **before** opening `<root>/.tmp/<id>.part`; `Writer` enforces the byte limit mid-stream (over-limit abort removes temp bytes and tombstones the row); `Finalize` verifies lease+fence ownership, fsyncs, renames to `<root>/<tenant>/<id>` (dir 0700, dir-fsync), then commits `state=final` through the same conditional fence — a reclaimed writer gets `ErrLeaseLost` and its bytes are removed.
- Path safety: every Ref component is validated (rejects `''`, `.`, `..`, `/`, `\`, NUL) before the path is composed under the durable root and prefix-checked; `Read`/`Delete`/`ScratchDir` never take a caller path; cross-tenant reads/deletes deny with `ErrNotFound`.
- `Reconcile(ctx)`: (a) expired-lease writing rows are atomically fenced (conditional `writing→tombstoned` on lease+fence) *before* their bytes are deleted — only the fencing winner reaps; (b) durable files are deleted only when name→row lookup proves no live writing row owns them (covers rename-before-commit); (c) tombstoned/deleting rows' files removed, final rows with missing bytes tombstoned with `tombstone_reason=missing_final_bytes`. Idempotent; FS errors collected, never treated as "missing".
- `postgres.ArtifactRepo`: all statements `(tenant_id, id)`-scoped; `FinalizeCommit`/`InvalidateWriting` are conditional UPDATEs; invalid tenant UUIDs map to `ErrNotFound` (unknown = forbidden); compile-time `var _ artifacts.MetadataStore` assertion in both files.

## Task Commits

1. **Task 1: Filesystem store — stream, finalize, tenant paths, traversal safety** — `15df271` (feat)
2. **Task 2: postgres.ArtifactRepo + crash/traversal reconciliation** — `7243240` (feat)

**Plan metadata:** `docs(01-07): plan summary` (this commit)

## Files Created/Modified

- `internal/artifacts/store.go` — Store, Writer, MetadataStore interface, ArtifactRow, sentinel errors, path validation
- `internal/artifacts/store_test.go` — in-memory MetadataStore fake + 9 tests (writing-row-first, over-limit, traversal, cross-tenant, delete, fence, scratch, concurrency, pure-FS reconcile)
- `internal/artifacts/reconcile.go` — three-stage cross-process-safe Reconcile
- `internal/store/postgres/artifact_repo.go` — concrete MetadataStore over migration-004 artifacts table
- `internal/store/postgres/artifact_repo_test.go` — real-PostgreSQL tests incl. barrier-controlled writer-vs-reconciler and crash-window cases

## Verification

- `go test ./internal/artifacts/ -count=1 -v` — 9/9 PASS, zero SKIP
- `ENCLAVA_TEST_DATABASE_URL=postgres://enclava:enclava@localhost:5432/enclava_test go test ./internal/store/postgres/ -count=1 -v` — 20/20 PASS, zero SKIP
- `go build ./...` — clean; `go vet ./internal/artifacts/ ./internal/store/postgres/` — clean; `gofmt -l` — clean
- Acceptance criteria all proven by named tests: over-limit leaves no tenant file; `..`/`/` refs error; no function accepts a caller path; barrier test proves live-lease survival + expired-lease fencing; crash case repaired; second Reconcile a no-op.

## Decisions Made

- `Begin` signature includes explicit `maxBytes` (plan's streaming-limit requirement needs a bound; plan signature was approximate).
- `MetadataStore` adds `InvalidateWriting` (the plan's required "conditional UPDATE" fencing step needs an atomic conditional, which an unconditional `UpdateState` cannot provide).
- `DeleteRow` is implemented on the repo for interface completeness but unused by the production path (reconcile repairs rows/files rather than deleting rows); called out as dead-weight-avoidance deviation.
- `.part` orphans without a row are not swept (plan scope); a tombstoned-row `.part` is covered by `Delete`/`abort`/`removeOwned`.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 2 — missing critical] `MetadataStore.InvalidateWriting` added**
- **Found during:** Task 2 (reconcile implementation)
- **Issue:** Plan's interface list lacked an atomic conditional fencing operation; `UpdateState` is unconditional and could resurrect/overwrite a tombstone reason.
- **Fix:** Added `InvalidateWriting(ctx, tenant, id, leaseOwner, fenceToken, reason) (bool, error)` — conditional `UPDATE … WHERE state='writing' AND lease_owner AND fence_token`; only the winner deletes bytes.
- **Files modified:** `internal/artifacts/store.go`, `internal/store/postgres/artifact_repo.go`, fakes/tests
- **Verification:** `TestReconcileExpiredWriterFenced`, `TestFinalizeAfterFence`
- **Committed in:** `15df271`, `7243240`

**2. [Rule 2 — missing critical] `Begin` gained an explicit `maxBytes` parameter**
- **Found during:** Task 1
- **Issue:** "streaming writes with limits" requires an enforced bound; plan's signature omitted it.
- **Fix:** `Begin(ctx, meta, leaseTTL, maxBytes)`; `maxBytes <= 0` rejected; over-limit abort removes temp bytes.
- **Files modified:** `internal/artifacts/store.go`
- **Verification:** `TestOverLimitLeavesNoFile`
- **Committed in:** `15df271`

**Total deviations:** 2 auto-fixed. **Impact on plan:** additive parameters/methods required by the plan's own semantics; no scope creep, no alternate backend.

## Issues Encountered

- `ENCLAVA_TEST_DATABASE_URL` was unset in the executor environment; used the same local test DSN plan 01-06 verified against (`postgres://enclava:enclava@localhost:5432/enclava_test`). All repo tests passed against the real database.

## Next Phase Readiness

- `artifacts.Store` is the only artifact API (`Begin/Finalize/Read/Delete/ScratchDir/Reconcile`); upload/connector/result plans (01-08+) can wire `postgres.ArtifactRepo` + `Store` directly. `Reconcile` is safe to run at startup.
- CORE-03 remains unchecked in REQUIREMENTS.md — sibling plans 01-36 also declare it (shared-ID gate).

## Self-Check: PASSED
