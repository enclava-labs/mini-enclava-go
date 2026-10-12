# Fix tracker

Status: done
Date: 2026-10-12
Branch: `main` (uncommitted platform work plus these fixes)

This is the living record for the review findings. Update the status column as
work lands. `docs/review-remediation-plan.md` is the earlier draft. Where it
disagrees with this file, this file wins.

## What the uncommitted tree already does

The working tree adds OpenAI Responses and a scoped `/v1` passthrough, RAG
(SQLite plus Qdrant), chatbots and prompt templates, GitHub/Slack/Notion/Linear
connectors, pricing and budgets, dashboard pages, API-key chatbot and agent
allowlists, Docker packaging, and a Postgres JSONL importer. Inference still
uses the env-selected provider from `buildProvider`.

## Architecture rules

Keep the current package layout. Fix the behavior in the function that owns it.

Rejected as extra machinery:

- No `internal/billing` package. Budget checks stay in `internal/httpx`. RAG
  calls a small optional guard wired from `main`.
- No `CONNECTOR_ALLOWED_CREDENTIAL_REFS` or `CONNECTOR_ALLOWED_BASE_URLS`.
  A name prefix plus a private-host block closes the secret and SSRF hole.
  Public `https` base URLs stay valid so GitHub Enterprise still works.
- No multi-provider router. `provider_configs` is editable metadata. Boot must
  stop overwriting it. The process keeps using `INFERENCE_PROVIDER`.
- No Qdrant API-key client. Compose binds the published ports to loopback.
  The app still reaches Qdrant on the compose network.
- No new public JSON money field. Microcents are internal. Existing cent
  fields stay, rounded only for display.
- No new dependency and no new env var.

## Streams

Streams own disjoint files so they can land together. Do not edit another
stream's files.

| Stream | Owns | Findings |
|---|---|---|
| A connectors | `internal/connectors/service.go`, `internal/connectors/service_test.go`, a comment in `.env.example` | 1 |
| B accounts | usage repo, API-key cost totals, `internal/httpx/budgets.go`, migration `014`, `cmd/enclava/migrate_import.go` and its test, pricing repo test | 3, 4, 10 |
| C rag | `internal/rag/**`, `internal/store/sqlite/rag_repo.go`, `cmd/enclava/main.go`, new `internal/httpx/rag_spend.go` | 5, 7, 8, 9, 18 |
| D chatbot | `internal/chatbot/**`, `internal/httpx/chatbot.go`, `internal/httpx/prompts.go` | 11, 12, 13, 17 |
| E proxy | `internal/httpx/openai_passthrough.go`, `internal/httpx/responses.go`, `internal/httpx/middleware.go`, `internal/llm/openai_adapter.go`, their tests, `internal/httpx/openai_contract_test.go` | 6, 14, 15, 21 |
| F small | `.dockerignore`, `docker-compose.yml`, `internal/store/sqlite/providers_repo.go` and its test, `internal/extract/service.go` and its test, one sentence in the providers dashboard template if it claims the row routes traffic | 2, 16, 19, 20 |

Finding 7 stays inside RAG. `UploadDocument` replaces an existing document when
`SourceID` is set. Connectors keep calling `UploadDocument`.

## Findings

| ID | Sev | Status | Stream | Pragmatic fix |
|---|---|---|---|---|
| 1 | bug | done | A | `credential_ref` must match `CONNECTOR_<TYPE>_[A-Z0-9_]+` for that source type. `base_url`, when set, must be `http` or `https`, have no userinfo, and must not be loopback, private, or link-local unless the service test hook allows it. Credentialed client does not follow redirects. Validate on create, update, and before sync fetches. |
| 2 | bug | done | F | `.dockerignore` ignores `.env` and `.env.*`, then re-includes `.env.example`. |
| 3 | bug | done | B | Add `usage_records.cost_microcents` and `api_keys.total_cost_microcents` (`1 cent = 1_000_000`). Round once into microcents. Budgets compare microcents. Keep `cost_cents` as the rounded display value. The dashboard budget partial uses the same window and converts microcents back to cents for display. |
| 4 | bug | done | B | Period window is the later of `starts_at` and the current UTC day or month. `total` with `starts_at` sums from activation. `total` without it stays lifetime. |
| 5 | bug | done | C | Optional `SpendGuard` on `rag.Service`. Before each embedding call with `APIKeyID > 0`, run the existing budget check. After success, record an embeddings usage row. Key id `0` stays unbilled. Nil guard keeps current tests working. |
| 6 | bug | done | E | Budget-check and record usage only for `POST` whose endpoint is exactly `/responses`. Cancel and other subroutes are not gated and do not write a zero-token usage row. |
| 7 | bug | done | C | Non-empty `(collection, source_type, source_id)` reuses one document id, deletes the previous points, and replaces chunks. Extra legacy duplicates are deleted on that sync. Uploads without a source id still insert. |
| 8 | bug | done | C | Next chunk start is `end - overlap`. If that does not advance, start at `end`. |
| 9 | bug | done | C | One 32 MiB expanded-byte cap for the whole DOCX. Reject when the next part would exceed it. Do not truncate. Close every zip entry. |
| 10 | bug | done | B | `INSERT` plus `ON CONFLICT(<primary key>) DO NOTHING`. Count `RowsAffected`. Any other constraint error aborts and rolls back. Report inserted and already-present separately. |
| 11 | bug | done | D | Resuming a conversation requires the same chatbot and, unless `IsAdmin`, the same non-nil API key. Mismatch is `sql.ErrNoRows`. |
| 12 | bug | done | D | Retrieval for collections stored on the bot uses service access for those ids only. Do not mark collections public. A missing collection is not reported as "chatbot not found". |
| 13 | bug | done | D | List and get chatbots honor `ChatbotAllowed`. An empty allowlist still means all. Get of a disallowed bot is 404. Prompt-template reads return only templates referenced by visible bots. |
| 14 | bug | done | E | Reject proxy paths that contain `..`, a backslash, or a scheme or host. Join onto the provider base and require the result to stay on that origin and under that base path. |
| 15 | bug | done | E | Keep a bounded tail of the passthrough body and parse usage from the tail. A JSON body with `"stream": true` uses the untimed client, including the Tinfoil passthrough. The client still receives the full body. |
| 16 | bug | done | F | If the value is nil and the schema type allows null, skip object and array structural checks. |
| 17 | bug | done | D | 400 only for validation. Deadline is 504. Provider or Qdrant connectivity is 502. Anything else is a generic 500. Log the real error. Do not return `err.Error()` for those cases. |
| 18 | bug | done | C | Embed at most 2048 inputs per call. Preserve input order. Fail before any Qdrant write if a later batch disagrees on dimension or is malformed. Record each successful batch when a guard is set. |
| 19 | bug | done | F | `EnsureDefault` inserts the env provider only when that id is absent. It does not update an existing row. The providers template does not claim the row selects the live backend, so it was left unchanged. |
| 20 | bug | done | F | Compose publishes Qdrant `6333` and `6334` on `127.0.0.1` only. |
| 21 | suggestion | done | E | CORS allows `PATCH`. Reflect only gateway headers plus `Idempotency-Key`, `X-Client-Request-Id`, and `OpenAI-*`. Set `Vary` when the allow list comes from the preflight request. |

## Verification

`gofmt` is applied. `go test ./...` passed on 2026-10-12.

Follow-up after the streams landed:

- Dashboard budget rows were still reading `SumSince` as cents. They now use `usedForPeriod` and convert microcents for display.
- The Tinfoil passthrough now passes the request body into the stream-client check, same as the OpenAI-compatible adapter.

No billing package, credential allowlist env vars, provider router, or Qdrant API-key client was added.

## Notes

- Scheduled connector sync runs as admin with no API key. Those embeddings stay unbilled. Key-attributed sync, search, upload, and chatbot retrieval are billed.
- Historical rows backfill microcents as `cost_cents * 1_000_000`. That preserves already-rounded amounts. New writes round from the pricing snapshot into microcents first.
