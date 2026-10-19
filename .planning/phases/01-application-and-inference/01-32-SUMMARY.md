---
phase: 01-application-and-inference
plan: "32"
subsystem: inference administration
 tags: [inference, endpoints, dashboard, assurance]
requires: ["01-31", "01-26"]
provides: [tenant-endpoint-configuration, endpoint-assurance-status]
affects: [inference-routing, dashboard-providers]
tech-stack:
  added: []
  patterns: [tenant-scoped endpoint API, current assurance invalidation]
key-files:
  created:
    - internal/httpx/dashboard/static/providers.js
    - internal/httpx/providers_test.go
  modified:
    - internal/httpx/providers.go
    - internal/httpx/dashboard_resources.go
    - internal/httpx/dashboard/templates/partial_providers.tmpl
key-decisions:
  - "Clear endpoint assurance whenever protocol, origin, credential reference, assurance mode, private authorization, or capabilities change."
  - "Display private/local transport separately from verified confidential assurance; omit credential reference values in rendered UI."
requirements-completed: []
coverage:
  - deliverable: "Endpoint configuration changes clear stale attestation and UI distinguishes local transport from verified confidentiality."
    verification:
      - kind: test
        ref: "go test ./internal/httpx -run 'TestEndpoint(UpdateInvalidatesAssuranceOnConfigChanges|CreateRejectsCallerClaimedVerification)$' -count=1"
        status: pass
    human_judgment: true
    rationale: "No browser session exercised; endpoint UI and accessibility require live review."
  - deliverable: "Tenant processing-policy administration and actual route verification."
    verification: []
    human_judgment: true
    rationale: "Not implemented or proven in this execution."
duration: ""
completed: 2026-10-19
---

# Phase 01 Plan 32: Manage inference endpoints Summary

Implemented tenant-scoped endpoint configuration UI/API updates: credential references can be explicitly cleared; changes to endpoint routing/security configuration invalidate prior assurance; operator-submitted assurance claims remain rejected. The embedded provider panel now displays configured endpoint origin, protocol, capabilities, private/local status and verification timestamp without displaying credential references, and offers a CSRF-protected endpoint creation form to administrators. It labels legacy provider health and model listing as insufficient evidence of confidential assurance or route readiness.

## Tasks

- Task 1: Partially completed. Existing tenant-scoped endpoint CRUD was retained and its update behavior now supports credential clearing and stale-assurance invalidation. Existing authentication middleware enforces the `pricing.manage` scope and cookie CSRF. Added regression tests for origin/credential changes and caller-forged verification. No new API for processing-policy mutations was implemented; the shared inference policy service already enforces persisted restriction intersections, but those configuration controls are absent.
- Task 2: Partially completed. Added feature-local UI for endpoint creation and accurate endpoint status display. No live local/confidential inference route was run, and no restriction change/denied request, non-admin, or cross-tenant browser/API behavior was exercised. Capability/price readiness is explicitly not inferred from `/models`, but this UI does not provide model-alias routing, approved credential-reference selection, or policy editing.

## Verification

- PASS: `go test ./internal/httpx -run 'TestEndpoint(UpdateInvalidatesAssuranceOnConfigChanges|CreateRejectsCallerClaimedVerification)$' -count=1` — package compiled and both new focused tests passed.
- BLOCKED: Required `test -n "$ENCLAVA_TEST_DATABASE_URL" && go test -count=1 ./internal/httpx ./internal/inference` — environment variable is unset; required database-backed verification was not run.
- Not exercised: browser-level verification against the live composed application and real local/confidential endpoint flows.

## Deviations from Plan

**[Rule 3 - Missing prerequisite] Database-backed acceptance verification unavailable** — Found during: verification | `ENCLAVA_TEST_DATABASE_URL` is unset in this worktree environment | Ran the required command guard, which reported the missing variable; focused non-database regression tests passed | No data or files deleted | Full verification remains outstanding.

**[Rule 3 - Missing implementation surface] Processing policy controls and end-to-end route proofs remain incomplete** — Found during: Tasks 1–2 | The task requires tenant/input policy mutation controls, actual route execution/denial proofs, alias routing and credential-reference selection; this execution only implemented endpoint creation and status display and did not add those behaviors | Files modified are listed above | Not verified; acceptance criteria are not fully met.

**Total deviations:** 2 auto-handled (1 prerequisite blocker, 1 incomplete requested behavior). **Impact:** This plan is not fully verified or complete; database integration, policy controls, actual route behavior, and live UI access checks remain necessary. No files were deleted.

## Self-Check: FAILED

The focused regression tests pass, but required database-backed verification and several plan acceptance criteria were not satisfied.
