---
phase: "01"
slug: "application-and-inference"
# status lifecycle: draft (seeded by plan-phase) → validated (set by validate-phase §6)
status: validated
nyquist_compliant: true
wave_0_complete: true
created: "2026-10-19"
---

# Phase 01 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | go test (Go 1.26) + Playwright (dashboard) + OpenAI Go SDK contract suite |
| **Config file** | none — stdlib go test; Playwright config under tests/ |
| **Quick run command** | `ENCLAVA_TEST_DATABASE_URL=<pg> go test ./internal/... -count=1` |
| **Full suite command** | `ENCLAVA_TEST_DATABASE_URL=<pg> QDRANT_INTEGRATION_URL=<qdrant> go test $(go list ./... | grep -v tests/contract)` then `OPENAI_CONTRACT_BASE_URL=<app/v1> OPENAI_CONTRACT_API_KEY=<key> OPENAI_CONTRACT_CHAT_MODEL=m OPENAI_CONTRACT_EMBEDDING_MODEL=m OPENAI_CONTRACT_RESPONSES_MODEL=m go test ./tests/contract -count=1` |
| **Estimated runtime** | ~45 seconds |

Live fixtures used throughout execution: PostgreSQL 17 (docker enclava-pg), Qdrant (127.0.0.1:6333), a composed real binary on 127.0.0.1:18444 with a stub OpenAI-compatible upstream on :18443, bootstrapped tenant `acme`.

---

## Sampling Rate

- **After every task commit:** package-scoped `go test`
- **After every plan wave:** full suite + live SDK contract run (executed all 28 waves)
- **Before `/gsd:verify-work`:** full suite green (confirmed post wave 28)
- **Max feedback latency:** ~60 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Threat Ref | Secure Behavior | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|------------|-----------------|-----------|-------------------|-------------|--------|
| 01-01 | 01 | 1 | CORE-02 | T-01-05 | validated DSN open, transactional migrations, fail-loud fixture | integration | `go test ./internal/store/postgres/ -count=1` | ✅ | ✅ green |
| 01-02 | 02 | 1 | INF-01/05 | T-01-08 | guarded dials, same-origin redirects, no-replay, watchdogs | unit | `go test ./internal/llm/ -count=1 -race` | ✅ | ✅ green |
| 01-03 | 03 | 1 | CORE-02 | — | DSN fail-closed config | unit | `go test ./internal/config/ -count=1` | ✅ | ✅ green |
| 01-04 | 04 | 2 | CORE-04/AUTH-04 | T-01-04 | contract types, allowlist all/list deny-everything | unit | `go test ./internal/auth/ -count=1` | ✅ | ✅ green |
| 01-05 | 05 | 3 | CORE-02/04 | T-01-05 | tenant-qualified identity + composite-FK schema | integration | `go test ./internal/store/postgres/ -run TestSchema -count=1` | ✅ | ✅ green |
| 01-06 | 06 | 4 | JOB-01/03, CORE-05 | T-01-11/12 | jobs/billing/domain schema integrity | integration | `go test ./internal/store/postgres/ -count=1` | ✅ | ✅ green |
| 01-07 | 07 | 5 | CORE-03 | T-01-10 | atomic finalize, tenant-isolated paths, reconciliation | unit+integration | `go test ./internal/artifacts/ ./internal/store/postgres/ -count=1` | ✅ | ✅ green |
| 01-08 | 08 | 5 | AUTH-01/03/04 | T-01-02/04 | bcrypt, revocable sessions+CSRF, delegation, tenant-scoped keys | integration | `go test ./internal/auth/ -count=1` | ✅ | ✅ green |
| 01-09 | 09 | 6 | AUTH-05/06, CORE-04 | T-01-06 | fail-closed source authorizer | unit+integration | `go test ./internal/source/ -count=1` | ✅ | ✅ green |
| 01-10 | 10 | 6 | JOB-01/02 | T-01-11 | fenced claims, no unknown-outcome replay | unit+integration | `go test ./internal/jobs/ -count=1` | ✅ | ✅ green |
| 01-11 | 11 | 6 | JOB-03/04, INF-01 | T-01-12 | exact billing, once-only settlement | integration | `go test ./internal/billing/ -count=1` | ✅ | ✅ green |
| 01-12 | 12 | 6 | AUTH-02 | T-01-01 | OIDC state/nonce/PKCE, link challenges | unit | `go test ./internal/auth/ -run OIDC -count=1` | ✅ | ✅ green |
| 01-13 | 13 | 7 | JOB-01/04 | T-01-11 | extract on artifacts+jobs+billing | integration | `go test ./internal/extract/ -count=1` | ✅ | ✅ green |
| 01-14 | 14 | 8 | AUTH-05, INF-02 | T-01-06 | rag/connectors on sources+jobs+principals | integration | `go test ./internal/rag/ ./internal/connectors/ -count=1` | ✅ | ✅ green |
| 01-15 | 15 | 9 | AUTH-04/05 | T-01-04 | prompts/chatbot principals | integration | `go test ./internal/prompts/ ./internal/chatbot/ -count=1` | ✅ | ✅ green |
| 01-16 | 16 | 10 | AUTH-03/04 | T-01-04 | identity param swap, session-aware auth, endpoint policy | integration | `go test ./internal/httpx/ -count=1` | ✅ | ✅ green |
| 01-17 | 17 | 11 | JOB-03/04 | T-01-12 | billing service cutover, budget enforcement | integration | `go test ./internal/httpx/ -run Budget -count=1` | ✅ | ✅ green |
| 01-18 | 18 | 12 | INF-01 | — | endpoints repo + provider admin | integration | `go test ./internal/httpx/ -run Provider -count=1` | ✅ | ✅ green |
| 01-19 | 19 | 13 | JOB-01 | — | extract param group cutover | integration | `go test ./internal/httpx/ -count=1` | ✅ | ✅ green |
| 01-20 | 20 | 14 | AUTH-03, UI-01 | T-01-02 | session-only dashboard, CSRF on mutations | integration | `go test ./internal/httpx/ -run DashboardSession -count=1` | ✅ | ✅ green |
| 01-21 | 21 | 15 | INF-06 | T-01-07 | operator-approved secret refs | unit | `go test ./internal/config/ ./internal/auth/ -run Secret -count=1` | ✅ | ✅ green |
| 01-22 | 22 | 16 | CORE-01/05, AUTH-01 | T-01-03 | bootstrap CLI, real-process app smoke | integration | `go test ./internal/testdb/... ./cmd/enclava/ -count=1` (apptest + bootstrap tests) | ✅ | ✅ green |
| 01-23 | 23 | 17 | INF-02 | T-01-06/07 | single authorized priced executor | integration | `go test ./internal/inference/ -count=1` | ✅ | ✅ green |
| 01-24 | 24 | 17 | AUTH-04 | T-01-03 | identity/role/key administration | integration | `go test ./internal/httpx/ -run Identit -count=1` | ✅ | ✅ green |
| 01-25 | 25 | 18 | INF-02, JOB-04 | T-01-12 | processing via shared executor | integration | `go test ./internal/extract/ -count=1` | ✅ | ✅ green |
| 01-26 | 26 | 18 | UI-01 | T-01-02 | session/CSRF dashboard UI | browser | `npx playwright test tests/dashboard.spec.ts` (29 passed, live app) | ✅ | ✅ green |
| 01-27 | 27 | 19 | INF-02 | T-01-06 | embeddings via shared executor + live Qdrant round-trip | integration | `QDRANT_INTEGRATION_URL=<url> go test ./internal/rag/ -count=1` | ✅ | ✅ green |
| 01-28 | 28 | 20 | INF-02, JOB-04 | T-01-12 | chatbot via executor, settlement on streams | integration | `go test ./internal/chatbot/ ./internal/httpx/ -count=1` | ✅ | ✅ green |
| 01-29 | 29 | 21 | INF-03/06 | T-01-13 | public OpenAI core API through executor | contract | live SDK suite vs composed app (models/chat/embeddings) | ✅ | ✅ green |
| 01-30 | 30 | 22 | INF-04 | T-01-13 | owned Responses lifecycle + bindings | contract | live SDK responses lifecycle (create/retrieve/items/tokens/chain/cancel/delete) | ✅ | ✅ green |
| 01-31 | 31 | 23 | CON-01/02 | T-01-09 | bounded admission, cross-process shared budget | integration | `ENCLAVA_PROXY_INTEGRATION=1 go test ./tests -run TestProxyTwoProcess` (passed) | ✅ | ✅ green |
| 01-32 | 32 | 24 | INF-01/06, UI-02 | T-01-07 | endpoint mgmt UI + processing restrictions | browser | authenticated browser: restriction save → 403 → restore → 200; assurance denial; revoked key 403 | ✅ | ✅ green |
| 01-33 | 33 | 25 | JOB-03/04, UI-02 | T-01-12 | exact usage/budget/pricing views | browser+integration | rendered held/settled/remaining over real aggregates (addendum) | ✅ | ✅ green |
| 01-34 | 34 | 26 | INF-03/05 | T-01-08 | passthrough retirement, explicit transports only | unit | `go test ./internal/llm/ -count=1` + unselected-route 404 contract | ✅ | ✅ green |
| 01-35 | 35 | 27 | AUTH-04, JOB-04 | — | legacy authority/accounting fields removed | unit | `go test ./internal/auth/ ./internal/httpx/ -count=1` | ✅ | ✅ green |
| 01-36 | 36 | 28 | CORE-01/05, INF-03 | — | single-store cutover, live core proof | integration+contract | full gate + live SDK suite + browser on final binary (addendum) | ✅ | ✅ green |
| nyquist-CORE-01 | — | post | CORE-01 | — | Go-only runtime/embedded UI | integration | `go test ./cmd/enclava/ -run TestApplicationRuntimeCutover -count=1` | ✅ | ✅ green |
| nyquist-CORE-05 | — | post | CORE-05 | — | sqlite/import tooling absent | integration | `go test ./cmd/enclava/ -run TestLegacyCutoverRemoved -count=1` | ✅ | ✅ green |
| nyquist-AUTH-01 | — | post | AUTH-01 | T-01-03 | bootstrap admin+key issuance, slug refusal | integration | `go test ./cmd/enclava/ -run TestBootstrap -count=1` | ✅ | ✅ green |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

Existing infrastructure covers all phase requirements; the three post-execution Nyquist tests were added in commit `test(01): add nyquist runtime cutover, legacy removal and bootstrap tests`.

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Confidential/attested upstream route (Tinfoil-style attestation + assurance verification) end to end | INF-01, INF-05 | No confidential/attested inference endpoint exists in this environment; the property is exercised only against protocol-equivalent plain-HTTP stubs | Configure a real confidential endpoint (origin + credential ref + attestation), verify /dashboard/providers shows assurance ACTIVE, run one chat + one responses call, confirm settlement rows and attestation timestamps |

*All other phase behaviors have automated verification, including live-app SDK contract runs and authenticated browser checks (evidence in plan SUMMARYs and orchestrator addenda).*

---

## Validation Audit

| Audit Date | Gaps found | Resolved | Escalated |
|------------|-----------|----------|-----------|
| 2026-10-19 | 9 | 8 (3 new tests + 5 satisfied by the live execution harness evidence) | 1 (confidential-attestation live check → Manual-Only) |

---

## Validation Sign-Off

- [x] All tasks have `<automated>` verify or Wave 0 dependencies
- [x] Sampling continuity: no 3 consecutive tasks without automated verify
- [x] Wave 0 covers all MISSING references
- [x] No watch-mode flags
- [x] Feedback latency < 60s
- [x] `nyquist_compliant: true` set in frontmatter

**Approval:** approved 2026-10-19
