---
phase: 01-application-and-inference
plan: "09"
type: summary
status: complete
requirements: [CORE-04, AUTH-05, AUTH-06]
---

# 01-09 Summary — Canonical source repository + fail-closed source Authorizer

## What landed

- `internal/source/repository.go` — `SourceRepository` narrow-interface seam: connections (create/get/list/status/sync state), objects (upsert-or-return-existing, get, list-by-connection, mark-deleted, tombstone), immutable versions (monotonic `observed_seq`), source identities/groups/memberships, revision-qualified grants, `BeginSnapshot`/`LatestCompleteRevision`, atomic `ApplyAccessSnapshot`/`ApplyMembershipSnapshot`, and restrictions. `source.Tx` interface satisfied by `*sql.Tx` (types-only `database/sql` import). `ErrDenied` is the single public denial outcome; `ReadOptions` carries `Now` + `RequestedRestriction`.
- `internal/source/types.go` — `SourceIdentity`/`SourceGroup` gained tenant-scoped row `ID` fields (grants/memberships reference rows, not native IDs); `SourceGrant`/`SourceGroupMembership` gained `ConnectionID`/`AccessRevision`.
- `internal/source/authorize.go` — `Authorizer{repo, MaxStale}` with `CanRead`, `CanReadVersion`, `EffectiveRestrictions`. Decision order: deleted/tombstoned or disabled/deleted connection → `ErrDenied`; then `owner` (actor or `DelegatedByActorID`), `audience`/`published_audience` (explicit `Audience` membership), `source_acl` (latest COMPLETE grants snapshot inside MaxStale, mapped `source_identities` via `mapped_user_id`, group membership at latest complete membership revision — **deny wins first**). Missing mapping, stale/incomplete/absent snapshots, or no applicable allow all deny identically. `checkNarrowing` rejects request options that would drop a persisted denied capability or lower required assurance — restrictions only tighten.
- `internal/store/postgres/source_repo.go` — `SourceRepo` implementing the seam over migration 003. `UpsertObject` uses the partial unique index for NULL-connection uploads; `InsertObjectVersion` enforces `observed_seq > max(existing)` inside `WithTx`; `Apply*Snapshot` writes revision-qualified rows, marks the snapshot complete and prunes unseen rows via `NOT EXISTS` deletes — all in the caller's transaction; `LatestCompleteRevision` returns `ErrDenied` when none exists.
- Tests: `internal/source/authorize_test.go` (real-PG fixtures via `internal/testdb`) and `internal/store/postgres/source_repo_test.go`.

## Verification

- `ENCLAVA_TEST_DATABASE_URL=postgres://enclava:enclava@localhost:5432/enclava go test ./internal/source/ ./internal/store/postgres/ -count=1` — all PASS, zero SKIPs (33 tests in postgres incl. prior suites; 7 top-level + 6 subtests in source).
- Denial matrix exercised: no-grant, stale snapshot, incomplete-only snapshot, missing mapping, tombstoned-with-grant, disabled connection, unknown-vs-forbidden identical `ErrDenied`.
- `gofmt -l`, `go vet`, `go build ./...` clean.

## Deviations from plan

- `ApplyAccessSnapshot`/`ApplyMembershipSnapshot` take `source.Tx` (interface) instead of concrete `*sql.Tx` so `internal/source` imports `database/sql` only for `sql.Result`; plan signature semantics preserved.
- `InsertObjectVersion` takes `tenantID` explicitly because `ObjectVersion` has no tenant field (composite PK requires it).
- Fixture `RequireTestDB` does not cross package boundaries; `internal/source` tests use the shared `internal/testdb` fixture (same isolated-schema contract).
- Unknown `required_assurance` values rank above all known levels (fail-closed ordering) — only "standard"/"high" are named ranks.

## Deferred (per plan)

- Connector observation flows (04-01) apply snapshots through this repo; no consumer wiring here.
