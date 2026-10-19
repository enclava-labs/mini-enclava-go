---
phase: "01"
slug: "application-and-inference"
status: verified
# threats_open = count of OPEN threats at or above workflow.security_block_on severity (the blocking gate)
threats_open: 0
asvs_level: 1
created: "2026-10-19"
---

# Phase 01 — Security

> Per-phase security contract: threat register, accepted risks, and audit trail.

---

## Trust Boundaries

| Boundary | Description | Data Crossing |
|----------|-------------|---------------|
| Browser/session | Dashboard + session cookie + CSRF token crossing into server-side session principals | Credentials, session tokens, CSRF tokens |
| API key / Bearer | Machine callers authenticating credentials against tenant-scoped identities | API keys, allowlists, rate limits |
| OIDC IdP | External identity provider flow (state/nonce/PKCE, code exchange, ID-token verification) | Authorization codes, ID tokens, link challenges |
| Inference upstreams | Routed model calls to operator-approved local/confidential endpoints | Prompts, artifacts, credentials, usage |
| PostgreSQL / artifacts | Durable tenant-scoped state and the per-tenant artifact filesystem | Tenant data, job payloads, billing rows |
| Public OpenAI API | Third-party SDK callers on /v1 (models/chat/embeddings/responses) | Public response IDs, bound references, SSE streams |

---

## Threat Register

| Threat ID | Category | Component | Severity | Disposition | Mitigation | Status |
|-----------|----------|-----------|----------|-------------|------------|--------|
| T-01-01 | Spoofing | OIDC identity and account linking | high | mitigate | internal/auth/oidc.go:107-131 state+nonce+PKCE; :153-200 single-use state consume, token+nonce verification before issuer+subject resolution | closed |
| T-01-02 | Spoofing | Sessions and CSRF | high | mitigate | internal/auth/sessions.go:24-53 opaque random session/CSRF tokens with expiry/idle/revocation; internal/httpx/sessionauth.go:280-295 session required + CSRF on mutations | closed |
| T-01-03 | Elevation of Privilege | Identity administration | high | mitigate | internal/httpx/identities.go:5-8,367-381 admin-scope + caller-scope-subset guards, tenant-scoped ids | closed |
| T-01-04 | Spoofing / EoP | API-key authentication and allowlists | high | mitigate | internal/httpx/app.go:235-264,288-293 credential verification, deny-on-error, required scope, allowlists; endpoint_policy.go:17-41 fail-closed endpoint/IP lists | closed |
| T-01-05 | Tampering | PostgreSQL tenant isolation | high | mitigate | migrations/002_identity.sql composite tenant keys/FKs; identity_repo.go tenant-pinned statements | closed |
| T-01-06 | EoP / Info Disclosure | Inference policy and source authorization | high | mitigate | internal/inference/policy.go:136-139 indistinguishable denial; internal/source/authorize.go:10-13,87-108,126-134 fail-closed membership/grant/restriction checks | closed |
| T-01-07 | EoP / Info Disclosure | Inference credentials | high | mitigate | internal/auth/secret_refs.go:90-102,162-186 tenant+purpose+ref+canonical-origin match before env resolution; no ambient-key inheritance | closed |
| T-01-08 | EoP | Inference SSRF and credential redirects | high | mitigate | internal/llm/provider.go:116-140,150-158 DNS-pinned guarded dials, link-local/metadata/unspecified/multicast refused, private requires approval; openai_adapter.go:64-72 same-origin redirects | closed |
| T-01-09 | Denial of Service | Inference admission | medium | mitigate | internal/httpx/middleware.go:264-266,295-298,447-474 bounded in-flight/queue/body admission with explicit overload rejection; internal/inference/admission.go pools + per-tenant sublimits | closed |
| T-01-10 | Tampering / Info Disclosure | Artifact storage | high | mitigate | internal/artifacts/store.go:159-217 server-side path composition with validation and byte limit; :257-315 fence-checked atomic finalize | closed |
| T-01-11 | Tampering / Repudiation | Durable jobs | high | mitigate | internal/jobs/repository.go:41-68 owner scoping, fencing, unknown-outcome retention; jobs_repo.go:207-212,381-405,436-449 fenced conditional writes | closed |
| T-01-12 | Tampering / Repudiation | Billing reservations and settlement | high | mitigate | internal/billing/service.go:152-202 snapshot settlement with evidence; :385-410,442-489 overflow checks; repository.go:57-59 once-only uniqueness | closed |
| T-01-13 | Info Disclosure | Public Responses IDs | high | mitigate | internal/httpx/responses.go:327-344,346-403 caller-owned binding resolution and reference rewriting; :490-503,600-649 mappings persisted before public exposure | closed |
| T-01-14 | Info Disclosure | Upstream error responses | medium | mitigate | internal/httpx/app.go WriteProviderError: only standard OpenAI error fields (message/type/code/param) pass through and ID-shaped tokens in the message are redacted — upstream resource IDs no longer reach clients (fix committed after the audit flagged the raw-body passthrough) | closed |

*Status: open · closed · open — below high threshold (non-blocking)*
*Severity: critical > high > medium > low — only open threats at or above workflow.security_block_on count toward threats_open*
*Disposition: mitigate (implementation required) · accept (documented risk) · transfer (third-party)*

---

## Accepted Risks Log

| Risk ID | Threat Ref | Rationale | Accepted By | Date |
|---------|------------|-----------|-------------|------|

No accepted risks.

---

## Security Audit Trail

| Audit Date | Threats Total | Closed | Open | Run By |
|------------|---------------|--------|------|--------|
| 2026-10-19 | 14 | 14 | 0 | gsd-security-auditor (retroactive-STRIDE) + orchestrator fix for T-01-14 |

---

## Sign-Off

- [x] All threats have a disposition (mitigate / accept / transfer)
- [x] Accepted risks documented in Accepted Risks Log
- [x] `threats_open: 0` confirmed
- [x] `status: verified` set in frontmatter

**Approval:** verified 2026-10-19
