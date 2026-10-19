---
phase: 01-application-and-inference
plan: "23"
subsystem: inference
tags: [executor, billing, authorization, postgres, sse, policy]
status: complete
requirements: [AUTH-04, AUTH-06, JOB-03, JOB-04, INF-01, INF-02, INF-05, INF-06]

requires:
  - phase: 01-application-and-inference (plan 01-02)
    provides: llm.Provider/ResponsesProvider transports — OpenAI-compatible
      adapter and attested Tinfoil provider with guarded dials
  - phase: 01-application-and-inference (plan 01-09)
    provides: source.Authorizer CanRead/EffectiveRestrictions with
      narrow-only request restrictions
  - phase: 01-application-and-inference (plan 01-11)
    provides: billing.Service reserve/settle/unknown/release and
      postgres.EndpointsRepo
  - phase: 01-application-and-inference (plan 01-21)
    provides: auth.SecretResolver tenant/purpose/origin-bound credentials

provides:
  - inference.Service.Execute — the single authorized priced boundary every
    model call routes through: analyze payload → resolve inputs → route →
    evaluate → reserve → mark dispatched → dispatch → settle-or-retain
  - inference.Service.AuthorizeDestination — reusable server-side
    destination evaluation for later approved outbound clients (MCP)
  - inference.Request/Result/Decision contracts: bounded payload, immutable
    source/artifact/conversation input references, narrowing-only request
    restriction, per-kind unit estimate bound
  - Model alias bindings inside endpoint capabilities ("model:<alias>[=id]")
    — an endpoint with no bindings exposes no models
  - accountingStream SSE wrapper: terminal usage settles once; truncation,
    missing usage or early Close retains unknown liability; settlement runs
    on a detached bounded context
  - Policy intersections: object/artifact/conversation/tenant restrictions ∪
    request narrowing ∧ actor allowlists ∧ endpoint capability ∧ required
    assurance; purported assurance_mode never substitutes for observed
    assurance_verified_at

affects: [01-25 extraction, 01-27 embeddings, 01-28 assistants,
  01-29/01-30 public API plans, 01-31 admission bounds]

actuals:
  tokens: ~11500
  tasks: 2
  commits: 1

tech-stack:
  added: []
  patterns:
    - "Pre-MarkDispatched = provable never-dispatched (release); post-dispatch = settle or MarkUnknown; one DB tx never spans network I/O"
    - "Endpoint-scoped price key first, model-scoped fallback; every billed unit kind needs a rate"
    - "Per-endpoint transport cache keyed by full endpoint signature; attesting transports stamp assurance_verified_at on proof"
    - "Payload 'type' interpreted only in tool/content discriminator contexts; schema 'type' stays data"

key-files:
  created:
    - internal/inference/types.go
    - internal/inference/policy.go
    - internal/inference/service.go
    - internal/inference/usage.go
    - internal/inference/policy_test.go
    - internal/inference/service_test.go
    - internal/inference/usage_test.go
  modified: []

key-decisions:
  - "Request.AttemptID is stored verbatim — it must reference a durable job_attempts row (FK); standalone executions use the reservation id as attempt identity"
  - "Model aliases bind through endpoint capabilities rows ('model:<alias>[=<upstream-id>]') — self-contained in the existing column until the route-UI plan adds config surfaces"
  - "Chat payload carrying image/file parts routes as capability 'vision' so pricing and endpoint capability follow the real shape"
  - "SSE streams inject stream_options.include_usage; a stream closed before a terminal event retains liability rather than settling zero"

requirements-completed: [AUTH-04, AUTH-06, JOB-03, JOB-04, INF-01, INF-02, INF-05, INF-06]

coverage:
  - id: D1
    description: "Authorized dispatch + exact once-only settlement under the immutable snapshot (input/output disjoint, fractional rates, overrun preserved)"
    requirement: JOB-03
    verification:
      - kind: integration
        ref: "internal/inference/service_test.go#TestExecuteSettlesExactUsage + #TestFractionalUnitPricing + #TestActualOverrunSettlesTruthfully + #TestDuplicateSettlementReturnsSameRecord"
        status: pass
    human_judgment: false
  - id: D2
    description: "Missing price blocks dispatch with no reservation; never-dispatched transport failure releases; ambiguous dispatched failure and early stream close retain unknown liability"
    requirement: JOB-04
    verification:
      - kind: integration
        ref: "internal/inference/service_test.go#TestMissingPriceBlocksDispatch + #TestNeverDispatchedFailureReleases + #TestDispatchedFailureRetainsLiability + #TestStreamEarlyCloseRetainsLiability"
        status: pass
    human_judgment: false
  - id: D3
    description: "Input/tenant/restriction intersection, forged fields, cross-tenant refs, stale versions, purported confidential flags, nested destination/tool rejection"
    requirement: AUTH-06
    verification:
      - kind: integration
        ref: "internal/inference/policy_test.go (11 cases)"
        status: pass
    human_judgment: false
  - id: D4
    description: "Real adapter dispatch through the bounded guarded HTTP transport to a customer-local endpoint"
    requirement: INF-01
    verification:
      - kind: integration
        ref: "internal/inference/service_test.go#TestRealAdapterEndToEnd + #TestConfidentialEndpointVerifiesBeforeDispatch"
        status: pass
    human_judgment: false

duration: ~1h
completed: 2026-10-19
---

# Phase 01-23: Centralized authorized inference routing, reservations and execution

**The one `inference.Service.Execute` boundary now owns policy, routing, reservation and outcome accounting for every model call.**

## Accomplishments

- `Execute` resolves source/artifact/conversation input references from tenant-owned storage under current `source.Authorizer` decisions, unions their restrictions with tenant policy and request narrowing, intersects actor model/endpoint allowlists and endpoint capability+assurance, picks a deterministic route, reserves an immutable price snapshot, marks dispatch durably before any network write, and settles exact usage or retains unknown liability — exactly once.
- `AuthorizeDestination` exposes the same evaluation for server-loaded destinations (the future MCP/outbound path), never caller-supplied assurance.
- Payload contract is bounded: identity/credential/origin/retention keys deny, hosted-tool and remote-resource `type` discriminators reject explicitly, URL fields accept only inline data URIs, and JSON-Schema `type` inside tool parameters stays data.
- Usage accounting is disjoint and truthful: cached input settles separately, unrecognized advertised units become billed `OtherUnits`, `usage` parsing failure means unknown — never zero — and a definitive upstream error settles its reported usage honestly.
- Live verification: per-endpoint transport cache keyed by endpoint signature; attesting endpoints verify before dispatch and stamp `assurance_verified_at` with observed proof.

## Files Created

- `internal/inference/types.go` — capability consts, Request/Result/Decision, operation bounds, model-alias binding
- `internal/inference/policy.go` — input resolution, restriction merge, payload walker, `AuthorizeDestination`, route selection
- `internal/inference/service.go` — `Execute` lifecycle, reserve/dispatch, transport factory + cache, credential resolution, attestation stamping
- `internal/inference/usage.go` — usage parsing and `accountingStream` once-only settlement wrapper
- `internal/inference/{policy,service,usage}_test.go` — 27 cases against real PostgreSQL plus SSE unit tests

## Verification

- `ENCLAVA_TEST_DATABASE_URL=postgres://enclava:enclava@127.0.0.1:5432/postgres?sslmode=disable go test -count=1 ./internal/inference ./internal/billing` — PASS, zero skips (27 + existing billing cases on isolated schemas)
- `go vet ./internal/inference`, `gofmt -l internal/inference` — clean
- `go build ./...` — clean; no existing callers needed migration (public routes were unmounted in 01-22 awaiting this boundary)

## Issues Encountered

- None blocking. One deliberate choice: reservation `attempt_id` accepts only existing `job_attempts` ids (FK); standalone executions identify by reservation id.

## Self-Check: PASSED
