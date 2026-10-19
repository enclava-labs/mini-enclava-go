---
phase: 01-application-and-inference
plan: "18"
subsystem: inference-httpx
tags: [postgres, inference, endpoints, providers-cutover]
status: complete
requirements: [CORE-05, INF-02]

requires:
  - phase: 01-application-and-inference (plan 01-11)
    provides: postgres.EndpointsRepo (inference.EndpointRepository over
      migration 007 — tenant-scoped Get/List/Create/Update/SetEnabled)
  - phase: 01-application-and-inference (plan 01-17)
    provides: dashboardTenantID convention + billing dashboard sections
provides:
  - httpx.App provider group on postgres — `providersRepo *sqlite.ProvidersRepo`
    → `endpoints *postgres.EndpointsRepo`; zero providersRepo/
    sqlite.ProviderConfig references remain in httpx or cmd
  - /api/v1/providers admin CRUD on inference.Endpoint: tenant-scoped to the
    request principal; list keeps type/include_disabled filters; PUT merges
    over the stored record with SetEnabled for the enabled toggle; DELETE
    disables (the endpoint seam has no destructive delete)
  - Admin payload keeps authorized_private and assurance_* distinct;
    client-set assurance_verified is rejected — verification timestamps are
    written by observed attestation only
  - main.go seeds a create-or-get env-provider endpoint row on the bootstrap
    tenant (GetTenantBySlug "bootstrap" → CreateTenant fallback);
    runtimeProviderConfig returns inference.Endpoint (base_origin stripped to
    a bare origin; http implies authorized_private) and feeds both seeding
    and the llm.Provider path
  - Dashboard providers partial carries tenant Endpoints rows alongside the
    existing prober snapshot/model list

affects: [01-20 dashboard session, 01-22 bootstrap CLI, 01-25 executor, 01-32 providers UI]

actuals:
  tokens: ~
  tasks: 1
  commits: 1

tech-stack:
  added: []
  patterns:
    - "Provider admin request type (endpointRequest) maps to canonical
      inference.Endpoint fields — legacy type/name/config keys translated at
      the boundary, never stored"

key-files:
  created:
    - .planning/phases/01-application-and-inference/01-18-SUMMARY.md
  modified:
    - internal/httpx/app.go
    - internal/httpx/providers.go
    - internal/httpx/dashboard_resources.go
    - cmd/enclava/main.go

key-decisions:
  - "DELETE on /api/v1/providers/{id} disables via SetEnabled — EndpointRepository exposes no destructive delete"
  - "Env-provider seed targets the 'bootstrap' tenant (create-or-get) until the 01-22 bootstrap CLI owns tenant creation"
  - "Client-supplied assurance_verified rejected; AssuranceVerifiedAt is repo-owned from observed attestation"

requirements-completed: [CORE-05, INF-02]

verification:
  - "go build ./... — clean"
  - "gofmt -l internal/httpx cmd/enclava — clean"
  - "ENCLAVA_TEST_DATABASE_URL go test ./internal/httpx/ -count=1 — pass"
  - "ENCLAVA_TEST_DATABASE_URL go test ./internal/modules/extract/ -count=1 — pass"
