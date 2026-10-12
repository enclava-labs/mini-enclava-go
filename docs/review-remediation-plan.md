# Review Remediation Plan

Date: 2026-08-19  
Status: Draft implementation plan; all findings are open.

## Objective

Resolve all 17 review findings without changing unrelated behavior. Security,
budget enforcement, accounting accuracy, and migration integrity are release
blockers. Each non-trivial fix must include a regression test that fails against
the current implementation.

## Implementation order

| Batch | Scope | Findings | Exit condition |
|---|---|---|---|
| 1 | Immediate secret and input-boundary containment | 1, 2, 4 | Connector configuration cannot select arbitrary secrets or destinations; Docker build context excludes local environment files; DOCX expansion is bounded. |
| 2 | Accounting and budget foundation | 3, 6, 8 | Costs accumulate in fine-grained integer units, periodic windows reset correctly, and non-generating Responses operations remain available. |
| 3 | RAG provider accounting and request sizing | 7, 14 | Every API-key-attributed embedding batch is budget-checked, recorded, and within the provider input limit. |
| 4 | RAG and connector data correctness | 9, 10 | Changed connector sources replace their prior document and chunking cannot omit input. |
| 5 | Migration integrity | 5 | Only explicit idempotency conflicts are skipped; every other constraint failure aborts and rolls back the import. |
| 6 | Proxy, tenant, schema, and HTTP correctness | 11, 12, 13, 15, 16, 17 | Long responses retain usage, JSON streaming selects the untimed client, tenant ownership is enforced, CORS matches the proxy contract, nullable schemas work, and infrastructure errors are redacted 5xx responses. |

Batch 3 depends on the cost representation and shared budget checker from batch
2. Finding 9 should be implemented after the embedding batching work so document
replacement uses the final embedding path. All other batches can be reviewed as
independent commits.

## Detailed remediation

### 1. Restrict connector credential references and outbound destinations (P1)

Current risk: `connectorToken` resolves the database-controlled
`credential_ref` with `os.Getenv`, while `connectorBaseURL` accepts a
database-controlled `config.base_url`. A key with `connectors.manage` can pair a
process secret with an attacker-controlled URL.

Remediation:

1. Replace arbitrary environment lookup with an operator-owned credential map.
   Add `CONNECTOR_ALLOWED_CREDENTIAL_REFS` as a comma-separated list of variable
   names, resolve only those names during startup, and inject the resulting map
   into `connectors.Service`. Do not call `os.Getenv` using a value read from a
   connector row.
2. Require allowed credential names to use a connector-specific namespace:
   `CONNECTOR_GITHUB_*`, `CONNECTOR_SLACK_*`, `CONNECTOR_NOTION_*`, or
   `CONNECTOR_LINEAR_*`, matching the source type. Reject a missing, malformed,
   cross-type, or non-allowlisted reference before persisting a source and again
   before every sync so legacy/imported rows cannot bypass the check.
3. Parse and canonicalize each base URL with `net/url`. Permit the built-in
   provider origin for the source type and operator-configured origins from
   `CONNECTOR_ALLOWED_BASE_URLS`; reject userinfo, fragments, non-HTTP(S)
   schemes, and every non-allowlisted origin. Match origins by normalized scheme,
   hostname, and effective port, not by string prefix.
4. Disable redirects for credentialed connector requests, or allow only redirects
   whose destination has the same validated origin. A redirect must never carry
   `Authorization` to a different origin.
5. Apply one shared validator from create, update, and sync paths. Imported or
   pre-existing invalid sources should fail closed with a validation error and a
   failed sync job; they must not make an outbound request.
6. Document the new allowlists in `.env.example` and the connector operations
   documentation. Existing official provider URLs require no custom destination
   configuration.

Files:

- `internal/config/config.go`
- `.env.example`
- `cmd/enclava/main.go`
- `internal/connectors/service.go`
- `internal/connectors/service_test.go`
- Connector documentation under `docs/`

Regression tests and acceptance criteria:

- A source using `ADMIN_PASSWORD`, `BOOTSTRAP_API_KEY`, or a provider API-key
  variable is rejected before any HTTP request.
- A Slack source cannot use a GitHub connector credential reference.
- An unallowlisted `base_url`, including loopback and link-local addresses, is
  rejected even when `allow_unauthenticated` is set.
- A credentialed redirect to a second test server is rejected and the second
  server observes no `Authorization` header.
- Official destinations and an explicitly configured test origin continue to
  work.

### 2. Exclude `.env` files from the Docker build context (P1)

Current risk: `COPY . .` receives `.env` because `.dockerignore` does not exclude
it, exposing secrets to the builder and its cache.

Remediation:

1. Add `.env` and `.env.*` to `.dockerignore`.
2. Re-include `!.env.example` because it is documentation, not a local secret.
3. Keep `COPY . .`; changing the build layout is unnecessary once the context is
   correctly filtered.

File: `.dockerignore`

Acceptance criteria:

- Docker context matching excludes `.env`, `.env.local`, `.env.production`, and
  similar local variants while retaining `.env.example`.
- A normal `docker build .` still produces the `enclava` image.

### 3. Preserve fractional cents in usage accounting (P1)

Current risk: `calculateUsageCostCents` rounds each request independently. A
request costing less than half a cent is stored as zero, so repeated requests can
avoid cost budgets permanently.

Remediation:

1. Introduce one canonical integer unit: microcents, where
   `1 cent = 1,000,000 microcents`. Do not introduce a decimal dependency and do
   not use floating-point totals for budget comparisons.
2. Add migration `014_usage_cost_microcents.sql` with
   `usage_records.cost_microcents` and `api_keys.total_cost_microcents`, both
   `INTEGER NOT NULL DEFAULT 0`. Backfill from existing cent columns by
   multiplying by `1,000,000`; this preserves all already-recorded amounts.
3. Calculate and round once to microcents from the stored pricing snapshot.
   Continue deriving legacy `cost_cents` fields only for compatibility/display;
   they are no longer the source of truth.
4. Update `UsageRepo.Record` to write microcents and atomically increment the API
   key's microcent total in the existing transaction. If a caller supplies only
   legacy `CostCents`, convert it to microcents before writing.
5. Change budget aggregation and comparisons to microcents. Convert a cent limit
   to microcents with checked integer multiplication. Round only after aggregate
   totals when producing existing cent-denominated API/dashboard fields.
6. Update usage summary queries, API-key total recomputation, migration import,
   and pricing tests so every internal financial total comes from the canonical
   microcent columns.

Files:

- `internal/store/sqlite/migrations/014_usage_cost_microcents.sql`
- `internal/store/sqlite/usage_repo.go`
- `internal/store/sqlite/models.go`
- `internal/store/sqlite/api_keys_repo.go`
- `internal/store/sqlite/pricing_repo_test.go`
- `internal/httpx/budgets.go` or the shared budget component introduced for finding 7
- `cmd/enclava/migrate_import.go`
- `cmd/enclava/migrate_import_test.go`

Regression tests and acceptance criteria:

- Ten records of 1,000 embedding tokens priced at `0.12` cents per 1K produce
  `1.2` aggregate cents in microcents and exhaust a one-cent budget.
- A single sub-cent record remains non-zero in `cost_microcents`.
- Existing databases backfill old cent totals exactly.
- Existing cent-denominated response fields remain compatible and are rounded
  only after aggregation.

### 4. Cap aggregate DOCX decompression (P1)

Current risk: every selected DOCX entry can independently expand to 32 MiB and
all extracted strings accumulate in memory.

Remediation:

1. Define one aggregate expanded-XML limit for the complete DOCX extraction
   (32 MiB is sufficient and preserves the current per-entry ceiling).
2. Track the total bytes read from `word/document.xml`, headers, and footers.
   Before reading an entry, reject a declared uncompressed size larger than the
   remaining allowance.
3. Read at most `remaining + 1` bytes and reject when the extra byte is present.
   The current `LimitReader(limit)` silently truncates an oversized entry and must
   not be retained.
4. Close every entry on all error paths and return a stable
   `docx expanded content exceeds limit` error without including archive data.

Files:

- `internal/rag/text.go`
- `internal/rag/text_test.go`

Regression tests and acceptance criteria:

- A highly compressed archive containing many individually valid headers whose
  combined expansion exceeds the limit is rejected.
- A single entry exceeding the remaining allowance is rejected rather than
  truncated.
- A normal document with body, header, and footer still extracts all text.

### 5. Fail imports when a row is not inserted (P1)

Current risk: every importer uses `INSERT OR IGNORE`, which suppresses NOT NULL,
foreign-key, CHECK, and unrelated UNIQUE violations. `importJSONLFile` increments
the row count before knowing whether an insert happened.

Remediation:

1. Replace every `INSERT OR IGNORE` with a normal `INSERT` followed by an explicit
   `ON CONFLICT(<primary-key>) DO NOTHING`. Only the stable source primary key is
   an idempotency conflict; conflicts on email, name, external ID, key hash, or
   other unique columns must surface.
2. Require a stable primary key in each import row instead of allowing SQLite to
   generate one. Without a source key, rerunning an import cannot be idempotent.
3. Route all importer statements through a small `execImport` helper that checks
   `RowsAffected`. One row means inserted; zero rows means the explicitly handled
   primary-key conflict; any other count is an error.
4. Return inserted and already-present counts separately from
   `importJSONLFile`. `Rows` in the report must mean rows actually inserted; add
   an `existing`/`skipped_existing` field for idempotent conflicts. Do not report
   ignored rows as migrated.
5. Preserve the single transaction. Any unexpected constraint error aborts the
   import and rolls back all files before cutover totals are recomputed.

Files:

- `cmd/enclava/migrate_import.go`
- `cmd/enclava/migrate_import_test.go`

Regression tests and acceptance criteria:

- A missing required value fails with the file and line number.
- A dangling foreign key fails and leaves all target tables unchanged.
- A duplicate natural unique value with a different primary key fails.
- Reimporting the same primary keys succeeds idempotently, reports zero new rows,
  and reports the existing-row count.
- No import statement contains `INSERT OR IGNORE`.

### 6. Keep periodic budgets resetting after `starts_at` (P1)

Current risk: `usedForPeriod` returns immediately when `starts_at` is set, so a
daily or monthly budget accumulates forever after activation.

Remediation:

1. Calculate the current period boundary first: UTC day start for `daily`, UTC
   month start for `monthly`, and the zero/epoch boundary for `total`.
2. When `starts_at` is present, use the later of `starts_at` and the current
   period boundary.
3. Use cached lifetime API-key totals only for a true `total` budget without
   `starts_at`; otherwise query usage since the calculated boundary.
4. Keep all window calculations in UTC to match stored timestamps.

Files:

- `internal/httpx/budgets.go` or the shared budget component introduced for finding 7
- `internal/httpx/budgets_test.go`

Regression tests and acceptance criteria:

- A monthly budget activated in a prior month ignores prior-month usage.
- A monthly budget activated during the current month ignores usage before
  activation.
- Equivalent daily cases pass.
- `total` with `starts_at` still sums from activation, while `total` without it
  retains lifetime behavior.

### 7. Enforce budgets and record usage for RAG embeddings (P1)

Current risk: RAG search, upload, reprocess, chatbot retrieval, and connector sync
call the embedding provider inside `rag.Service`; the call bypasses HTTP budget
checks and discards the returned usage.

Remediation:

1. Move the existing budget decision logic into a small shared component (for
   example `internal/billing`) constructed from the API-key, budget, and usage
   repositories. Keep `App.EnforceBudget` as a thin delegate so existing HTTP
   call sites and error rendering remain stable.
2. Inject that same budget checker and the existing usage recorder into
   `rag.Service`. This avoids duplicating budget policy in RAG and covers direct
   RAG routes, chatbot retrieval, connector-triggered work, and future callers.
3. Pass `rag.Access` through document processing to the embedding call. Before
   each provider batch, enforce the budget when `APIKeyID > 0`. Dashboard or
   scheduler work with no attributable API key remains unrecorded rather than
   being assigned to an unrelated tenant.
4. After every successful provider batch, write a usage record with the API key,
   embedding model, prompt/total tokens from `embeddingResponse.Usage`, status
   200, and an endpoint classified as an embedding call so embedding pricing is
   selected. Use `context.WithoutCancel` plus the existing short recording
   timeout so client disconnects do not erase accounting.
5. Surface shared budget-exceeded errors as the existing 429
   `budget_exceeded` response from RAG, chatbot, and connector handlers. Other
   accounting failures must fail closed as redacted 5xx errors, not allow an
   unrecorded provider call.
6. Record each batch immediately. This lets a first batch consume the remaining
   budget and prevents a later batch in the same document from continuing after
   exhaustion.

Files:

- New shared budget implementation under `internal/billing/`
- `internal/httpx/budgets.go`
- `internal/httpx/app.go`
- `internal/httpx/rag.go`
- `internal/httpx/chatbot.go`
- `internal/httpx/connectors.go`
- `internal/rag/service.go`
- `internal/rag/embedding.go`
- `cmd/enclava/main.go`
- Related RAG, chatbot, connector, and budget tests

Regression tests and acceptance criteria:

- An exhausted API key causes RAG search, upload, and reprocess to return 429
  without calling the embedding provider.
- Chatbot RAG retrieval and API-key-triggered connector sync obey the same rule.
- Successful search and document embedding calls create usage rows with the
  returned token counts and pricing snapshots.
- Multi-batch embedding stops before the next provider call once prior recorded
  batches exhaust the budget.

### 8. Allow response cancellation after budget exhaustion (P1)

Current risk: every Responses `POST` is budget-checked, including
`/responses/{id}/cancel` and non-generating utility endpoints.

Remediation:

1. Replace the method-only condition with a route predicate.
2. Enforce budgets only for `POST /responses`, the route that creates a new
   generation. Do not budget-gate cancel, input-token calculation, retrieval,
   deletion, or other non-generating subroutes.
3. Keep usage recording conditional so a successful cancel is not recorded as a
   zero-token generation merely because it uses POST.

Files:

- `internal/httpx/responses.go`
- `internal/httpx/openai_contract_test.go`

Regression tests and acceptance criteria:

- An exhausted key can call `POST /responses/{id}/cancel`, and the upstream
  receives the request.
- The same key remains blocked from `POST /responses` with 429.
- Cancellation does not add a zero-token usage record.

### 9. Replace changed connector documents instead of appending (P1)

Current risk: a changed source hash calls `UploadDocument`, which always creates a
new random document ID. Old SQLite chunks and Qdrant points remain searchable.

Remediation:

1. Make source identity an upsert key inside `rag.Service`, where all ingestion
   paths converge: `(collection_id, source_type, source_id)` when `source_id` is
   non-empty. Ordinary uploads without a source ID continue creating new
   documents.
2. Add a repository lookup by that identity and a transactional replacement
   method that updates the existing document metadata and blob, replaces its
   chunks, and retains the document ID.
3. Split document processing from vector-store mutation. Build embeddings,
   chunks, and points first; for a replacement, delete the existing document's
   Qdrant points, upsert the new points, then commit the SQLite replacement. On a
   new-document database failure, best-effort delete the just-created points.
4. If legacy duplicates already exist for one source identity, enumerate them,
   delete all duplicate Qdrant points and SQLite rows, then retain and replace one
   canonical document. This cleanup occurs on the next changed sync without a
   separate repair command.
5. Update connector hash checkpoints only after replacement succeeds. A failed
   replacement must retry on the next sync.

Files:

- `internal/store/sqlite/rag_repo.go`
- `internal/rag/service.go`
- `internal/rag/service_test.go`
- `internal/connectors/service.go`
- `internal/connectors/rag_integration_test.go`

Regression tests and acceptance criteria:

- Sync source version A, then version B with the same `source_id`; exactly one
  document remains and it retains a stable ID.
- Search returns version B and cannot return version A.
- Qdrant contains no points from the old version.
- An unchanged hash still performs no embedding or replacement work.

### 10. Advance chunks from the actual boundary (P2)

Current risk: after `backtrackToBoundary` shortens `end`, the loop advances by the
nominal `step`, potentially jumping past characters that were never emitted.

Remediation:

1. Derive the next start from the actual boundary: `next = end - overlap`.
2. Guarantee progress with `next > start`; if trimming or an extreme overlap
   would stall, advance to `end`.
3. Keep rune indexing so UTF-8 text is not split mid-codepoint.

Files:

- `internal/rag/chunk.go`
- `internal/rag/text_test.go`

Regression tests and acceptance criteria:

- A string with a boundary in the backtracking region followed by a long URL or
  base64 token has no omitted rune when chunks are reconstructed using overlap.
- Existing overlap behavior and Unicode input remain correct.
- The loop terminates for overlap values near the chunk size.

### 11. Capture final usage from long passthrough responses (P2)

Current risk: `limitedBuffer` retains only the first 128 KiB, while JSON and SSE
usage normally appears at the end of the response.

Remediation:

1. Replace the prefix buffer with a bounded rolling tail buffer that always keeps
   the last `maxSSEEventBytes` while `io.Copy` continues streaming unchanged to
   the client.
2. For SSE, parse complete final events from the tail and retain the last valid
   usage object. Ignore a partial first event caused by the rolling boundary.
3. For JSON, add a bounded tail parser that locates the final `usage` object and
   decodes that object directly; it must not require the captured tail to be a
   complete top-level JSON document. Continue using the request model as the
   fallback when the response model was outside the retained tail.
4. Keep memory bounded and do not buffer the full upstream response.

Files:

- `internal/httpx/openai_passthrough.go`
- `internal/httpx/openai_contract_test.go`

Regression tests and acceptance criteria:

- A JSON passthrough response larger than 128 KiB with trailing usage records the
  correct prompt, completion, and total tokens.
- An SSE response with more than 128 KiB before its final usage event records the
  final usage.
- The response body received by the client is byte-for-byte unchanged.
- The capture allocation remains bounded by the configured tail size.

### 12. Select the stream client from the JSON `stream` flag (P2)

Current risk: passthrough client selection checks only `Accept` and the query
string. A JSON body containing `"stream": true` can use the normal timed client.

Remediation:

1. Pass the already-read request body into `passthroughShouldUseStreamClient`.
2. Preserve the existing `Accept: text/event-stream` and `stream=true` query
   checks.
3. For JSON content types (and an absent content type), decode only a small struct
   containing `Stream bool`; return true when the body explicitly requests
   streaming. Invalid JSON remains non-streaming and is handled by the upstream
   request path as it is today.

Files:

- `internal/llm/openai_adapter.go`
- `internal/llm/openai_adapter_test.go`

Regression tests and acceptance criteria:

- `POST /completions` with `Accept: application/json` and
  `{"stream":true}` selects the no-timeout client.
- `{"stream":false}` retains the normal timed client.
- Header- and query-selected streaming continue to work.

### 13. Enforce chatbot conversation ownership (P2)

Current risk: `ensureConversation` checks only the chatbot ID. A different API key
that learns a conversation ID can append messages to it.

Remediation:

1. When resuming a conversation, require both the same chatbot ID and, for a
   non-admin caller, a non-nil `conversation.APIKeyID` equal to
   `access.APIKeyID`.
2. Return `sql.ErrNoRows` for mismatches so the response does not reveal whether
   another tenant's conversation exists.
3. Allow `platform.admin` callers to resume a conversation for operational
   support, as required by the review finding.

Files:

- `internal/chatbot/service.go`
- `internal/chatbot/service_test.go`

Regression tests and acceptance criteria:

- Key B cannot resume key A's conversation and no message is inserted.
- The owner can resume it.
- An admin can resume it.
- A legacy conversation with a null owner is hidden from non-admin keys.

### 14. Batch document embedding requests (P2)

Current risk: all document chunks are submitted in one embedding request, which
can exceed the common 2,048-input provider limit.

Remediation:

1. Add a package constant of 2,048 inputs per provider call; no new configuration
   is needed until a supported provider requires a different limit.
2. Keep the existing one-batch request/response decoder as the primitive. Add a
   thin batching loop around it, using slices of at most 2,048 inputs.
3. Append vectors in original input order, translating batch-local indices to
   global positions. Reject missing, duplicate, out-of-range, or empty vectors.
4. Sum prompt and total token usage across batches for callers, while recording
   each batch immediately as required by finding 7.
5. Validate that vector dimensions remain consistent across batches before
   writing any Qdrant points.

Files:

- `internal/rag/embedding.go`
- `internal/rag/service.go`
- `internal/rag/service_test.go`

Regression tests and acceptance criteria:

- 2,049 inputs produce two provider calls of 2,048 and 1.
- Returned vectors preserve exact input order even when each response lists data
  out of order by index.
- Aggregated usage equals the sum of both responses.
- A malformed second batch fails before Qdrant mutation.

### 15. Advertise PATCH and passthrough headers in CORS (P2)

Current risk: the proxy accepts PATCH, `Idempotency-Key`, and `OpenAI-*` request
headers, but browser preflight does not advertise them.

Remediation:

1. Add `PATCH` to `Access-Control-Allow-Methods`.
2. Keep a fixed base allowlist for gateway headers and explicitly supported
   passthrough headers such as `Idempotency-Key` and `X-Client-Request-Id`.
3. For `Access-Control-Request-Headers`, safely reflect only names accepted by the
   gateway or the existing passthrough-header predicate, including validated
   `OpenAI-*` names. Never reflect arbitrary requested headers.
4. Add `Vary: Access-Control-Request-Headers` when the allowed header response is
   derived from the preflight request.

Files:

- `internal/httpx/middleware.go`
- `internal/httpx/middleware_test.go`
- `internal/llm/openai_adapter.go` if the passthrough-header predicate must be exported for reuse

Regression tests and acceptance criteria:

- A preflight for PATCH with `Idempotency-Key`, `OpenAI-Beta`, and
  `X-Client-Request-Id` succeeds and advertises each value.
- An unsupported custom header is not reflected.
- Existing configured-origin and credential behavior remains unchanged.

### 16. Accept null in nullable object and array schemas (P2)

Current risk: the general type check accepts null for `type: ["object", "null"]`
or `["array", "null"]`, then the structural branch incorrectly requires the
value to be an object or array.

Remediation:

1. After enum and type validation, return from structural validation when the
   actual value is nil and the schema allows `null`.
2. Continue validating nested properties/items only when the runtime value is an
   object/array. A null value in a non-nullable schema must still fail in the
   existing type check.

Files:

- `internal/extract/service.go`
- `internal/extract/service_test.go`

Regression tests and acceptance criteria:

- Nullable object and nullable array properties accept null.
- The same schemas still validate non-null object/array contents.
- Non-nullable object and array schemas reject null.
- Required means the property must exist; a present nullable property may be
  null.

### 17. Map chatbot infrastructure failures to server errors (P2)

Current risk: the final branch of `writeChatbotCompletionError` returns raw error
text with HTTP 400, even for network, timeout, database, or Qdrant failures.

Remediation:

1. Introduce one typed/sentinel chatbot validation error for caller-controlled
   failures such as invalid JSON and missing messages. Preserve 400 only for that
   class and the existing explicit authorization/not-found cases.
2. Keep `llm.UpstreamError` handling unchanged so valid upstream status/error
   envelopes are preserved.
3. Send all other preparation/provider/infrastructure failures through the
   redacted server/provider path: log the redacted internal error and return a
   stable generic 5xx body. Use 504 for `context.DeadlineExceeded`, 502 for
   provider/Qdrant connectivity, and 500 when no upstream classification is
   available.
4. Apply the same mapping to streaming preparation failures; do not expose raw
   endpoint URLs, tokens, SQL text, or network details.

Files:

- `internal/chatbot/types.go`
- `internal/chatbot/service.go`
- `internal/httpx/chatbot.go`
- `internal/chatbot/service_test.go`
- `internal/httpx/openai_contract_test.go`

Regression tests and acceptance criteria:

- Malformed chatbot input remains a 400 validation response.
- A raw provider dial/timeout error is a generic 502/504 and its original text is
  absent from the response.
- A Qdrant failure during chatbot RAG preparation is a redacted 5xx.
- Existing typed upstream errors and access errors retain their current status.

## Cross-cutting verification

Run after each batch:

```sh
gofmt -w <changed-go-files>
go test ./...
go vet ./...
```

Run before declaring the review resolved:

1. Apply migrations to both a fresh database and a copy at schema version 13;
   verify microcent backfill and row counts.
2. Run `go test ./...` with the race detector for the connector/RAG packages:
   `go test -race ./internal/connectors ./internal/rag ./internal/httpx`.
3. Build the container from a checkout containing sentinel `.env` variants and
   confirm they are absent from the build context while `.env.example` remains.
4. Run the existing optional real-Qdrant integration test when
   `QDRANT_INTEGRATION_URL` is available.
5. Re-run the OpenAI contract tests against a configured provider to verify
   passthrough headers, Responses cancellation, and long streaming behavior.

## Completion criteria

- All 17 regression tests pass and the full Go suite remains green.
- No connector-managed value can select an arbitrary process environment
  variable or outbound origin.
- All provider calls attributable to an API key are budget-checked and accounted
  in canonical fine-grained units.
- Migration reports distinguish inserted rows from existing rows and abort on
  every unexpected constraint failure.
- No stale connector document version remains searchable after a successful
  changed-source sync.
- Client-visible infrastructure errors are redacted and use 5xx status codes.

No new third-party dependency is required for this remediation.
