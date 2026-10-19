# Enclava company platform

## What This Is

One greenfield Go application for company knowledge, source-aware search, agents, customer-local/confidential inference routing, and narrow managed extraction/vision use cases. It consumes inference endpoints that customers or confidential-inference providers operate; it does not host general-purpose models or manage GPUs.

The application lives in `mini-enclava-go/`. Existing Go and Python implementations are reuse material, not compatibility obligations. The embedded HTML/JavaScript UI and every API, worker and agent use the same identity, authorization, execution and accounting boundaries.

## Core Value

Give a company one permission-aware place to use its knowledge and AI capabilities without sending data to an unauthorized processing destination.

## Requirements

### Validated

None for the new application. Existing implementations and earlier synthetic search probes are not evidence that the new platform requirements are complete.

### Active

See [REQUIREMENTS.md](REQUIREMENTS.md) for individually traceable requirements.

- One PostgreSQL application store and durable filesystem artifacts, with tenant ownership and immutable evidence versions.
- Local accounts plus OIDC, revocable sessions/service keys, explicit identity linking and delegated-agent authority.
- Customer-local and confidential inference, OpenAI core APIs plus Responses, exact usage accounting and bounded concurrency.
- Structural document processing, legal chunking controls, source spans and usable previews.
- Asynchronous structured-extraction and image-recognition jobs that work without a knowledge index.
- Slack, Teams, GitHub, Notion and Google Drive connectors, including meaningful source references and explicit authorization semantics.
- Qdrant keyword/dense retrieval, bounded context reconstruction and server-resolved citations.
- Configurable assistants, persistent conversations, built-in knowledge/extraction tools and explicitly approved outbound MCP tools.
- Real embedded product screens with keyboard access, truthful status and errors.

### Outside This Build

- Legacy customer/data migration, compatibility aliases or a second application runtime. There are no live customers to migrate. Normal initialization/versioning of the new schema remains necessary.
- Deployment, infrastructure provisioning, reproducible-build pipelines, production-hardening programs, release processes or compliance certification. Application security at trust boundaries remains in scope.
- Model hosting, GPU scheduling, a general workflow engine, arbitrary-code tools, an inbound MCP server or a generic webhook/delivery platform.
- Separate reranking services, graph databases or open-ended agentic retrieval without a demonstrated need and a new decision.
- Synchronous extraction/waiting mode and reviewer-correction workflows. Neither is part of the selected asynchronous processing contract.

### Deferred by the User

Healthcare-specific interfaces and clinical-record interoperability are not needed now. Do not build FHIR, CDA/CDA-CH, HL7 v2, EPD/IHE, openEHR, patient/encounter query models or EHR write-back in these phases. Generic sensitive documents still receive the shared access, provenance, retention and processing-policy controls; this is not a claim of clinical correctness, automatic PHI detection or healthcare compliance.

## Confirmed Decisions

<decisions>

- **D-01 — Go, greenfield:** implement one Go application in this repository; reuse useful code without preserving old data or behavior for compatibility.
- **D-02 — PostgreSQL:** one primary application metadata, identity, job and accounting store. No SQLite compatibility backend.
- **D-03 — Qdrant:** use Qdrant for dense and native lexical retrieval with an explicit fusion rule; do not add a second keyword engine by default.
- **D-04 — Embedded UI:** HTML templates and JavaScript served by the Go application. No React/Next.js or Node server in this application.
- **D-05 — Inference API:** OpenAI models, chat completions including streaming/tool calls/vision input, embeddings, and Responses. No unrestricted OpenAI-product passthrough.
- **D-06 — Login:** both OIDC and local accounts, plus API/service keys. Account linking is explicit; identical email addresses do not automatically merge identities.
- **D-07 — Notion authorization:** an authorized publisher selects content and explicitly assigns Enclava audiences. This is application-managed publishing, not a claim of native Notion sharing parity.
- **D-08 — Documents:** Go plus native Poppler/Tesseract processing, not a Docling/Python service. Native executables and OCR assets are runtime prerequisites, not part of a single-file stack claim.
- **D-09 — Agents:** built-in knowledge/extraction assistants plus explicitly approved outbound MCP tools. No arbitrary-code execution or generic workflow engine.
- **D-10 — Artifacts:** durable filesystem originals/results; all workers processing the same work must see the same durable data directory. Scratch files have a separate lifecycle.
- **D-11 — Processing API:** asynchronous jobs only: persist input, return a job ID, expose status/result/cancel/delete.
- **D-12 — Healthcare:** defer healthcare-specific interfaces and clinical-record interoperability for this build.
- **D-13 — Source coverage:** retain Slack, Teams, GitHub, Notion and Google Drive; source limitations must be explicit rather than silently dropping a family or widening access.
- **D-14 — Build scope:** no migration tooling, deployment/reproducibility/hardening/compliance/release workstream. No extra product choices presented as user-approved assumptions.

</decisions>

## Implementation Constraints

- Follow [CONTRACTS.md](CONTRACTS.md) for the shared interfaces and invariants. These are implementation decisions for the selected product, not additional user-selected features.
- Use existing package boundaries and small repository interfaces where useful. No backend factory, new queue service or duplicate retrieval/accounting stack.
- PostgreSQL and Qdrant remain separate services; filesystem artifacts and native processors remain explicit prerequisites. The Go binary contains the application and its UI, not every dependency.
- A private inference URL is not a connectivity solution. A customer-local endpoint must already be reachable from the application; building VPNs, relays or tunnels is outside this work.
- Do not infer confidential assurance from a hostname, model name or successful health check. Retain verified confidential transport and reject unsupported assurance requirements.
- Verified confidential inference is not an attestation or encryption guarantee for PostgreSQL, Qdrant, filesystem storage or the entire deployment. Their placement/assurance remains outside this application-build plan; do not market the unfinished stack as certified or end-to-end confidential.
- Source permission synchronization is eventually observed. A recorded revocation must affect reads/citations/context immediately; do not promise instant knowledge of unsynced upstream changes.
- Source-native ACLs remain required except for the explicitly selected Notion publishing model and explicitly owned uploads. Unknown required grants fail closed.
- Read-only third-party source credentials/workspaces and real configured inference endpoints are required for integration verification. Missing credentials are a visible prerequisite, never permission to substitute a fake successful integration.
- Keep public APIs and model-produced text from inventing authority, source links or factual evidence. A citation proves provenance, not answer correctness.
- Retain applicable source/dependency notices. Onyx enterprise permission-sync code is not covered by the community MIT retrieval license.

## Source and Authority

Imported from `docs/company-platform-implementation-plan.md` on 2026-10-19.

Source SHA-256: `e238bf62e7041063741f07a3e6a6d5cbfd675c687d4ed476dd235ff4ff33448d`.

The source plan, its audit and HTML presentations remain unchanged as prior planning artifacts. This project contract records the subsequent confirmed choices and healthcare deferral; those decisions take precedence over the source document's open-choice and clinical sections. The previous Grok review covered an earlier source-plan snapshot, not these executable GSD plans.

`../enclava/`, `../onyx/`, `../ODS/` and their planning/history are references only. Do not delete, modify or migrate them as part of this application cutover. Do not delete local `.env`, database files or unrelated user data.

*Last updated: 2026-10-19 during executable-plan import. Application implementation has not started under this plan.*
