---
phase: 01-application-and-inference
plan: "02"
subsystem: infra
tags: [llm, transport, http, attestation, tinfoil, ssrf, streaming]

requires:
  - phase: none
    provides: leaf transport package; no prior phase output consumed
provides:
  - Bounded, origin-isolated OpenAI-compatible transport with guarded dialing,
    redirect refusal, overflow-safe body reads and no mutation replay
  - Confidential transport bounded by verified attestation, shutdown-scoped
    verification work and disabled SDK retries
  - Per-instance immutable model cache observations and cancellable prober
affects: [inference executor plans, endpoint administration, probes/readiness]

actuals:
  tokens: 30000
  tasks: 2
  commits: 2
  plan_head_before: 09fa4b214568fd34cc2e32bdd74bacdee2bb2013
  plan_head_after: 6233f6a

tech-stack:
  added: []
  patterns:
    - "Shared pooled transports keyed by origin + private-approval with DNS-pinned guarded dials"
    - "noRetryBody (NopCloser) disables net/http GetBody so mutations/streams are never replayed"
    - "boundedStreamBody applies cancellable idle/wall limits to SSE streams"
    - "singleflight verification bound to shutdown context with timeout, not caller ctx"

key-files:
  created: []
  modified:
    - internal/llm/provider.go
    - internal/llm/openai_adapter.go
    - internal/llm/openai_adapter_test.go
    - internal/llm/tinfoil_provider.go
    - internal/llm/tinfoil_provider_test.go
    - internal/llm/model_cache.go
    - internal/llm/model_cache_test.go
    - internal/llm/prober.go

key-decisions:
  - "Private/cleartext endpoint targets default to refused; explicit approval via WithAllowPrivateEndpoint (main.go wiring belongs to the integration owner)"
  - "Link-local/metadata, unspecified and multicast dial targets are refused even when an endpoint is approved"
  - "Cross-origin redirects refused outright so credentials cannot leak; same-origin limited to 5"

patterns-established:
  - "Endpoint transport policy: one shared transport per (origin, approval, header-timeout); dial resolves once and pins the approved IP"
  - "No client-side replay of possibly accepted mutations or partial streams"

requirements-completed: [INF-01, INF-05, INF-06]

coverage:
  - id: D1
    description: "OpenAI-compatible adapter denies unapproved private/cleartext and link-local endpoints, refuses credential-changing redirects, fails explicitly on oversized/truncated bodies, never replays accepted mutations, and closes the upstream body on cancel"
    requirement: INF-01
    verification:
      - kind: unit
        ref: "internal/llm/openai_adapter_test.go#TestAdapterRejectsUnapprovedPrivateEndpoint, TestAdapterRejectsMetadataTarget, TestAdapterRedirectDoesNotLeakCredentials, TestAdapterOversizedBodyIsExplicitFailure, TestAdapterTruncatedBodyIsFailure, TestAdapterMutationIsNotReplayed, TestAdapterStreamCancelClosesUpstreamBody, TestAdapterStreamIdleLimit"
        status: pass
    human_judgment: false
  - id: D2
    description: "Tinfoil verification failure denies dispatch without ordinary-HTTP fallback; cancelled waiter does not poison status; endpoint instances keep independent attestation state"
    requirement: INF-05
    verification:
      - kind: unit
        ref: "internal/llm/tinfoil_provider_test.go#TestTinfoilVerificationFailureDeniesDispatch, TestTinfoilCancelledWaiterDoesNotPoisonStatus, TestTinfoilProvidersHaveIndependentState"
        status: pass
    human_judgment: false
  - id: D3
    description: "Model cache returns deep copies and per-instance invalidation cannot leak model data across endpoint instances"
    requirement: INF-06
    verification:
      - kind: unit
        ref: "internal/llm/model_cache_test.go#TestModelCacheProviderCachesAndInvalidatesModels, TestModelCacheProviderReturnsIndependentCopies, TestModelCacheProviderConcurrentRefreshAndInstances"
        status: pass
    human_judgment: false
  - id: D4
    description: "Real verified attestation against a live Tinfoil enclave and an authorized local endpoint"
    requirement: INF-05
    verification: []
    human_judgment: true
    rationale: "No endpoint credentials/enclave configured in this environment; plan requires recording missing live prerequisites rather than passing them. Fake verification proves control flow only."

duration: ~40 min
completed: 2026-10-19
status: complete
---

# Phase 01 Plan 02: Safe local and confidential endpoint transports Summary

**Bounded, origin-isolated HTTP transport for OpenAI-compatible endpoints and attestation-gated Tinfoil transport with per-instance model cache isolation — all without touching shared composition files.**

## Performance

- **Duration:** ~40 min
- **Started:** 2026-10-19
- **Completed:** 2026-10-19
- **Tasks:** 2
- **Files modified:** 8

## Accomplishments
- Shared pooled `net/http` transports per approved endpoint origin with DNS-pinned guarded dials: resolved addresses are checked once, private/loopback require explicit approval, and link-local/metadata/unspecified/multicast are always refused — credentials can never reach an unapproved resolved address.
- Redirect safety: same-origin-only redirect policy (max 5) prevents credential leakage across origins; a test proves no `Authorization` reaches a redirect target.
- Bounded reads: JSON and error bodies fail explicitly on overflow or truncation instead of returning clipped success JSON; streams get cancellable idle and wall limits and context cancel closes the real upstream body.
- No client-level replay: request bodies are wrapped so `GetBody` is never set (a possibly accepted POST is observed exactly once), and the tinfoil SDK is built with `MaxRetries(0)`.
- Confidential verification runs on a shutdown-bound context with a deadline inside singleflight — a cancelled caller gets `ctx.Err()` without killing shared work or poisoning status; failed verification denies dispatch with no ordinary-HTTP fallback.
- Model cache returns deep copies of models and stays per instance, so caller mutation or one endpoint's invalidation cannot leak into another endpoint.
- Prober failure backoff is cancellable and probe work stays bounded.

## Task Commits

1. **Task 1: Bound HTTP transport and isolate endpoint credentials** — `d0bde4b` (feat)
2. **Task 2: Keep confidential verification and cache/probe state endpoint-specific** — `6233f6a` (feat)

**Plan metadata:** `TBD` (docs: complete plan)

## Files Created/Modified
- `internal/llm/provider.go` — transport policy: shared transports, guarded dial, base-URL validation, redirect policy, bounded bodies, stream watchdog
- `internal/llm/openai_adapter.go` — constructor validation, shared transports, bounded reads, no-replay bodies, stream limits, approval gate
- `internal/llm/openai_adapter_test.go` — SSRF/redirect/overflow/truncation/replay/cancel/idle tests
- `internal/llm/tinfoil_provider.go` — shutdown-scoped bounded verification, `WithMaxRetries(0)`, redirect refusal, bounded reads, stream limits
- `internal/llm/tinfoil_provider_test.go` — verification-denial, cancelled-waiter and instance-isolation tests
- `internal/llm/model_cache.go` — deep-copy reads and per-instance invalidation
- `internal/llm/model_cache_test.go` — mutation-isolation and concurrent refresh tests
- `internal/llm/prober.go` — cancellable failure jitter

## Decisions Made
- Private/cleartext/local endpoints default to **denied** until `WithAllowPrivateEndpoint()` marks the endpoint explicitly approved; `main.go` (owned by the integration owner) does not yet pass approval, so locally configured private endpoints will be refused until that wiring lands.
- Streaming is bounded by response-header timeout plus idle/wall limits on the body rather than a single client timeout, preserving long-lived SSE.
- Verification goroutine is bounded by `verifyTimeout` (default 60s) on the shutdown context; the SDK's `Verify()` has no context API, so a hard hang is released to callers but the SDK call itself may linger — documented in code.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 2 - Missing Critical] Added `WithAllowPrivateEndpoint` approval gate the plan requires but main.go cannot yet set**
- **Found during:** Task 1
- **Issue:** The plan requires explicit administrative approval for private endpoints, but `cmd/enclava/main.go` is reserved for the integration owner and cannot be edited by this track.
- **Fix:** Implemented default-deny with an opt-in approval method; documented in 01-USER-SETUP.md for the inference executor wiring.
- **Files modified:** internal/llm/openai_adapter.go, .planning/phases/01-application-and-inference/01-USER-SETUP.md
- **Verification:** `go test -count=1 ./internal/llm` passes; new tests assert denial before approval and success after.
- **Committed in:** d0bde4b

**Total deviations:** 1 auto-fixed (missing critical wiring)
**Impact on plan:** Required by the contracts; no scope creep. The public constructor signature is unchanged.

## Issues Encountered
- Live verification against a real Tinfoil enclave and an authorized local endpoint could not be exercised: no endpoint credentials/enclave configured in this environment. Recorded as a prerequisite in 01-USER-SETUP.md, not as a passing check.

## Authentication Gates
None.

## User Setup Required
**External services require manual configuration.** See [01-USER-SETUP.md](./01-USER-SETUP.md) for:
- Endpoint base URLs and API keys for the approved local endpoint
- Tinfoil API key, enclave and repo for attestation binding

## Next Phase Readiness
- `internal/llm` transports are safe for multi-endpoint reuse; the inference executor can compose them with endpoint records and pass per-endpoint approval + shutdown context via `WithAllowPrivateEndpoint`, `WithShutdownContext`, `WithStreamLimits`.
- Ready for 01-03+.

## Self-Check: PASSED
- `go test -count=1 ./internal/llm` — pass (incl. `-race`)
- `go test -count=1 ./...` — pass (whole module)
- `go build ./...` — pass
- Key files exist on disk; 2 commits on `worktree-agent-p02-1792402081` over base `09fa4b2`.

---
*Phase: 01-application-and-inference*
*Completed: 2026-10-19*
