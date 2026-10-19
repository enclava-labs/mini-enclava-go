---
phase: 01-application-and-inference
plan: "12"
subsystem: auth
tags: [oidc, pkce, nonce, sessions, postgres, templates]

requires:
  - phase: 01-application-and-inference
    provides: "01-08 identity layer: IdentityRepository over migration 002, IssueSession/AuthenticateSession/ValidateCSRF, bcrypt users"
provides:
  - "auth.OIDCService: BeginFlow (state+nonce+PKCE, hashed state row, return_to allowlist) and CompleteFlow (atomic single-use consume, code exchange, ID-token signature/claims/nonce verification)"
  - "Identity linking rules: issuer+subject resolves only via stored auth_identities; unlinked identities get a one-shot link challenge confirmed by local credentials; linked_by recorded; disabled users deny"
  - "httpx.SessionAuth: HandleLogin/HandleLogout/HandleOIDCStart/HandleOIDCCallback/HandleLinkIdentity + RequireSession middleware on server-revocable sessions"
affects: ["01-16", "01-20", "01-24", "01-26"]

actuals:
  tokens: ~5500
  tasks: 2
  commits: 2

tech-stack:
  added: ["github.com/coreos/go-oidc/v3 v3.17.0", "golang.org/x/oauth2 v0.36.0"]
  patterns: ["single-use state rows via atomic UPDATE...WHERE consumed_at IS NULL", "pending-link challenges persisted in oidc_states (subject/email/pending_link_user_id, migration 007)", "client_secret_ref resolved only via env:<NAME> binding"]

key-files:
  created:
    - internal/auth/oidc.go
    - internal/auth/linking.go
    - internal/auth/oidc_test.go
    - internal/httpx/sessionauth.go
    - internal/store/postgres/migrations/007_oidc_link_states.sql
  modified:
    - internal/auth/types.go
    - internal/store/postgres/identity_repo.go
    - internal/httpx/dashboard.go
    - internal/httpx/dashboard/templates/login.tmpl
    - go.mod
    - go.sum

key-decisions:
  - "Link challenges are persisted as oidc_states rows (nonce='link', pkce='-', subject/email/pending_link_user_id columns from migration 007) so the verified issuer+subject survives callback→confirm server-side; browser re-asserts nothing."
  - "No auto-provisioning on the login path: an unknown issuer+subject always produces a link challenge; the proven local account binds. 'member-role provisioning' narrows to the explicit confirm path since no provider provisioning-policy column exists."
  - "tenant resolves from a slug via new GetTenantBySlug (browser asserts only the slug); sessionauth.NewSessionAuth takes (authSvc, oidcSvc, sessionTTL, templates, tenantSlug)."
  - "Identity getters now map ErrNoRows/invalid-UUID to auth.ErrNotFound so deny paths distinguish absence from store failure; wrapped errors keep errors.Is working."
  - "Session cookie Path=/ (not /dashboard) so /auth/oidc/* callback and link paths carry it."

patterns-established:
  - "Consume-then-verify: state rows flip consumed_at atomically before expiry checks — a race can only over-deny, never replay."
  - "ConfirmLink peeks the challenge (GetOIDCState), verifies the password, THEN consumes — a wrong password never burns the one-shot token."

requirements-completed: [AUTH-02, AUTH-03]

coverage:
  - id: D1
    description: "OIDC begin→complete round-trip against a stub provider verifies signature, issuer, audience and nonce and resolves a linked user"
    requirement: AUTH-02
    verification:
      - kind: integration
        ref: "internal/auth/oidc_test.go#TestOIDCRoundTripLinkedIdentity"
        status: pass
    human_judgment: false
  - id: D2
    description: "State single-use, expiry, external return_to, nonce mismatch and disabled provider/user all deny"
    requirement: AUTH-02
    verification:
      - kind: integration
        ref: "internal/auth/oidc_test.go#TestOIDCStateReplayDenied,TestOIDCExpiredStateDenied,TestOIDCExternalReturnToRejected,TestOIDCNonceMismatchDenied,TestOIDCDisabledUserDenied,TestOIDCDisabledProviderDenied"
        status: pass
    human_judgment: false
  - id: D3
    description: "Unlinked identity never gains silent access — link challenge requires the candidate account's password, other accounts and replay are denied, linked_by recorded"
    requirement: AUTH-02
    verification:
      - kind: integration
        ref: "internal/auth/oidc_test.go#TestOIDCUnlinkedIdentityRequiresLinkStep"
        status: pass
    human_judgment: false
  - id: D4
    description: "sessionauth handlers exist and compile-mounted: login issues a real revocable session, logout revokes server-side with CSRF, OIDC callback/link handlers consume via CompleteFlow/ConfirmLink"
    requirement: AUTH-03
    verification:
      - kind: other
        ref: "go build ./... (sessionauth compiles against the package as it stands); end-to-end mount proof deferred to 01-20 root_test.go per plan split"
        status: pass
    human_judgment: true
    rationale: "Handlers are constructed/mounted by 01-20; the HTTP round-trip (cookie→authorized GET→logout revoke) is that plan's task-2 proof, so this deliverable still needs the human/integration UAT at the mounted surface."

duration: ~20 min
completed: 2026-10-19
status: complete
---

# Phase 01 Plan 12: OIDC Flow, Identity Linking and Session Handlers Summary

**OIDC authorization-code flow with PKCE + single-use hashed state rows, interactive identity linking via stored challenges, and httpx.SessionAuth login/logout/OIDC handlers on server-revocable sessions.**

## Performance

- **Duration:** ~20 min
- **Tasks:** 2
- **Files modified:** 11 (5 created)

## Accomplishments

- `auth.OIDCService` — `BeginFlow` validates return_to (relative paths only), stores hashed state + nonce + PKCE verifier (10 min TTL), returns the provider authorize URL; `CompleteFlow` consumes state atomically (replay denied), exchanges the code, verifies the ID token via discovery/JWKS (signature, iss, aud, exp, nonce) and resolves issuer+subject through linking rules.
- `linking.go` — unlinked verified identities produce `LinkRequired` + one-shot challenge row; `ConfirmLink` re-verifies the candidate account's password, records `linked_by_user_id`, denies wrong-account credentials and replay. Disabled users deny at every stage.
- `httpx.SessionAuth` — `HandleLogin` (GET render w/ enabled-provider links, POST → AuthenticatePassword → IssueSession → HttpOnly+CSRF cookies), `HandleLogout` (CSRF-validated RevokeSession), `HandleOIDCStart`/`HandleOIDCCallback`/`HandleLinkIdentity`, plus `RequireSession`/`SessionPrincipal` for 01-20's dashboard wiring.
- Migration `007_oidc_link_states.sql` extends `oidc_states` with subject/email/pending_link_user_id so link challenges reuse the single-use machinery without new tables.
- Login template renders local form, per-provider OIDC links, and the link-confirmation step.

## Task Commits

1. **Task 1: OIDC flow + identity linking** — `c796d4a` (feat)
2. **Task 2: Session HTTP handlers + login page** — `7392a09` (feat)

## Files Created/Modified

- `internal/auth/oidc.go` — OIDCService, BeginFlow/CompleteFlow, env-bound secret resolver
- `internal/auth/linking.go` — resolveIdentity, link challenges, ConfirmLink, ResolveTenantID
- `internal/auth/oidc_test.go` — stub discovery/token/JWKS provider + 8 integration tests
- `internal/auth/types.go` — OIDCState, ErrNotFound, GetTenantBySlug + OIDC-state repo methods
- `internal/store/postgres/identity_repo.go` — OIDC-state + tenant-slug impls; getters map ErrNoRows → auth.ErrNotFound
- `internal/store/postgres/migrations/007_oidc_link_states.sql` — pending-link columns
- `internal/httpx/sessionauth.go` — session/OIDC handler set
- `internal/httpx/dashboard.go` — dashboardLoginData extended (Providers/ReturnTo/Link* fields)
- `internal/httpx/dashboard/templates/login.tmpl` — local + OIDC + link forms
- `go.mod`, `go.sum` — go-oidc/v3 v3.17.0, oauth2 v0.36.0

## Decisions Made

- State/link-challenge persistence reuses `oidc_states` (migration 007) instead of a new table — the single-use `consumed_at` machinery is identical.
- `client_secret_ref` resolves only `env:<NAME>` refs via `EnvSecretResolver` — arbitrary lookups denied; tests inject a stub resolver env var.
- `SessionAuth` resolves tenant UUID per request from a configured slug — no order dependency between binary start and `enclava bootstrap`.
- CSRF token ships in a non-HttpOnly `enclava_csrf` cookie so dashboard JS can echo `X-CSRF-Token`; the server compares against the stored per-session token.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 2 — Missing Critical] IdentityRepository had no oidc_states or tenant-slug methods**
- **Found during:** Task 1
- **Issue:** Plan's `files_modified` listed no repo files, but `oidc_states` storage and slug→UUID tenant resolution are unreachable without new interface methods.
- **Fix:** Added `GetTenantBySlug`, `CreateOIDCState`, `GetOIDCState`, `ConsumeOIDCState` + `OIDCState` type; implemented in `identity_repo.go`.
- **Files modified:** internal/auth/types.go, internal/store/postgres/identity_repo.go
- **Verification:** `go build ./...`; replay/consume covered by tests
- **Committed in:** `c796d4a`

**2. [Rule 2 — Missing Critical] Pending-link state needs persistence columns**
- **Found during:** Task 1
- **Issue:** A browser-driven link confirmation crosses an HTTP round trip; verified subject/email/candidate must persist server-side or the browser could re-assert them.
- **Fix:** Migration 007 adds subject/email/pending_link_user_id to oidc_states.
- **Verification:** testdb migrations apply; link-flow test passes end-to-end
- **Committed in:** `c796d4a`

**3. [Rule 1 — Bug] Getters wrapped sql.ErrNoRows as opaque errors**
- **Found during:** Task 1 (test failure: unlinked identity surfaced as store error, not "not found")
- **Issue:** `resolveIdentity`/`AuthenticatePassword` couldn't distinguish absent rows from failures.
- **Fix:** All 12 `auth.*`-returning getters map ErrNoRows/invalid-UUID → `auth.ErrNotFound` (wrapped errors still match `errors.Is(sql.ErrNoRows)`).
- **Files modified:** internal/store/postgres/identity_repo.go
- **Verification:** full auth test suite passes
- **Committed in:** `c796d4a`

**4. [Rule 2 — Missing Critical] No auto-provisioning policy exists on provider rows**
- **Found during:** Task 1
- **Issue:** Plan says "if the provider's tenant policy allows provisioning, create member user" — no such column/config exists and no member-role seeding exists.
- **Fix:** Unknown identities route through the same interactive link step; no account is silently created or bound. Strictly denies more than the plan's most permissive reading.
- **Verification:** TestOIDCUnlinkedIdentityRequiresLinkStep (unknown subject → LinkRequired)
- **Committed in:** `c796d4a`

**Total deviations:** 4 auto-fixed (2 missing-critical storage/API, 1 bug, 1 absent-policy narrowing)
**Impact on plan:** All were prerequisites for the planned behavior; the narrowing (explicit link for unknown identities) is the fail-closed choice and satisfies "no auto-provisioning of privileged access".

## Issues Encountered

- The stub provider's token endpoint reads the consumed state's nonce directly from `oidc_states`; a code-prefixed `FORGED:` probe covers nonce-mismatch without corrupting rows. This is test-only machinery.
- `Secure` cookie flag keys off `r.TLS != nil`; behind a TLS-terminating proxy the cookie is non-Secure (same convention as the dashboard code it will replace). If the deployment terminates TLS upstream, 01-20 should thread the trust-proxy config into `externalBase`/cookie flags.

## User Setup Required

Operators configuring OIDC need provider rows (`oidc_providers`) and `env:`-named client secrets — the 01-26 user-setup surface documents the flow; no new env requirements beyond the secret bindings themselves.

## Next Phase Readiness

- 01-16 can extend `IdentityRepository` freely (additive).
- 01-20 constructs `httpx.NewSessionAuth(authSvc, oidcSvc, cfg.SessionTTL, dashboardTemplates, tenantSlug)` in main, mounts `/dashboard/login|logout` + `/auth/oidc/{provider}/start|callback` + `/auth/oidc/link`, deletes the signed-cookie scheme.
- Neither AUTH-02 nor AUTH-03 marked Complete yet — shared-ID gate: 01-24/01-26 (AUTH-02) and 01-16/01-20/01-24 (AUTH-03) still declare them.

## Self-Check: PASSED

- `go build ./...` clean; `gofmt -l internal/` empty.
- `ENCLAVA_TEST_DATABASE_URL=postgres://enclava:enclava@localhost:5432/enclava_test go test ./internal/auth/ -count=1` — 20 tests pass, zero skips, including the 8 new OIDC/linking tests.
- key-links verified: sessionauth.go calls `IssueSession`/`CompleteFlow`; oidc.go resolves through `resolveIdentity`/`RecordAuthIdentity` in linking.go.

---
*Phase: 01-application-and-inference*
*Completed: 2026-10-19*
