---
phase: 01-application-and-inference
plan: "04"
type: summary
status: complete
requirements: [CORE-04, AUTH-04]
commits:
  - cdaa9b7 feat(01-04): domain contract types for source, jobs, billing, inference, artifacts
  - 494fb3b feat(01-04): canonical auth.Principal with all/list allowlist semantics
---

# 01-04 Summary — Canonical contract types + auth.Principal

## What landed

**Contract packages (pure types, acyclic — no service/store/llm/rag imports):**

- `internal/source/types.go` — `Connection` (kind const set incl. `google_drive`), `Object` (`ObservedSeq int64` non-pointer + `UpstreamRevision *string`; `AuthorizationMode` const set incl. `published_audience` for Notion per D-07), `ObjectVersion`, `SourceIdentity`, `SourceGroup`, `SourceGroupMembership`, `SourceGrant` (allow/deny, AccessRevision), `AccessSnapshot` (incomplete never authoritative), `SourceRestriction`, canonical `ObjectRef` {TenantID, ObjectID, ObservedSeq}, `Span`, `Locator`.
- `internal/source/document.go` — `Document` (immutable ordered Blocks + ObjectRef), `Block` (UTF-8 byte offsets into extracted text + Locator), `Chunk`, `Span` via `ObjectRef`, `ChunkProfile` (immutable+versioned; sizes in Unicode code points, truthfully labelled).
- `internal/jobs/types.go` — `Status` const set (queued…deleted incl. `unknown`), `Subject`, `EnqueueRequest`, `Job` (PrincipalSnapshot json.RawMessage, FenceToken, LeaseOwner/LeaseExpiresAt), `Attempt`, `Lease`, `Progress`, `ErrIdempotencyConflict`/`ErrStaleFence`/`ErrJobNotFound`.
- `internal/billing/types.go` — `UnitKind` (input|output|cached_input), `Price` (rational RateMicros/UnitQuantity), `PriceSnapshot` ({rates,currency,rounding_rule}), `Budget`, `Reservation` (held|settled|unknown|released), `Usage` (per-kind units, never clipped), errors incl. `ErrCurrencyMismatch`/`ErrArithmeticOverflow`. Zero float64; int64 micros only.
- `internal/inference/endpoint.go` — `Endpoint` (AuthorizedPrivate, AssuranceVerifiedAt, Capabilities, CredentialRef, etc.) + tenant-scoped `EndpointRepository` interface.
- `internal/artifacts/types.go` — opaque `Ref` {TenantID, ID, Kind, Version, SHA256, ByteSize} + `Meta`. Never a filesystem path.

**auth.Principal (AUTH-04):**

- `internal/auth/principal.go` — canonical Principal (tenant/actor/credential/session/delegation/billing fields), `ActorKind`/`CredentialKind`/`BillingSubject*` const sets, six `AllowedXxxAll` flags + item maps, `EndpointAllowed`, `IPAllowed`, `DecodeAllowlist` (mode:all → all; mode:list → items; absent/malformed → deny-everything). Context helpers retained; all resource helpers consult All flags — empty map never means unrestricted. Legacy fields (APIKeyID, Name, Budget*, etc.) retained until 01-35.
- `internal/auth/types.go` — `Session`/`Identity`/`Delegation` records + tenant-scoped `SessionRepository`/`IdentityRepository`/`DelegationRepository` seams (FindByTokenHash documented as the pre-auth exception). No `APIKeyRepository` redeclared (stays in service.go:45 for 01-08). No `IsAdmin`, no id-zero authority.
- `internal/auth/service.go` — `Authenticate` populates All flags via `allowlistSet`, preserving legacy storage semantics (empty column → all, "*" → all) until 01-08's mode-row cutover.
- Callers: `chatbot.Access` gained All flags; `chatbotAllowlistOpen` now delegates to `auth.ChatbotAllowed`. Tests updated to express all-mode grants explicitly; `TestPrincipalAllowlists` added covering all/list/empty-deny + decode contract for all six allowlists.

## Verification

- `go build ./...` clean; `go test ./...` — all packages pass except `internal/store/postgres` which fails on missing `ENCLAVA_TEST_DATABASE_URL` — **preexisting on base ab6d3eb**, unrelated.
- Gates: no `float64` in `internal/billing`; no contract package imports `internal/rag|llm|store`; `gofmt -l` clean; `go vet` clean.

## Deferred (per plan)

- `service.go` rewrite → 01-08; legacy Principal fields → 01-35; httpx endpoint/IP fail-closed rewrite → 01-16; `allowlistSet` legacy list→all mapping retires with the storage cutover.
