---
phase: 01-application-and-inference
plan: "03"
subsystem: infra
tags: [postgres, config, env, entrypoint]

requires: []
provides:
  - "config.Load rejects non-PostgreSQL DATABASE_URL values (empty, file paths, .db suffix, file: URIs) and accepts postgres://, postgresql:// and key=value DSNs"
  - "Config.DataDir (ENCLAVA_DATA_DIR, default ./data) and Config.SessionTTL (SESSION_TTL, default 12h)"
  - "DSN-safe retained launch files: entrypoint no longer dirname/mkdir's DATABASE_URL"
affects: [01-08, 01-12, 01-22, any plan reading cfg.DataDir or cfg.SessionTTL]

actuals:
  tokens: ~1500
  tasks: 1
  commits: 1

tech-stack:
  added: []
  patterns: [fail-closed DSN validation at config load, mirrored by postgres.Open at open time]

key-files:
  created: []
  modified:
    - internal/config/config.go
    - internal/config/config_test.go
    - .env.example
    - docker/enclava-go-cap-entrypoint
    - scripts/entrypoint_smoke.sh

key-decisions:
  - "DATABASE_URL default changed from ./enclava.db to empty with fail-closed validation — no implicit SQLite path can reach runtime"
  - "key=value DSNs accepted alongside postgres:// URLs to match the postgres.Open contract (01-01)"
  - "No OIDC env vars added — providers are DB-configured rows (01-12)"

patterns-established:
  - "DSN validation duplicated at config boundary AND store boundary (defense in depth, fail closed)"
  - "clearConfigEnv seeds a valid postgres:// DATABASE_URL so tests exercise real parsing, not the zero value"

requirements-completed: [CORE-02]

coverage:
  - id: D1
    description: "config.Load accepts only PostgreSQL DSNs and rejects SQLite filenames/URIs"
    requirement: CORE-02
    verification:
      - kind: unit
        ref: "internal/config/config_test.go#TestLoadRejectsNonPostgresDatabaseURL + TestLoadAcceptsPostgresDatabaseURL"
        status: pass
    human_judgment: false
  - id: D2
    description: "ENCLAVA_DATA_DIR and SESSION_TTL configurable so later plans never edit config.go"
    requirement: CORE-02
    verification:
      - kind: unit
        ref: "internal/config/config_test.go#TestLoadDataDirAndSessionTTL + TestLoadDataDirAndSessionTTLDefaults"
        status: pass
    human_judgment: false
  - id: D3
    description: "Retained entrypoint/smoke never apply dirname/mkdir to a PostgreSQL DSN"
    verification:
      - kind: integration
        ref: "sh scripts/entrypoint_smoke.sh"
        status: pass
    human_judgment: false

duration: ~10min
completed: 2026-10-19
status: complete
---

# Phase 01 Plan 03: DSN-validated config + env/entrypoint corrections

**config.Load now fail-closes on any non-PostgreSQL DATABASE_URL; durable-artifact root and session TTL are env-configurable; no launch file treats the DSN as a filename.**

## Accomplishments

- `internal/config/config.go`: added `DataDir` and `SessionTTL` fields; `DATABASE_URL` default removed (empty now fails closed); new `validatePostgresDSN` accepts `postgres://`, `postgresql://` and `key=value` DSNs and rejects empty values, filesystem paths, `.db` suffixes and `file:` prefixes.
- `internal/config/config_test.go`: `clearConfigEnv` seeds a valid `postgres://` DSN and clears the two new env vars; added `TestLoadRejectsNonPostgresDatabaseURL` (6 reject cases incl. filesystem path), `TestLoadAcceptsPostgresDatabaseURL` (3 accept cases), and DataDir/SessionTTL default + parse tests.
- `.env.example`: `DATABASE_URL` is now a `postgres://` example; documented `ENCLAVA_TEST_DATABASE_URL`, `ENCLAVA_DATA_DIR`, `SESSION_TTL`; fixed stale "keys stored in sqlite" comment.
- `docker/enclava-go-cap-entrypoint`: removed the `dirname`/`mkdir -p` block on `DATABASE_URL`; default changed to a `postgres://` DSN (leftover `/state/app-data/enclava.db` default would otherwise still smuggle a file path into runtime).
- `scripts/entrypoint_smoke.sh`: fake `DATABASE_URL` is a DSN-shaped string; expected env echo updated; smoke run passes end-to-end.

## Verification

- `go test ./internal/config/ -count=1` — PASS
- `go build ./internal/config` — PASS
- `go vet ./internal/config/` — clean
- `bash -n docker/enclava-go-cap-entrypoint` — PASS
- `bash -n scripts/entrypoint_smoke.sh` — PASS
- `sh scripts/entrypoint_smoke.sh` — PASS (restart + env echo verified)
- `docker-compose.yml` — untouched (still carries its own legacy `/data/enclava.db`; deployment work excluded by plan)

## Deviations

- Plan said remove "ONLY the dirname/mkdir" from the entrypoint, but its default `DATABASE_URL=/state/app-data/enclava.db` was itself a file path that config now rejects — changed the default to `postgres://postgres:postgres@localhost:5432/enclava` to keep the retained launch path bootable (Rule 2: missing-critical fix inside task scope).

## Self-Check: PASSED
