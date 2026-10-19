# Requirements

Scope: the confirmed decisions in PROJECT.md override unresolved choices and healthcare-specific material in the source plan. All requirements below are unimplemented under this imported plan.

## Phase 1: Application and inference

- [ ] **CORE-01**: One Go application and embedded UI, reusing useful code without a Python/Node application runtime or compatibility layer.
- [ ] **CORE-02**: PostgreSQL owns tenant-scoped application metadata, identity, jobs and accounting; fresh schema initialization and isolated fixtures are real, not an SQLite driver swap.
- [ ] **CORE-03**: Durable filesystem originals/results are finalized before acknowledgement, restart-safe, bounded and protected from path traversal and cross-tenant reads.
- [ ] **CORE-04**: Immutable connection-qualified source/content/profile/result versions, provenance, grants and server-owned processing restrictions have explicit storage contracts.
- [ ] **CORE-05**: Complete the application cutover: migrate code callers to the selected store/executor, remove obsolete SQLite/import commands and dead aliases, and do not alter other repositories or user data.
- [ ] **AUTH-01**: Administrator-controlled local accounts use secure password storage and tenant-scoped roles; no public signup or implicit administrator actor.
- [ ] **AUTH-02**: OIDC validates issuer/subject/state/nonce/PKCE and supports explicit account linking without email-based account merging.
- [ ] **AUTH-03**: Revocable server sessions and API/service keys enforce expiry, scopes, CSRF where applicable and immediate subsequent-request revocation.
- [ ] **AUTH-04**: Every API, worker and agent carries a validated tenant/actor/delegation/billing identity; no IsAdmin or key-ID-zero bypass.
- [ ] **AUTH-05**: Authorization handles tenant/resource ownership, explicit audiences, effective source grants, membership freshness and revocations; unknown required permissions deny access.
- [ ] **AUTH-06**: Persisted processing restrictions are inherited by derivatives and may only be tightened by requests; authorize referenced data before exposing metadata or content.
- [ ] **JOB-01**: Durable jobs freeze inputs and submitting authority before acknowledgement; idempotency detects conflicting reuse and preserves distinct A-to-B-to-A observations.
- [ ] **JOB-02**: Leases, fencing, bounded safe retries, cancellation and tombstones prevent stale workers from publishing or resurrecting work after restart.
- [ ] **JOB-03**: Atomic cross-process reservations and exact immutable price snapshots enforce tenant and billing-subject budgets before bounded paid execution.
- [ ] **JOB-04**: Usage/outcomes settle once without clipping or float drift; unknown outcomes retain liability and are not refunded or replayed automatically.
- [ ] **INF-01**: Tenant-scoped endpoint clients support reachable customer-local and verified confidential inference with explicit capabilities, assurance and origin-bound credentials.
- [ ] **INF-02**: One executor enforces actor, tenant, input and destination restrictions plus accounting for every public/internal inference stage; no alternate direct-provider path.
- [ ] **INF-03**: OpenAI models, chat completions/streaming/tool calls/vision input and embeddings preserve supported wire semantics and expose truthful capability errors.
- [ ] **INF-04**: Responses create/stream/read/delete/cancel/input-items/input-token/compact operations retain owner, endpoint, input and policy bindings across follow-up calls.
- [ ] **INF-05**: Disconnect, timeout and cancellation close upstream work; no unsafe cross-endpoint retry after possible acceptance or partial output.
- [ ] **INF-06**: Nested resources, tools and remote destinations receive authorization/egress checks; unknown shared-provider IDs and unrestricted passthrough are rejected.
- [ ] **CON-01**: Active requests, queued requests/bytes, streams and internal fan-out are bounded; permits survive until bodies close and cancel promptly under overload.
- [ ] **CON-02**: Two running application processes demonstrate authoritative shared admission/accounting and distinguish per-process capacity from global limits.
- [ ] **UI-01**: Embedded HTML/JavaScript provides real local/OIDC login, logout, identity/key/role controls and accessible navigation with server-side authorization.
- [ ] **UI-02**: Provider/routing/policy and usage/budget screens show actual capability, assurance, reservation and failure state without exposing credentials.

## Phase 2: Structured document processing

- [ ] **DOC-01**: Real Go/Poppler/Tesseract processing handles supported text/Markdown/HTML/CSV/JSON/DOCX/PDF/image bytes with truthful MIME/page/pixel/size errors.
- [ ] **DOC-02**: Ordered blocks, hierarchy, immutable extracted text and actual page/paragraph/table/message/code locations produce stable source spans without invented structure.
- [ ] **DOC-03**: Versioned legal and content-aware chunk profiles preserve clause/parent/exception/table/footnote context and expose bounded size/overlap/boundary controls.
- [ ] **DOC-04**: The actual embedded preview shows the same chunks, spans, locations and warnings that downstream indexing will use; profile changes are versioned.
- [ ] **DOC-05**: Native parsers run with bounded resources, isolated scratch/process trees, no network or application secrets; cancellation and malformed input cannot leak retained data.
- [ ] **DOC-06**: A maintained local JSON Schema validator enforces the selected dialect, exact numeric values and local-only references, rejecting unsupported schema features explicitly.

## Phase 3: Standalone extraction and vision

- [ ] **PROC-01**: Asynchronous extraction/vision APIs persist inputs, return job IDs and provide restart-safe status/result/cancel/delete with no synchronous waiting mode.
- [ ] **PROC-02**: Structured extraction and image-recognition tasks use real configured models through the shared executor and locally validate schema/label outputs.
- [ ] **PROC-03**: Owner-authorized immutable input/task/model/result versions retain source evidence, processing restrictions and truthful errors/usage; no fabricated certainty or locations.
- [ ] **PROC-04**: Standalone processing and its usable embedded screens work without Qdrant, connectors or a knowledge corpus.
- [ ] **PROC-05**: Cancellation, explicit deletion and retention remove only authorized owned artifacts, fence stale results and do not automatically publish generated outputs into search.

## Phase 4: Connected knowledge and search

- [ ] **SRC-01**: GitHub sync ingests selected repository content and supported issue/PR context with pagination, edits/deletes, qualified identities, native links and verified access.
- [ ] **SRC-02**: Slack sync preserves channel/thread/message structure, authorship, edits/deletes, cursor progress and effective channel/private membership without replacing history with a delta page.
- [ ] **SRC-03**: Teams sync handles selected channel/chat content and standard/private/shared access semantics with stable source links, bounded pagination and explicit unsupported-scope errors.
- [ ] **SRC-04**: Notion sync handles selected recursive structured content and real page links under explicit publisher-selected Enclava audiences; no native-sharing-parity claim.
- [ ] **SRC-05**: Google Drive sync handles selected files and native-document exports with meaningful locations/revisions, pagination/deltas and inheritance/shared-drive/restriction-aware access.
- [ ] **SRC-06**: Connector orchestration durably checkpoints content, grant and membership observations separately; incomplete scans never imply authoritative deletion or company-wide access.
- [ ] **SRC-07**: Source management screens configure selected scopes/audiences, run/cancel sync, and expose actual permission freshness, progress, errors and original/retained source locations.
- [ ] **IDX-01**: Ingestion stages immutable parsing/chunk/embedding/index generations and publishes only complete authorized versions; failures retain the last still-authorized generation.
- [ ] **IDX-02**: Recorded revocations/deletions/connection disablement hide affected content immediately while fenced idempotent cleanup removes stale artifacts/index points without harming a newer generation.
- [ ] **IDX-03**: Explicit authorized import of uploaded/processing-result artifacts preserves provenance and inherited restrictions; generated results are never mislabeled original or indexed automatically.
- [ ] **SEARCH-01**: Qdrant native BM25 and dense vectors fuse by an explicit supported RRF configuration, with direct lexical-only retrieval and no second keyword service.
- [ ] **SEARCH-02**: Both retrieval branches enforce candidate tenant/access/generation filters and authoritative reauthorization before snippets, expansion, counts or evidence are returned.
- [ ] **SEARCH-03**: Context reconstruction respects hierarchy, actual source spans and model bounds, deduplicating identical spans without merging distinct legal clauses or exceptions.
- [ ] **SEARCH-04**: Server-resolved evidence IDs provide authorized native links, retained-copy views and real locations; stale/fabricated/forbidden IDs cannot leak source details.
- [ ] **SEARCH-05**: Bounded optional query rewriting/section selection uses the shared executor and fails safely to direct retrieval rather than introducing open-ended model loops.
- [ ] **SEARCH-06**: Real Qdrant and application runs demonstrate useful keyword/semantic/mixed-source retrieval, citation validity, chunk-boundary behavior and revocation safety with a fixed representative corpus.

## Phase 5: Agents and approved tools

- [ ] **AGENT-01**: Assistants and durable conversations bind approved models, instructions, collections, tools and caller/delegated authority, retaining input/evidence restrictions across turns.
- [ ] **AGENT-02**: Built-in knowledge and asynchronous extraction tools call the shared retrieval/job paths rather than privileged alternate services or hidden synchronous processing.
- [ ] **AGENT-03**: Approved outbound MCP Streamable HTTP servers/tools use origin-bound credentials, bounded I/O, schema approval and inherited data restrictions; no arbitrary URLs, stdio or code execution.
- [ ] **AGENT-04**: Agent runs persist bounded tool attempts, cancellation and unknown-side-effect outcomes; revoked source text is removed from reusable context and unsafe retries are refused.
- [ ] **AGENT-05**: Usable embedded assistant/conversation screens show evidence, real tool/job progress and errors; end-to-end app runs exercise both local/confidential routing and standalone-plus-knowledge use cases.

## Explicit exclusions

No customer/data migration, deployment/provisioning/reproducible-build program, model hosting, general workflow engine, arbitrary-code tools, clinical-record interfaces or healthcare compliance claims. No automatic Notion native-sharing parity, synchronous extraction mode or correction-review workflow. See PROJECT.md for the complete confirmed exclusions.

## Traceability

| Requirement | Delivery phase | Status |
|---|---|---|
| CORE-01 | 1 | Planned; not implemented |
| CORE-02 | 1 | Planned; not implemented |
| CORE-03 | 1 | Planned; not implemented |
| CORE-04 | 1 | Planned; not implemented |
| CORE-05 | 1 | Planned; not implemented |
| AUTH-01 | 1 | Planned; not implemented |
| AUTH-02 | 1 | Planned; not implemented |
| AUTH-03 | 1 | Planned; not implemented |
| AUTH-04 | 1 | Planned; not implemented |
| AUTH-05 | 1 | Planned; not implemented |
| AUTH-06 | 1 | Planned; not implemented |
| JOB-01 | 1 | Planned; not implemented |
| JOB-02 | 1 | Planned; not implemented |
| JOB-03 | 1 | Planned; not implemented |
| JOB-04 | 1 | Planned; not implemented |
| INF-01 | 1 | Planned; not implemented |
| INF-02 | 1 | Planned; not implemented |
| INF-03 | 1 | Planned; not implemented |
| INF-04 | 1 | Planned; not implemented |
| INF-05 | 1 | Planned; not implemented |
| INF-06 | 1 | Planned; not implemented |
| CON-01 | 1 | Planned; not implemented |
| CON-02 | 1 | Planned; not implemented |
| UI-01 | 1 | Planned; not implemented |
| UI-02 | 1 | Planned; not implemented |
| DOC-01 | 2 | Planned; not implemented |
| DOC-02 | 2 | Planned; not implemented |
| DOC-03 | 2 | Planned; not implemented |
| DOC-04 | 2 | Planned; not implemented |
| DOC-05 | 2 | Planned; not implemented |
| DOC-06 | 2 | Planned; not implemented |
| PROC-01 | 3 | Planned; not implemented |
| PROC-02 | 3 | Planned; not implemented |
| PROC-03 | 3 | Planned; not implemented |
| PROC-04 | 3 | Planned; not implemented |
| PROC-05 | 3 | Planned; not implemented |
| SRC-01 | 4 | Planned; not implemented |
| SRC-02 | 4 | Planned; not implemented |
| SRC-03 | 4 | Planned; not implemented |
| SRC-04 | 4 | Planned; not implemented |
| SRC-05 | 4 | Planned; not implemented |
| SRC-06 | 4 | Planned; not implemented |
| SRC-07 | 4 | Planned; not implemented |
| IDX-01 | 4 | Planned; not implemented |
| IDX-02 | 4 | Planned; not implemented |
| IDX-03 | 4 | Planned; not implemented |
| SEARCH-01 | 4 | Planned; not implemented |
| SEARCH-02 | 4 | Planned; not implemented |
| SEARCH-03 | 4 | Planned; not implemented |
| SEARCH-04 | 4 | Planned; not implemented |
| SEARCH-05 | 4 | Planned; not implemented |
| SEARCH-06 | 4 | Planned; not implemented |
| AGENT-01 | 5 | Planned; not implemented |
| AGENT-02 | 5 | Planned; not implemented |
| AGENT-03 | 5 | Planned; not implemented |
| AGENT-04 | 5 | Planned; not implemented |
| AGENT-05 | 5 | Planned; not implemented |

57 requirements; each must be referenced by at least one executable plan in its delivery phase. Cross-phase consumers also reference the relevant shared requirements. No requirement is marked complete by importing a plan.
