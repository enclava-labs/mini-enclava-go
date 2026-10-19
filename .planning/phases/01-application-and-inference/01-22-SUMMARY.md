---
phase: 01-application-and-inference
plan: "22"
subsystem: composition-root
tags: [cutover, bootstrap, postgres, auth, jobs, artifacts]
status: complete
requirements: [CORE-01, CORE-05, AUTH-01]

requires:
  - phase: 01-application-and-inference (plan 01-08)
    provides: postgres repos + testdb schema fixture used by apptest
  - phase: 01-application-and-inference (plan 01-15/01-17)
    provides: artifacts.Store+Reconcile, billing service/repos
  - phase: 01-application-and-inference (plan 01-20)
    provides: SessionAuth dashboard login handlers wired into NewApp
  - phase: 01-application-and-inference (plan 01-21)
    provides: jobs.Service + jobs.Runner fence/drain protocol

provides:
  - cmd/enclava/main.go is postgres-only: postgres.Open is the sole database
    open; sqlite.Open and every sqlite repo construction are gone;
    httpx.NewApp keeps the positional signature with usageRepo=nil (dead
    slot, removed by 01-36)
  - artifacts.NewStore(<data>/artifacts, <data>/scratch) + Reconcile run at
    startup; reconcile failure aborts startup
  - jobs.NewRunner runs infrastructure-only (empty handler map) under its
    own context; shutdown drains the runner BEFORE the HTTP server
  - `enclava bootstrap` CLI: one transaction creates the tenant (slug flag,
    default "default"; fails on existing slug), an admin role carrying
    platform.admin + every feature/manage scope, and the first admin user
    with a generated crypto/rand password (32 chars, printed once, never
    logged or stored); --issue-key mints a one-time bootstrap API key
  - config: BOOTSTRAP_API_KEY/BOOTSTRAP_ENABLED/BOOTSTRAP_API_KEY_MIN_LENGTH
    env seeding and validateBootstrapAPIKey are deleted (the CLI replaces
    them); SESSION_TENANT_SLUG (default "bootstrap") selects the tenant the
    dashboard login resolves
  - Route unmount: every paid path not executor-backed 404s — /v1 and
    /api/v1 models listings, chat/completions, embeddings, responses,
    openai_passthrough catch-alls, attestation, extract module mount,
    chatbot external completions, rag search/upload/reprocess, connector
    sync dispatch. Metadata CRUD, admin, and dashboard surfaces stay.
  - internal/testdb/apptest.go StartApp: builds ./cmd/enclava into
    t.TempDir, whitelisted env (DATABASE_URL/ENCLAVA_DATA_DIR/APP_PORT only,
    never HOME/.env), one process per call on a free selected port,
    optional bootstrap, /livez health wait, cookie-jar client, Stop().

actuals:
  tokens: ~
  tasks: 4
  commits: 1

tech-stack:
  added: []
  patterns:
    - "Session-authenticated template listing is chatbot-allowlist-filtered;
      persistence assertions in the real-run smoke use the bootstrap API key
      (mode:all grants)"

verification:
  - "CGO_ENABLED=0 go build -trimpath -o /tmp/enclava-build ./cmd/enclava: clean; go build ./...: clean"
  - "ENCLAVA_TEST_DATABASE_URL go test ./... -count=1: all green except internal/rag TestQdrantClientIntegration, which requires ENCLAVA_TEST_QDRANT_URL (pre-existing missing fixture, out of scope)"
  - "Real smoke on fresh schema: bootstrap --tenant smoke (password printed once; rerun on same slug fails) → app start on APP_PORT → /livez 200 → POST /dashboard/login 303 + session cookie → GET /dashboard 200 → POST /api/v1/prompt-templates 201 via session+CSRF → /v1/chat/completions, /api/v1/models, /api/v1/extract/process all 404 → admin key create+delete via session → process restart → template list still returns smoke_t1 → revoked key call 401 → POST /dashboard/logout 303 then GET /dashboard 303 to login"
---
# 01-22 Summary — final core composition cutover

The composition root is fully on the new stack. `sqlite.Open` and every
sqlite repo construction left `main.go`; `postgres.Open` is the only
database open. The artifact store reconciles before serving and the
infrastructure-only jobs runner drains first at shutdown. The legacy
usageRepo survives only as a nil positional param into `httpx.NewApp`.

`enclava bootstrap` is the single first-login path: it creates the first
tenant and admin in one transaction with a generated password printed once,
and refuses an existing slug. BOOTSTRAP_API_KEY env seeding is gone; there
is no fixed credential anywhere.

All paid routes without executor backing now 404 (unmounted, not stubbed).
Handler files still compile for 01-34's removal; openai_contract_test.go
was deleted because it asserted only the removed passthrough behavior.
The real-process smoke (throwaway /tmp/smoke_core.sh) proved
bootstrap → login → owned-metadata write → restart persistence → key
revocation denial → session logout denial.

## Deviations from plan text
- Artifact roots are <data>/artifacts + <data>/scratch (siblings); the
  plan's literal `NewStore(cfg.DataDir, cfg.DataDir+"/scratch")` fails the
  store's disjoint-roots validation.
- `openai_contract_test.go` deleted in addition to the declared files: it
  exercised only unmounted routes and the sqlite usage store.
- Rag ingest/reprocess and connector sync are 404-gated at their sub-route
  dispatch while collection/source/sync-job metadata CRUD stays mounted.
- Bootstrap admin role scopes encoded as a plain JSON array (roles table
  shape), bootstrap key scopes as the mode/items grant document — the two
  columns store different shapes.
