---
phase: 01-application-and-inference
plan: "05"
type: summary
status: complete
requirements: [CORE-02, CORE-04]
---

# 01-05 Summary — Identity + canonical-source schema on PostgreSQL

## What landed

- `internal/store/postgres/migrations/002_identity.sql` — `tenants` plus
  tenant-scoped `users`, `roles`, `user_roles`, `service_accounts`,
  `auth_identities`, `oidc_providers`, `oidc_states`, `sessions` (token_hash +
  revoked_at; server-side revocation), `api_keys` (CHECK: owner_user_id OR
  service_account_id — never global), `delegations`. Every tenant-owned
  table is keyed `(tenant_id, id)` and every tenant-owned reference is a
  composite `(tenant_id, ref_id)` FK.
- `internal/store/postgres/migrations/003_source.sql` — `connections`
  (kind CHECK incl. `google_drive`), `source_objects` with
  `UNIQUE(tenant_id, connection_id, kind, native_id)` plus a partial unique
  index on `(tenant_id, kind, native_id) WHERE connection_id IS NULL` so
  tenant-owned uploads dedupe in their own scope; insert-only
  `source_object_versions` (`UNIQUE(tenant_id, object_id, observed_seq)`);
  `source_identities`, `source_groups`, revision-qualified
  `source_group_memberships` and `source_grants` (`access_revision`),
  `access_snapshots` (`incomplete|complete`, unique per
  connection+kind+revision), `source_restrictions`.
- `internal/store/postgres/source_identity_test.go` — real-PostgreSQL suite:
  `TestMigrationsCompositeFKs` (pg_constraint introspection asserts every
  tenant-owned FK starts with tenant_id), `TestSourceIdentityIsolation`
  (two connections share a native_id; NULL-connection upload coexists and
  still dedupes), `TestCrossTenantVersionRejected`,
  `TestObservedVersionsInsertOnly` (A→B→A persists three rows; duplicate
  observed_seq conflicts), `TestAccessSnapshotRevisionIsolation`
  (incomplete revisions never shadow the last complete membership/grant set).

## Verification

- `ENCLAVA_TEST_DATABASE_URL=postgres://enclava:enclava@localhost:5432/enclava
  go test ./internal/store/postgres/ -count=1 -v` — 9/9 PASS, zero SKIP.
- `go vet ./internal/store/postgres/` clean.
- No SQLite dialect (no AUTOINCREMENT/BLOB affinity/rowid) in either
  migration.

## Deferred (per plan)

- Repositories against these tables → 01-09.
