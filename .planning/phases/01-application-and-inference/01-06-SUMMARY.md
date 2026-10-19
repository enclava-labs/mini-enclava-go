---
phase: 01-application-and-inference
plan: "06"
type: summary
status: complete
requirements: [CORE-02, CORE-03, JOB-01, JOB-03]
---

# 01-06 Summary — Durable jobs, exact billing and tenant-scoped domain schema

## What landed

- `internal/store/postgres/migrations/004_artifacts_jobs.sql` —
  `artifacts` (kind `original|result|derived`, state
  `writing|final|deleting|tombstoned`, server-composed `storage_path`,
  `lease_owner`/`lease_expires_at`/`fence_token` so Begin persists the
  writing row before bytes exist and Finalize/Reconcile verify the same
  row+fence; `tombstone_reason` required when tombstoned; composite FK to
  `source_object_versions`). `jobs` (`UNIQUE(tenant_id, operation,
  idempotency_key)`, frozen `principal` JSONB + `submitter_key` owner
  binding, `fence_token`, lease fields, `dispatched_attempt_id` FK added
  after `job_attempts`; claim index on `(status, lease_expires_at)` and
  subject index on `(tenant_id, connection_id, object_id,
  observed_revision)`). `job_attempts` (composite FK to `jobs`,
  `fence_token`, `usage_ref`, `error` JSONB).
- `internal/store/postgres/migrations/005_billing.sql` — integer micros
  only: `billing_prices` (one exact rational row per unit kind,
  `rate_micros >= 0`, `unit_quantity > 0`,
  `UNIQUE(tenant_id,subject_type,subject_id,capability,unit_kind,effective_from)`),
  `billing_price_audit`, `budgets` (`amount|tokens`,
  `daily|weekly|monthly`; amount budgets bind `currency` — enforced by
  CHECK — tokens budgets carry NULL), `reservations` (non-null immutable
  `price_snapshot` JSONB, per-kind `reserved_units` JSONB, bound
  `currency`, `limit_amount_micros` BIGINT, state
  `held|settled|unknown|released`, composite FKs to `jobs`,
  `job_attempts`, `billing_prices`), `usage_records` (insert-only;
  `UNIQUE(tenant_id, reservation_id)` so a reservation settles exactly
  once; global `attempt_identity` UNIQUE; outcome
  `completed|unknown|not_dispatched`), `billing_audit`.
- `internal/store/postgres/migrations/006_domain.sql` — tenant-scoped
  `extract_templates`, `extract_settings` (one `id='default'` row per
  tenant), `prompt_templates`, `chatbots` (`UNIQUE(tenant_id,
  external_id)`), `conversations`, `messages`; `chunk_profiles` as
  immutable version rows (`PRIMARY KEY(tenant_id,id)` +
  `UNIQUE(tenant_id,profile_id,version)` — updates insert a new version,
  never mutate); `knowledge_collections` binding
  `chunk_profile_version_id` to the immutable row; `collection_items`,
  `parsed_documents`, `blocks`, `chunks` (pinned to the exact version),
  `index_generations` (`staging|published|superseded|failed`). Every
  table carries `tenant_id` plus `owner_actor_id` and/or `audience`; no
  global or APIKeyID-owned rows.
- `internal/store/postgres/jobs_billing_schema_test.go` — plain-SQL
  real-PostgreSQL assertions: idempotency-key unique fires on replay and
  coexists cross-tenant/cross-operation; reservation `price_snapshot` +
  `reserved_units` + `currency` round-trip read by a second connection;
  second `usage_records` insert per reservation conflicts; price CHECKs
  reject negative rates / zero unit_quantity and per-unit-kind
  uniqueness holds; amount budgets and reservations persist their bound
  currencies; chunk-profile versions coexist/pin/conflict correctly;
  cross-tenant `index_generations` reference fails the composite FK.

## Verification

- `ENCLAVA_TEST_DATABASE_URL=postgres://enclava:enclava@localhost:5432/enclava_test
  go test ./internal/store/postgres/ -count=1 -v` — 12/12 PASS, zero
  SKIP; migrations 001–006 apply on an empty schema each run.
- `go vet ./internal/store/postgres/` clean.
- No REAL/DOUBLE/FLOAT/NUMERIC column and no SQLite dialect in
  migrations 004–006.

## Deviations from Plan

Minor schema strengthening beyond the spelled-out columns (Rule 1 —
missing-critical auto-fix): `jobs.dispatched_attempt_id` FK is added via
`ALTER TABLE` after `job_attempts` to break the circular reference;
`budgets` carries `CHECK (kind='tokens' OR currency IS NOT NULL)`;
`artifacts` carries `CHECK (state <> 'tombstoned' OR tombstone_reason IS
NOT NULL)`; `owner_actor_id` was added to the plan-spec'd tables that
lacked an ownership column so the "owner or audience" acceptance rule
holds for every 006 table. **Total deviations:** 1 auto-fixed.
**Impact:** additive constraints only.

## Deferred (per plan)

- Repositories and service ports against these tables → wave-5 plans.
