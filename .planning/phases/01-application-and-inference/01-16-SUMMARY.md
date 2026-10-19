---
phase: 01-application-and-inference
plan: "16"
subsystem: auth-httpx
tags: [postgres, identity, api-keys, session, csrf, allowlist]
status: complete
requirements: [CORE-05, AUTH-03, AUTH-04]

requires:
  - phase: 01-application-and-inference (plan 01-08)
    provides: postgres.IdentityRepo + testdb fixture, auth.Service on
      IdentityRepository (Authenticate/AuthenticateSession/ValidateCSRF),
      seeded PG identities in httpx/module tests
  - phase: 01-application-and-inference (plan 01-12)
    provides: sessionauth cookie names (enclava_session) and
      csrfFromRequest helper shared by authenticated()
provides:
  - httpx.App identity group on postgres.IdentityRepo — `keysRepo` and every
    `sqlite.APIKeysRepo` reference removed from httpx and cmd; NewApp keeps
    its positional shape
  - admin key CRUD (list/create/get/update/delete/restore/regenerate)
    tenant-scoped on IdentityRepo + new IdentityRepo.SetAPIKeySecret for
    secret rotation; string key IDs; hashes still redacted; billing audit
    keyed by auth.LegacyAPIKeyID
  - authenticated() dual-credential path: Bearer/X-API-Key → Authenticate,
    enclava_session cookie → AuthenticateSession with X-CSRF-Token enforced
    on cookie-authenticated mutations
  - allowlist fail-closed rewrite: all/list semantics only — empty, absent
    or malformed grants deny 403; endpoint grants hold inference endpoint
    IDs (route families, "/v1" alias-normalized) enforced by
    auth.EndpointAllowed at dispatch; the URL-path-pattern check and
    openai_passthrough's route-prefix gating are deleted
    (internal/httpx/endpoint_policy.go removed)
  - sqlite.UsageRepo.KeyTotals so the legacy EnforceBudget lifetime path
    survives without an APIKeysRepo handle (both die in 01-17)

affects: [01-17 billing port, 01-20 dashboard session rework, 01-22 sqlite removal, 01-34/01-35 legacy field removal]

actuals:
  tokens: ~
  tasks: 2
  commits: 1

tech-stack:
  added: []
  patterns:
    - "Endpoint allowlists carry route-family endpoint IDs (chat/completions, embeddings, models, responses, <family>); URL paths are never grant items and HTTP routes remain scope-gated"
    - "Credential resolution order in one middleware: API key → session cookie; cookie-authenticated non-GET requires ValidateCSRF; rate-limit keys use CredentialID"
    - "Allowlist enforcement applies only to CredentialKindAPIKey principals — session principals carry no key grants"

key-files:
  created: []
  modified:
    - internal/httpx/app.go
    - internal/httpx/admin.go
    - internal/httpx/budgets.go
    - internal/httpx/dashboard.go
    - internal/httpx/openai_passthrough.go
    - internal/httpx/responses.go
    - internal/httpx/middleware_test.go
    - internal/httpx/budgets_test.go
    - internal/httpx/openai_contract_test.go
    - internal/store/postgres/identity_repo.go
    - internal/store/sqlite/usage_repo.go
    - internal/auth/service.go
    - cmd/enclava/main.go
  deleted:
    - internal/httpx/endpoint_policy.go

key-decisions:
  - "Admin create requires a stored owner: owner_user_id/service_account_id fields added, defaulting to the calling user's UserID — the api_keys CHECK constraint forbids global keys"
  - "auth.LegacyAPIKeyID exported (renamed from private legacyKeyID) so dashboard usage/budget rows keep joining legacy numeric usage_records.api_key_id until 01-17"
  - "Dashboard key sections resolve tenantID from PrincipalFromContext; password-cookie mode gets a truthful 401 (session principal lands with 01-20), matching the extractPrincipal convention"
  - "Delete/restore map to RevokeAPIKey / IsActive=true update — the postgres identity row has no soft-delete column"
  - "EnforceBudget gate relaxed to usageRepo!=nil: budgets reference legacy numeric handles only, no key-row read needed; lifetime totals come from usageRepo.KeyTotals"

requirements-completed: [CORE-05, AUTH-03, AUTH-04]

coverage:
  - id: D1
    description: "Admin key CRUD + dashboard key sections run tenant-scoped on postgres IdentityRepo; hashes redacted; audit uses LegacyAPIKeyID"
    requirement: CORE-05
    verification:
      - kind: unit
        ref: "go build ./... + gofmt gate; ENCLAVA_TEST_DATABASE_URL go test ./internal/httpx"
        status: pass
    human_judgment: false
  - id: D2
    description: "Session cookie authenticates via AuthenticateSession; cookie-authenticated mutations require the issued CSRF token"
    requirement: AUTH-03
    verification:
      - kind: integration
        ref: "internal/httpx/middleware_test.go#TestSessionCookieAuthAndCSRFGuard"
        status: pass
    human_judgment: false
  - id: D3
    description: "Allowlists fail closed: empty/absent/malformed grants deny; list-mode endpoint IDs dispatch listed routes and 403 unlisted; IP grants keep CIDR/proxy matching"
    requirement: AUTH-04
    verification:
      - kind: integration
        ref: "internal/httpx/middleware_test.go#TestEndpointGrantEmptyAndMalformedDenyAll, TestEndpointGrantListModeDispatchesListedEndpointOnly, TestIPGrantListModeAllowsListedIPDeniesOthers; openai_contract_test.go#TestEndpointRestrictionsTreatOpenAIAliasesAsEquivalent, TestOpenAIAPIKeyAllowedIPsSupportsCIDRAndTrustedProxy"
        status: pass
    human_judgment: false
---

# 01-16 Summary — httpx identity group on postgres + session/CSRF middleware

**The App's `keysRepo *sqlite.APIKeysRepo` is gone: admin key CRUD, dashboard key sections and main all run on the tenant-scoped `postgres.IdentityRepo`, and `authenticated()` now accepts the dashboard session cookie with CSRF enforcement on mutations.**

## Accomplishments
- `app.go`: `identity *postgres.IdentityRepo` replaces `keysRepo` in struct + positional constructor; `authenticated()` resolves API key → session cookie (`authSvc.AuthenticateSession`), validates `X-CSRF-Token` (header, cookie fallback) via `ValidateCSRF` on cookie-authenticated non-GET; post-auth rate limits key on `CredentialID`
- Allowlist fail-closed rewrite: `enforceAllowlists` uses `AllowedEndpointsAll`/`AllowedIPsAll` + item maps — absent/empty/malformed grants deny 403; endpoint grants hold route-family endpoint IDs (`inferenceEndpointID` normalizes `/v1` aliases; `/api/v1/responses/x/cancel` → "responses"), enforced by `auth.EndpointAllowed`; URL-path matching deleted with `endpoint_policy.go`; only API-key credentials carry grants
- `admin.go` rewritten: list/get/update/delete/restore/regenerate on `IdentityRepo` (string IDs), `SetAPIKeySecret` added for rotation, admin payload `allowed_endpoints` documents endpoint IDs; `nil` allowlist field or `"*"` encodes `{"mode":"all"}`, any list encodes `{"mode":"list"}`; audit subject uses `auth.LegacyAPIKeyID`
- `dashboard.go`: keys/usage/budgets sections on `ListAPIKeys` with `dashboardTenantID` (principal-sourced; password mode → 401 like extractPrincipal); usage rows join via `auth.LegacyAPIKeyID`; `parseDashboardID` deleted
- `openai_passthrough.go`/`responses.go`: `openAIProxyEndpointAllowed` and `openAIProxyRoutePrefixes` deleted — dispatch auth is scopes + endpoint-ID allowlist; `openAIProxyPathSafe` retained for transport safety
- `budgets.go`: lifetime totals via new `usageRepo.KeyTotals` (api_keys counters live with the usage repo until 01-17)
- `main.go`: passes `identityRepo`; `sqlite.NewAPIKeysRepo` construction removed
- Tests: middleware_test gains fail-closed endpoint grant, list-mode endpoint/IP, and session-cookie+CSRF cases on the testdb fixture; contract test rewritten for endpoint-ID grants; budgets_test seeds sqlite rows directly

## Task Commits
1. `feat(01-16): identity group on postgres IdentityRepo + session-aware authenticated()`

## Verification
- `go build ./...` clean; `gofmt -l internal/httpx cmd/enclava` empty
- `ENCLAVA_TEST_DATABASE_URL=… go test ./internal/httpx/ -count=1` — pass
- `go test ./internal/auth ./internal/store/postgres ./internal/modules/extract ./internal/store/sqlite` — pass
- `grep -rn 'keysRepo\|sqlite.APIKeysRepo' internal/httpx/ cmd/` — empty

## Deviations
- `authenticated()` lives in `app.go` (not middleware.go, as the plan's file list suggested); middleware.go carries only transport middleware — the dual-credential logic went where the function already was
- Dashboard per-key budget-limit display dropped (columns don't exist on postgres identity rows); the budgets partial still reports active keys + lifetime usage, full billing budgets land in 01-17
- `internal/rag` integration test fails without ENCLAVA_TEST_QDRANT_URL — pre-existing env gate, unrelated
