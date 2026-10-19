---
phase: 01-application-and-inference
plan: "21"
subsystem: auth
tags: [secrets, tenant-binding, credential-ref, origin-canonicalization]

requires:
  - phase: 01-application-and-inference
    provides: inference.Endpoint.CredentialRef contract (01-20), env: OIDC binding (01-03)
provides:
  - auth.SecretResolver — tenant/purpose/origin-bound credential resolver owned by auth
  - auth.SecretBinding + auth.CanonicalizeSecretOrigin validation
  - Config.SecretBindings parsed from operator-owned SECRET_BINDINGS env var
affects: [01-25 executor composition, source adapter plans, MCP composition]

actuals:
  tokens: ~5600
  tasks: 2
  commits: 2

tech-stack:
  added: []
  patterns:
    - "Opaque credential_ref resolved only via operator-declared binding (tenant+purpose+origin+env)"
    - "ErrSecretDenied as single sanitized outcome for every refused resolution"
    - "Canonical scheme://host[:port] origin matching — no substring/prefix origin checks"

key-files:
  created:
    - internal/auth/secret_refs.go
    - internal/auth/secret_refs_test.go
  modified:
    - internal/config/config.go
    - internal/config/config_test.go
    - internal/auth/oidc.go

key-decisions:
  - "OIDC's func-type SecretResolver renamed ClientSecretResolver; the concrete bound resolver owns the SecretResolver name per CONTRACTS.md"
  - "SECRET_BINDINGS format: ref_id|tenant_id|purpose|origins|env_name entries separated by ';', origins comma-separated"
  - "config imports auth for the binding type; auth never imports config — acyclic per plan"
  - "Empty credential_ref resolves to empty credential: explicitly unauthenticated endpoints never inherit SDK/env defaults"

patterns-established:
  - "Binding check before env read: a resolver can never reach os.LookupEnv on an unbound ref"
  - "Metadata surface (SecretRefMeta) exposes identity only — never values, env names or origins"

requirements-completed: [AUTH-05, INF-01, INF-02]

coverage:
  - id: D1
    description: "Tenant/purpose/origin-bound secret resolution with sanitized denial"
    requirement: AUTH-05
    verification:
      - kind: unit
        ref: "internal/auth/secret_refs_test.go#TestSecretBindings"
        status: pass
    human_judgment: false
  - id: D2
    description: "Operator binding parse/validation in Config without a new secret backend"
    requirement: INF-01
    verification:
      - kind: unit
        ref: "internal/config/config_test.go#TestLoadSecretBindings + TestLoadRejectsMalformedSecretBindings"
        status: pass
    human_judgment: false
  - id: D3
    description: "End-to-end Config→resolver check over two tenant bindings plus a no-auth local endpoint"
    requirement: INF-02
    verification:
      - kind: other
        ref: "throwaway go run self-check (deleted): valid + cross-tenant/cross-origin/cross-purpose denials, empty-ref local endpoint"
        status: pass
    human_judgment: false

duration: ~25min
completed: 2026-10-19
status: complete
---

# Plan 01-21: Tenant-Bound Credential References Summary

**`auth.SecretResolver` resolves an opaque `credential_ref` only for its explicitly bound tenant, purpose and canonical origin; anything else is a single sanitized `ErrSecretDenied`.**

## Accomplishments
- `internal/auth/secret_refs.go`: `SecretBinding`, `CanonicalizeSecretOrigin` (http/https only, default-port folding, rejects userinfo/path/query/fragment/scheme-relative), `SecretResolver.Resolve(tenantID, purpose, origin, refID)` checking all binding fields before any `os.LookupEnv`, and `Metadata` returning identity-only rows.
- `internal/config/config.go`: `SecretBindings []auth.SecretBinding` populated from `SECRET_BINDINGS` with structural validation at Load; errors never contain secret values or echo env names beyond config syntax. No second vault/backend added — process env stays the only source.
- `internal/auth/oidc.go`: pre-existing func-type `SecretResolver` renamed `ClientSecretResolver` (call sites use `EnvSecretResolver`, unchanged) to free the contract name.
- Tests: `TestSecretBindings` (12 subtests — valid, canonicalized forms, multi-origin connector, cross-tenant/ref guessing, wrong purpose, scheme/port drift, redirect/credential-bearing origins, missing value, sentinel isolation, no-auth local endpoint, nil resolver, metadata) and `TestSecretBindingValidation` plus `TestLoadSecretBindings`/`TestLoadRejectsMalformedSecretBindings`.

## Verification (observed)
- `go build ./...` — pass.
- `go test -count=1 ./internal/config` — pass.
- `go test -count=1 ./internal/auth -run 'TestSecretBindings|TestSecretBindingValidation'` — pass.
- Throwaway `go run` self-check (Config.Load + NewSecretResolver, two tenant bindings, no-auth local endpoint): valid resolution + cross-tenant/cross-origin/cross-purpose denials + empty-ref → empty credential; deleted after run.
- Pre-existing DB-backed auth tests (`TestSession*`, `TestOIDC*`, `TestAuthenticate*`, …) fail on `ENCLAVA_TEST_DATABASE_URL is not set` — no PostgreSQL on localhost:5432; unrelated to this change and unchanged by it.

## Deviations from Plan
None — plan executed as written. Consumer wiring remains assigned to 01-25 (executor) and later source/MCP composition plans; legacy direct credential construction is untouched per plan.

## Issues Encountered
None.
