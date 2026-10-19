---
phase: 01-application-and-inference
plan: "08"
type: summary
status: complete
requirements: [AUTH-01, AUTH-03, AUTH-04]
---

# 01-08 Summary — Tenant-scoped identity: bcrypt users, PG IdentityRepo, sessions, delegation

## What landed

- `internal/auth/types.go` — full `IdentityRepository` seam covering all migration-002 tables. Every method is tenant-scoped except the two documented pre-auth locators: `FindKeyCandidates(prefix)` (bounded cross-tenant prefix scan) and `GetSessionByTokenHash` (unique hash lookup). Record types: `User`, `Role`, `ServiceAccount`, `Session` (with `CSRFToken`/`IdleExpiresAt`), `Delegation`, `AuthIdentity`, `OIDCProvider`. Legacy `Identity`/`SessionRepository`/`DelegationRepository` seams removed (superseded). `ScopePlatformAdmin` const.
- `internal/auth/service.go` — rewritten `Authenticate(ctx, plaintext)`: prefix-candidate scan + constant-time sha256/hmac-sha256 verify (HashForStorage + pepper kept), expiry/revocation verified BEFORE trusting the row, tenant/actor resolved strictly from the verified stored row; owner user/service_account status checked; `BillingSubject` = owner (fallback: the key itself, kind api_key). New `AuthenticatePassword` (bcrypt + role-scope union). Legacy `Principal.APIKeyID` = deterministic FNV-1a handle of (tenant,credential id) so unported callers keep compiling until 01-35.
- `internal/auth/sessions.go` — `IssueSession` (crypto/rand token, sha256 hash stored, per-session CSRF returned once), `AuthenticateSession` (hash lookup, absolute+idle expiry, revocation, sliding `TouchSession` renewal capped at expires_at), `RevokeSession`, `RevokeUserSessions`, `ValidateCSRF` (constant-time).
- `internal/auth/keys.go` — `IssueAPIKey`/`RevokeAPIKey` (platform.admin actor required, owner_user_id XOR service_account_id), `Allowlist` marshal contract (`{"mode":"all"}` only unrestricted form), `ValidateDelegation` (expiry/revocation, allowed_scopes ⊆ delegator role scopes, effective = delegate ∩ allowed, `DelegatedByActorID` + delegator billing subject).
- `internal/auth/users.go` — `HashPassword`/`VerifyPassword` (bcrypt DefaultCost), `Service.CreateUser` (admin-actor only).
- `internal/store/postgres/identity_repo.go` — `IdentityRepo` implementing all 26 interface methods over migration 002; `$N::uuid`/`$N::jsonb` casts; IsActive ≡ revoked_at IS NULL.
- `internal/testdb/` — shared fixture: `New(t)` creates isolated `test_<rand>` schema on ENCLAVA_TEST_DATABASE_URL, migrates, drops on cleanup, hard-fails when DSN missing (never skips, never reads .env); `SeedTenant`/`SeedUser`/`SeedServiceAccount`/`SeedAPIKey` insert real identity rows.
- `cmd/enclava/main.go` — `postgres.Open(cfg.DatabaseURL)` + `NewIdentityRepo` + `auth.NewService(identityRepo, cfg.APIKeyPepper)`; `EnsureBootstrapKey` seeding deleted (01-22 bootstrap CLI replaces it). Legacy sqlite store opens at `cfg.DataDir + "/enclava-legacy.db"` until 01-22 removes it.
- Test migrations: `internal/httpx/openai_contract_test.go` and `internal/modules/extract/handlers_test.go` authenticate against real seeded PG identities; the seeded credential's deterministic `APIKeyID` is mirrored into the sqlite `api_keys` table so unported usage/budget FKs hold until those stores port. `cmd/enclava/migrate_import_test.go` deleted per plan.
- `internal/store/sqlite/api_keys_repo.go` — retained producer reshaped to the new `APIKeyRecord` (string id, JSONB grant columns via `legacyGrant` preserving old empty/"*"=unrestricted contract).
- `go.mod` — `golang.org/x/crypto` promoted to direct require.

## Verification

- `ENCLAVA_TEST_DATABASE_URL=postgres://enclava:enclava@localhost:5432/enclava_test go test ./...` — all packages pass, zero SKIPs.
- `go build ./...` clean; `gofmt -l` clean; `go vet` clean on touched packages; no IsAdmin/ID-zero symbol in auth.
- Tests cover: happy path, wrong-secret deny, revoked/expired key deny, tenant-A key→tenant A + scoped lookup cannot expose B user, session issue→auth→revoke→deny, idle+absolute expiry, CSRF issued-only, malformed allowlist denies, delegation subset enforcement + scope-widening rejection + delegator billing, disabled-user deny, bcrypt round-trip + wrong password, all six allowlist decoder columns (all/list/empty/malformed).

## Deviations from plan

- Postgres JSONB physically cannot hold malformed JSON, so the "malformed stored row" tests use a valid-JSON non-conforming grant (`{"mode":"bogus"}`) — same deny-everything decode path is exercised; the malformed-bytes case is covered by `DecodeAllowlist` unit assertions.
- `Principal.APIKeyID` for PG keys is a deterministic FNV-1a handle of the stored credential identity (no `int64` exists on the row); documented in service.go, retired in 01-35.
- `ValidateDelegation` signature takes the authenticated delegate `Principal` plus delegator user id (tenant comes from the verified principal) instead of a bare pair — required to stay tenant-scoped.
- Legacy sqlite open moved to `cfg.DataDir + "/enclava-legacy.db"` because cfg.DatabaseURL is now a validated postgres DSN (01-03); 01-22 deletes the whole sqlite open.

## Deferred (per plan)

- Admin key CRUD on IdentityRepo (httpx keysRepo swap) → 01-16; bootstrap CLI → 01-22; legacy Principal field removal → 01-35; `allowlistSet` legacy mapping retired with the service rewrite.
