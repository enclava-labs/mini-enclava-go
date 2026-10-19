---
phase: 01-application-and-inference
plan: "32"
subsystem: inference administration
tags: [inference, endpoints, dashboard, assurance, processing-policy]
requires: ["01-31", "01-26"]
provides: [tenant-endpoint-configuration, endpoint-assurance-status, processing-restriction-administration]
affects: [inference-routing, dashboard-providers, source-policy]
tech-stack:
  added: []
  patterns: [tenant-scoped endpoint API, origin-bound credential resolution, persisted source restrictions]
key-files:
  created:
    - internal/httpx/dashboard/static/providers.js
    - internal/httpx/providers_test.go
    - internal/httpx/processing_restrictions.go
    - internal/httpx/processing_restrictions_test.go
    - internal/store/postgres/migrations/009_tenant_processing_restrictions.sql
  modified:
    - internal/httpx/providers.go
    - internal/httpx/dashboard_resources.go
    - internal/httpx/dashboard/templates/partial_providers.tmpl
    - internal/httpx/app.go
    - internal/inference/service.go
    - internal/inference/policy.go
    - internal/source/types.go
    - cmd/enclava/main.go
    - internal/httpx/root_test.go
    - internal/httpx/identities_test.go
    - internal/httpx/openai_contract_test.go
    - internal/httpx/responses_contract_test.go
    - internal/modules/extract/handlers_test.go
key-decisions:
  - "Endpoint configuration changes clear stored assurance; authorization to contact a private origin remains separate from attestation."
  - "Credential references are selectable by approved tenant/purpose metadata and resolved against the configured endpoint origin; secret values are never rendered."
  - "Saved tenant, connection, object and artifact restrictions are persisted through the shared source policy service and intersect with request constraints."
requirements-completed: [UI-02, INF-01, INF-02, AUTH-06]
coverage:
  - deliverable: "Tenant-scoped endpoints, truthful status and processing restrictions."
    verification:
      - kind: test
        ref: "go test -count=1 ./internal/httpx ./internal/inference ./internal/store/postgres ./internal/modules/extract"
        status: pass
      - kind: test
        ref: "go build ./... && go test -count=1 $(go list ./... | grep -v '/tests/contract')"
        status: pass
      - kind: browser
        ref: "Authenticated provider dashboard on worktree app :18445; endpoint edit fields, private-origin flag and processing restriction controls displayed."
        status: pass
    human_judgment: true
    rationale: "Browser exercised the local stub route and saved restriction behavior; no confidential upstream was available."
duration: ""
completed: 2026-10-19
---

# Phase 01 Plan 32: Manage inference endpoints Summary

Implemented tenant-scoped endpoint configuration through the existing registry and embedded provider page. Administrators can create, edit, and disable endpoints; select only server-approved inference credential references; configure protocol, base origin, model aliases/capabilities, assurance mode, and explicit private-origin permission. Endpoint configuration changes invalidate prior assurance. The runtime resolves credentials using tenant, inference purpose, configured origin and reference ID; no secret value is returned in provider JSON or HTML. The page displays actual endpoint capabilities, configured prices, verification state, transport failures/readiness, and missing price setup without treating `/models` success or local reachability as confidential assurance.

Added an authenticated, CSRF-protected processing restriction API and dashboard controls for tenant, source connection, input object and artifact subjects. Subject lookups are tenant-scoped. Saved assurance and denied-capability restrictions use the shared source policy registry and apply to subsequent inference requests; request-level choices cannot widen the saved policy. Added migration support for tenant restriction subjects and explicit application repository wiring.

## Tasks

- Task 1: Completed. Existing provider endpoints remain tenant-owned; credential clearing and config-change assurance invalidation are covered by regression tests. Processing policy reads/writes enforce the administrative scope, validate tenant-owned subjects and persist via the shared source policy service. Integration tests confirm cross-tenant subjects are rejected and saved tenant restrictions deny real subsequent inference.
- Task 2: Partially completed. The embedded page has endpoint/model/capability/price, credential-reference, local-origin and assurance controls. Browser verification exercised the local route and policy-denial flow, and checked the rendered providers panel and edit form. A limited-scope API key received 403 on processing-policy access and was revoked. The required confidential-route smoke could not be performed in this environment; see deviation below.

## Verification

- PASS: `node --check internal/httpx/dashboard/static/providers.js`.
- PASS: `go test -count=1 ./internal/httpx ./internal/inference ./internal/store/postgres ./internal/modules/extract` with the provided PostgreSQL and Qdrant test URLs.
- PASS: `go build ./... && go test -count=1 $(go list ./... | grep -v '/tests/contract')` with the provided PostgreSQL and Qdrant test URLs.
- PASS: Authenticated browser at `http://127.0.0.1:18445/dashboard/providers` displayed the local endpoint, its capabilities/prices, explicit local-not-confidential state, edit controls, and processing restrictions. The edit form loaded the stored endpoint fields including its local-origin permission.
- PASS: Browser route check against the local stub returned 200 without restrictions; saving a tenant chat restriction caused the actual chat route to return 403; restoring the unrestricted policy returned 200. Requiring high assurance also denied the local endpoint. The persisted state was restored to no added assurance minimum and no denied capabilities.
- PASS: Direct API authorization check with a limited-scope key returned 403; that temporary key was revoked. The dashboard actions/forms are rendered only when the session principal has `pricing.manage`.
- NOT RUN: A verified confidential route could not be exercised because this environment only supplied the local OpenAI-compatible stub endpoint; there was no configured/reachable confidential provider, attestation credential, or approved confidential credential reference.

## Deviations from Plan

**[Rule 3 - Missing live prerequisite] Verified confidential route smoke unavailable** — Found during: Task 2 verification | The running app was configured only with the local OpenAI-compatible stub at `127.0.0.1:18443`, which has no confidential attestation provider or approved confidential credential/reference. No real confidential upstream was supplied. Fabricating assurance or treating local health as confidentiality would violate the plan, so the verified-confidential-route request could not be exercised. No files were deleted.

**Implementation scope beyond declared plan files** — Added processing-restriction API/service wiring, repository migration, application constructor dependency and affected callers/tests to implement the plan's required processing-policy controls and integration coverage. No files were deleted.

**Total deviations:** 1 live-environment verification blocker; additional implementation files recorded above. **Impact:** endpoint and processing-policy behavior is implemented and tested, but the plan's live verified-confidential-route criterion remains unverified.

## Self-Check: FAILED

All available build, test, local-route and policy-denial checks passed. Self-check remains failed because the required real verified-confidential-route smoke could not be performed with the supplied local-only upstream; the plan's success criteria are not all empirically met.
