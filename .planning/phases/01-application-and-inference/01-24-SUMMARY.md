---
phase: 01-application-and-inference
plan: "24"
title: "Expose tenant-scoped identity, role and service-key administration"
status: complete
requirements: [AUTH-01, AUTH-02, AUTH-03, AUTH-04, UI-01]
---

# 01-24 Summary

Tenant-scoped identity administration is now served by real authorized APIs
under `/api/v1/identities` over the phase-1 auth services — no shared
administrator password or public signup.

## Files changed

- `internal/httpx/identities.go` — new API: `users` (list/create/patch/
  password/roles/sessions/identities), `roles` (list/create/read with member
  ids), `service-accounts` (list/create/patch status), `me` (self account,
  password change re-proving the current credential, session list/revoke,
  linked identities) and `POST /api/v1/identities/link` which redeems the
  existing explicit OIDC link challenge for the authenticated account only
  (rejects when the challenge resolves to a different user).
- `internal/httpx/identities_test.go` — real-PostgreSQL regressions: tenant
  scoping (foreign ids → 404), non-admin 403s, self-service password + session
  revocation denying the next request, explicit link success/replay denial,
  secret-material redaction, service-account disable → next-request denial,
  last-admin lockout guard, key-scope subset enforcement.
- `internal/httpx/admin.go` — key create/PATCH now deny scopes outside the
  issuing principal's grant; revoke/delete/is_active=false refuse to remove
  the tenant's last usable `platform.admin` holder; key owner defaults to the
  calling service-account principal.
- `internal/httpx/app.go` — registered identity routes through the existing
  auth + CSRF middleware; empty scope denotes "authenticated, any scope" for
  `/me` and `/link`.
- `internal/store/postgres/identity_repo.go` — added tenant-scoped
  `ListUsers`, `ListRoles`, `GetRole`, `RoleUsers`, `UnassignRole`,
  `ListServiceAccounts`, `ListSessionsForUser`, `ListAuthIdentitiesForUser`
  and `AdminHolderCounts` (active users with the scope via roles + active
  keys with active owners). `AssignRole` is now idempotent.

## Verification (observed)

`ENCLAVA_TEST_DATABASE_URL=postgres://enclava:enclava@127.0.0.1:5432/enclava`
ran `go test -count=1 ./internal/httpx ./internal/auth` — ok; full
`go test -count=1 ./...` passes with the qdrant integration URL set.

## Notes

- No public signup, SMTP or recovery flow was added; enrollment remains
  admin-controlled via `auth.Service.CreateUser`.
- Identical emails never merge accounts; linking requires the pending
  challenge plus the account's own password (AUTH-02/D-06 preserved).
- `role_id` grants, service-account scopes and key scopes are all
  caller-scope-subset checked, so request JSON can never mint new authority.
