---
phase: 01-application-and-inference
plan: "25"
title: "Route every processing model call through the shared executor"
completed_at: "2026-10-19"
requirements: ["INF-02", "AUTH-04", "AUTH-06", "JOB-03", "JOB-04"]
---

# 01-25 Summary — extraction through the shared executor

## What changed

- `internal/extract/service.go`: dropped the direct `llm.Provider` and
  `*billing.Service` dependencies. New narrow seams: `Executor`
  (`Execute(ctx, Principal, inference.Request)`, satisfied by
  `*inference.Service`) and `EndpointsLister` (`ListCapabilities`, satisfied
  by `*postgres.EndpointsRepo`). The fenced attempt re-checks template
  authorization against the frozen principal, then hands the bounded chat
  payload, the immutable input artifact reference (`Ref{TenantID, ID,
  Version}` recorded in the job policy as `input_version`), the per-kind
  unit estimate and the job/attempt ids to the executor — reserve,
  dispatch, settle-or-retain all happen inside `Execute` exactly once. A
  `SettlementError` on the result fails the attempt; the artifact version
  pinned in the job policy is the immutable input the executor verifies.
  `resolveModel`/`ModelsForExtract` now read tenant endpoint `model:`
  capability bindings instead of probing the provider.
- `internal/modules/extract/handlers.go`: removed the module-level
  `EnforceBudget` call and `ProviderReady` gates (they ignored the actual
  route/capability and could double-charge); `inference.ErrDenied` maps to
  403. `HandleHealth` reports configured executor-route model availability
  instead of a raw `ListModels` probe.
- `internal/httpx/dashboard_extract.go`: `handleDashboardExtractModels`
  passes the session principal's tenant to `ModelsForExtract`.
- `internal/httpx/dashboard.go`: mounted the previously unregistered
  `partials/extract/process` and `partials/extract/models` routes the
  dashboard JS already calls, behind session+CSRF.
- `cmd/enclava/main.go`: constructs `auth.SecretResolver` from
  `SECRET_BINDINGS` plus an operator-env-key-equivalent binding returned by
  `ensureEnvEndpoint` (only when the named env var is actually set and the
  origin canonicalizes); builds the shared `inference.Service` executor;
  seeds the env endpoint with chat/embeddings/vision capabilities and
  `model:` aliases discovered from the provider; wires `extract.NewService`
  to it; registers `extract.OperationProcess` on the jobs runner; mounts
  the extract module on the app mux.

## Tests

- `service_integration_test.go`: fixture now composes the real
  `inference.Service` over a deterministic fake upstream — policy, routing,
  reservations and settlement run for real. Prices keyed by the vision
  capability (file/image parts upgrade the routed capability). New
  `TestAttemptRestrictedInputDeniesBeforeDispatch` proves a persisted
  artifact restriction denying `vision` stops the attempt before any
  upstream dispatch; `TestAttemptPricedModelReservesAndSettles` proves one
  reservation/one settled usage row; unpriced model still refuses before
  dispatch with no usage row.
- `handlers_test.go`: same executor-over-fake-upstream wiring; single
  usage_record with `capability=vision` asserted.

## Verification performed

- `ENCLAVA_TEST_DATABASE_URL=postgres://enclava:enclava@localhost:5432/enclava_test`
  `go test -count=1 ./internal/extract ./internal/modules/extract
  ./internal/httpx ./cmd/enclava` → ok / ok / ok / no test files.
- `go build ./...` clean; `go vet` clean on touched packages.

## Acceptance criteria

- Prohibited processing destination denied before dispatch: covered by
  `TestAttemptRestrictedInputDeniesBeforeDispatch` (persisted restriction →
  `inference.ErrDenied`, upstream calls == 0) and disallowed-model attempt
  (ErrDenied/ErrNoRoute, upstream calls == 0).
- One attempt → one reservation/settlement: billing calls removed from
  extract service/module; executor owns reserve/settle; single usage row
  asserted.
- Invalid-provider/schema errors still surface: provider errors propagate
  as `provider chat completion` failures; schema validation unchanged.

## Deviations

- Mounted `/dashboard/partials/extract/process` and `/models` in
  `dashboard.go` (outside listed file set): the dashboard JS already called
  these endpoints but they were never registered — required for the
  dashboard processing path to exist at all.
- Env provider seeding discovers model aliases via `ListModels` at startup
  and auto-adds an env-key binding for `<provider>_env` only when the env
  var is set; an explicit operator `SECRET_BINDINGS` entry for the same
  ref wins.

## Self-Check: PASSED
