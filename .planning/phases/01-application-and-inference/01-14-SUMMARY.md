---
phase: 01-application-and-inference
plan: "14"
type: summary
status: complete
requirements: [CORE-05, AUTH-05, JOB-02]

requires:
  - phase: 01-application-and-inference
    provides: source.SourceRepository + objects/versions/grants (01-09), durable jobs.Enqueue/Lease/fencing (01-10/01-13), artifacts.Store, auth.Principal context
provides:
  - postgres.RAGRepo — collections + chunk_profiles + documents projection over source_objects
  - rag.Service on auth.Principal + auth.SourceAuthorizer (pre-Qdrant grant resolution) + rag.embed fenced jobs
  - connectors.Service on source.Connection rows + connector.sync durable jobs (queued-not-running)
  - httpx rag/connectors/dashboard routes wired to principals; rag_spend.go deleted
affects: [01-15 billing/usage port, 01-25+ shared executor registers connector.sync/rag.embed]

tech-stack:
  added: []
  patterns:
    - "RAG documents are projections of source_objects/object_versions — one canonical identity"
    - "embed completes only inside attempt txn via ReplaceVersionChunksAndStage — fenced against stale workers"
    - "connection rows never contain secrets — credential_ref indirection"

key-files:
  created:
    - internal/store/postgres/rag_repo.go
    - internal/store/postgres/rag_repo_test.go
  modified:
    - internal/rag/service.go, types.go, embedding.go — principal + canonical model port
    - internal/connectors/service.go, types.go — source.Connection + jobs port
    - internal/httpx/rag.go, connectors.go, dashboard_resources.go
    - internal/chatbot/service.go — RAGSearcher interface on auth.Principal
    - cmd/enclava/main.go — postgres RAGRepo + ported services
  deleted:
    - internal/httpx/rag_spend.go

key-decisions:
  - "Search resolves authorized object set via auth.SourceAuthorizer BEFORE Qdrant (pre-query scope), not post-filter"
  - "rag.embed job re-leases safely: stale attempt on superseded version no-ops"
  - "connector.sync enqueues only; no operation handler registered on the in-process runner (01-25+ registers)"
  - "SyncSource idempotency key = connection + config fingerprint; same work replays to the same job"
  - "chatbot synthesizes minimal apikey principal when context lacks one — never fabricates platform.admin"
  - "InsertObjectVersion now persists ver.Provenance JSON (was '{}' literal) so source_uri/metadata reach Document"

requirements-completed: [CORE-05, AUTH-05, JOB-02]

coverage:
  - id: D1
    description: "RAG upload→embed→search→delete on canonical model via fenced jobs"
    requirement: CORE-05
    verification:
      - kind: integration
        ref: "internal/rag/service_test.go#TestServiceUploadIndexSearchDelete"
        status: pass
    human_judgment: false
  - id: D2
    description: "Tenant + grant + audience deny paths; no Access/IsAdmin remain"
    requirement: AUTH-05
    verification:
      - kind: integration
        ref: "internal/rag/service_test.go#TestServiceTenantAndGrantDeny"
        status: pass
      - kind: other
        ref: "grep -rn 'sqlite\\.|IsAdmin|Access{' internal/rag internal/connectors internal/httpx/rag.go internal/httpx/connectors.go → empty"
        status: pass
    human_judgment: false
  - id: D3
    description: "Stale embed attempt cannot clobber newer version (fence per version)"
    requirement: JOB-02
    verification:
      - kind: integration
        ref: "internal/rag/service_test.go#TestEmbedFenceStaleAttemptCannotClobberNewerVersion"
        status: pass
    human_judgment: false
  - id: D4
    description: "connector.sync durable queued job; re-sync idempotent; cross-tenant deny"
    requirement: JOB-02
    verification:
      - kind: integration
        ref: "internal/connectors/service_test.go#TestSyncSourceIdempotentEnqueue, TestCrossTenantConnectionDeny"
        status: pass
    human_judgment: false
  - id: D5
    description: "Real Qdrant round-trip"
    verification:
      - kind: integration
        ref: "internal/rag/qdrant_integration_test.go (ENCLAVA_TEST_QDRANT_URL)"
        status: unknown
    human_judgment: true
    rationale: "No Qdrant instance reachable in this environment (docker socket denied); test correctly fails when URL unset. Must be run where Qdrant is provisioned."
---

# 01-14 Summary — RAG + connectors on canonical source model, jobs, principals

**RAG documents are now projections of `source_objects`/`object_versions`; embed and sync flow through durable fenced jobs; all `Access`/`IsAdmin` types deleted.**

## Accomplishments
- `postgres.RAGRepo`: collections, chunk_profiles (immutable per-(size,overlap) versions), documents as source-object projections, staged/published generation rows
- `rag.Service`: every method takes `auth.Principal`; Search pre-scopes via `auth.SourceAuthorizer` before Qdrant; upload writes version+artifact refs then enqueues `rag.embed`
- `attemptEmbed` stages generation + replaces chunks in one txn (fenced by `object_versions.observed_seq`); superseded attempts no-op; billing settled via final usage when `*billing.Service` is wired
- `connectors.Service`: `Connection` CRUD on `source_connections` (no secrets in rows); `Sync` enqueues `connector.sync` with principal snapshot + connection/config idempotency key; job persists queued until a shared executor registers the handler
- httpx rag/connectors/dashboard handlers pass the request principal; `rag_spend.go` deleted (billing moves to service layer); chatbot passes through `auth.PrincipalFromContext`, never fabricates `platform.admin`
- `InsertObjectVersion` now writes `provenance` JSON so `source_uri`/metadata reach the Document projection

## Task Commits
1. `feat(01-14): canonical rag service + postgres repo + fenced rag.embed jobs`
2. `feat(01-14): connectors on source.Connection + durable connector.sync jobs`
3. `feat(01-14): port rag+connectors callers to principals, delete rag_spend, rewire main`

## Verification
- `go build ./...` ✓, `go vet ./...` ✓
- `ENCLAVA_TEST_DATABASE_URL=postgres://enclava:enclava@localhost:5432/enclava go test ./internal/rag/ ./internal/connectors/ ./internal/httpx/ ./internal/store/postgres/ -count=1` → all pass except `TestQdrantClientIntegration` (correctly fails: ENCLAVA_TEST_QDRANT_URL unset, no Qdrant reachable in this environment)
- Verification greps for `sqlite.`/`IsAdmin`/`Access{` in ported files → clean

## Deviations
- `internal/store/postgres/rag_repo_test.go` placed in `package postgres_test` (import cycle with `internal/testdb` in same-package tests — established convention for testdb-using tests)
- `rag service_test.go` moved to `package rag_test` for the same cycle
- `CONNECTOR_SYNC_INTERVAL` scheduler removed from main wiring; scheduled sync now flows through durable jobs (pending executor registration)
- Legacy `linear` source type rejected (no canonical adapter); allowed credential_ref names restricted to `CONNECTOR_<TYPE>_TOKEN`
