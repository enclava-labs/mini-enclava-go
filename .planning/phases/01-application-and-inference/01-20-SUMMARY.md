---
phase: 01-application-and-inference
plan: "20"
subsystem: httpx-dashboard
tags: [auth, session, csrf, dashboard, cutover]
status: complete
requirements: [CORE-05, AUTH-03, AUTH-04, AUTH-05]

requires:
  - phase: 01-application-and-inference (plan 01-12)
    provides: httpx.SessionAuth — HandleLogin/HandleLogout/HandleOIDCStart/
      HandleOIDCCallback/HandleLinkIdentity on server-revocable sessions,
      plus csrfFromRequest and auth.Service.AuthenticateSession/ValidateCSRF/
      RevokeSession
  - phase: 01-application-and-inference (plan 01-16)
    provides: dashboardTenantID principal convention — session cookie
      principal lands via auth.WithPrincipal from RequireSession

provides:
  - Dashboard auth is the server-side session layer end to end: the signed
    email|expiry cookie scheme (dashboardSessionValid/setDashboardSession/
    clearDashboardSession/signDashboardPayload/dashboardSessionSecret/
    dashboardCredentialsValid/secureStringEqual), the adminPassword login
    path and the api_key auth-mode fallback are all deleted
  - httpx.NewSessionAuth constructed in main (authSvc, OIDCService,
    cfg.SessionTTL, DashboardTemplates(), tenant slug "bootstrap") and passed
    through the positional NewApp as the single `sessionAuth` param;
    /dashboard/login|logout, /auth/oidc/{provider}/start|callback and
    /auth/oidc/link mount that instance
  - Every dashboard page and partial route is wrapped in
    sessionAuth.RequireSession: unauthenticated GETs redirect to
    /dashboard/login, non-GETs get 401, and cookie-authenticated mutations
    require the session CSRF token (header or script-readable cookie) — 403
    otherwise; RequireSession is nil-receiver safe
  - config.AdminEmail/AdminPassword + env parsing + validation removed;
    ADMIN_* keys die here — `enclava bootstrap` is the only first-login story
  - dashboard.js AUTH_MODE switches to 'session' and echoes the enclava_csrf
    cookie into X-CSRF-Token for HTMX partial calls and extractFetch

affects: [01-22 bootstrap CLI, 01-35 legacy principal sweep]

actuals:
  tokens: ~
  tasks: 2
  commits: 1

tech-stack:
  added: []
  patterns:
    - "Cookie-authenticated mutations validate CSRF against the stored
      per-session token inside RequireSession — same contract as
      authenticated()'s session path"

key-files:
  created:
    - .planning/phases/01-application-and-inference/01-20-SUMMARY.md
  modified:
    - internal/httpx/app.go
    - internal/httpx/dashboard.go
    - internal/httpx/dashboard_resources.go
    - internal/httpx/sessionauth.go
    - internal/httpx/dashboard/static/dashboard.js
    - internal/httpx/dashboard/templates/_layout_nav.tmpl
    - internal/httpx/dashboard/templates/page_dashboard.tmpl
    - internal/httpx/dashboard/templates/partial_keys.tmpl
    - internal/httpx/root_test.go
    - internal/httpx/openai_contract_test.go
    - internal/modules/extract/handlers_test.go
    - internal/config/config.go
    - cmd/enclava/main.go

verification:
  - go build ./... — clean
  - ENCLAVA_TEST_DATABASE_URL set: go test ./internal/httpx/ -count=1 — pass,
    zero skips
  - grep 'dashboardSession\|adminPassword\|adminEmail' internal/ cmd/ — empty
  - go test ./... — all pass except internal/rag TestQdrantClientIntegration,
    which Fatals on missing ENCLAVA_TEST_QDRANT_URL (unrelated fixture env)

deferred: []

notes:
  - TestDashboardSessionLoginFlow proves login→cookie→authorized GET, wrong-
    password inline error without cookies (including a user-less email path),
    403 on CSRF-less mutations, 200 with the session CSRF token, and
    server-side revocation on logout.
  - Pre-existing latent bug fixed along the way: partial_keys.tmpl referenced
    .Description, which auth.APIKeyRecord never had — dropped the field so
    the keys partial renders real rows.
  - Page templates keep the api_key AuthMode branches dead code; AuthMode is
    now always "session" so the API-key unlock panels never render.
---
