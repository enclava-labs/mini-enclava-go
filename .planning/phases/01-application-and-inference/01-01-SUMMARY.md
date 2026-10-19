---
phase: 01-application-and-inference
plan: "01"
subsystem: database
tags: [postgres, pgx, migrations, database-sql]

requires: []
provides:
  - internal/store/postgres with validated Open (postgres:// / postgresql:// / key=value DSNs only)
  - Embedded ordered transactional migrations with schema_migrations bookkeeping
  - WithTx transaction helper
  - RequireTestDB schema-isolated fixture for all later repository tests
affects: [all repository/service plans in phase 01 and later]

actuals:
  tokens: ~2500
  tasks: 2
  commits: 2

tech-stack:
  added: [github.com/jackc/pgx/v5]
  patterns: [per-schema test isolation via search_path, embedded fs migrations]

key-files:
  created:
    - internal/store/postgres/db.go
    - internal/store/postgres/migrations.go
    - internal/store/postgres/migrations/001_schema_migrations.sql
    - internal/store/postgres/tx.go
    - internal/store/postgres/testdb_test.go
    - internal/store/postgres/db_test.go
  modified:
    - go.mod
    - go.sum

key-decisions:
  - "DSN validation fails closed: file paths, .db suffix, file: URIs and empty strings rejected"
  - "schema_migrations table created unconditionally in Go; migration files are content-only"
  - "Test isolation via random PostgreSQL schema + search_path DSN parameter"

patterns-established:
  - "applyMigrations(ctx, fs.FS) accepts any fs.FS so tests inject failing migrations"
  - "RequireTestDB fails hard (t.Fatalf) when ENCLAVA_TEST_DATABASE_URL is unset"

requirements-completed: [CORE-02]

coverage:
  - id: D1
    description: "postgres.Open validates DSN, bounds the pool and migrates transactionally"
    requirement: CORE-02
    verification:
      - kind: unit
        ref: "go test ./internal/store/postgres/ (TestOpenRejectsNonDSN, TestMigrateEmptySchemaIdempotent, TestFailedMigrationRollsBack, TestTwoSchemasIsolated)"
        status: pass
    human_judgment: false
  - id: D2
    description: "Strict schema-isolated test fixture; suite fails without ENCLAVA_TEST_DATABASE_URL"
    verification:
      - kind: unit
        ref: "go test ./internal/store/postgres/ with env unset -> FAIL exit 1"
        status: pass
    human_judgment: false

duration: ~10min
completed: 2026-10-19
status: complete
---

# Phase 01 Plan 01: PostgreSQL store foundation

Created `internal/store/postgres`: pgx/v5-backed `Open` with DSN validation and bounded pool, embedded transactional migrations, `WithTx`, and a schema-isolated strict test fixture proven on a real PostgreSQL 17.

## Accomplishments

- `postgres.Open` accepts postgres://postgresql:// URLs and key=value DSNs; rejects empty strings, filesystem paths and file: URIs (no SQLite path per D-02)
- Migrations run inside per-version transactions; `schema_migrations` created in Go; failures roll back table and version row atomically
- `RequireTestDB` creates a random `test_<hex>` schema, sets `search_path`, migrates, and drops the schema on cleanup; unset/unreachable env = hard failure
- Tests prove: non-DSN rejection, idempotent migration, failed-migration rollback verified via second connection, two-schema isolation on one DSN

## Verification

- `go test ./internal/store/postgres/ -count=1 -v` with `ENCLAVA_TEST_DATABASE_URL` set: 4/4 PASS, zero SKIP lines
- `go test ./internal/store/postgres/ -count=1` with env unset: FAIL, exit 1
- `go build ./internal/store/postgres`, `go vet`: clean; `gofmt -l`: clean

## Deviations

None.

## Self-Check: PASSED

- All key-files.created exist on disk
- 2 commits on this branch scoped to the plan
- All acceptance criteria re-verified during final test run
