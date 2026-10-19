---
phase: 01-application-and-inference
plan: "27"
title: "Route indexing and query embeddings through the shared executor"
completed_at: "2026-10-19"
requirements: ["INF-02", "AUTH-04", "AUTH-06", "JOB-03", "JOB-04"]
---

# 01-27 Summary — embeddings through the shared executor

## What changed

- `internal/rag/service.go`: dropped the direct `llm.Provider` and
  `*billing.Service` dependencies; the service now holds the single
  `Executor` seam (`Execute(ctx, Principal, inference.Request)`, satisfied
  by `*inference.Service`). `NewService(repo, vectorStore, executor,
  defaultModel, artifactStore, jobSvc)` — no provider, no per-call
  Reserve/Settle bookkeeping. Indexing `attemptEmbed` dispatches each
  bounded batch through the executor with the frozen job principal, the
  immutable `source.ObjectRef{ObjectID, ObservedSeq}` input plus the
  durable original-artifact ref, and the job/attempt ids — one priced
  executor attempt per batch, liability retained on ambiguous outcomes
  inside `Execute`. `Search` passes the caller's authorized collection
  object set as `Inputs` so query embeddings resolve the same merged
  restrictions; pasted text without source linkage still runs under
  tenant/actor policy, never an implicit classification. The
  `OperationEmbed+":query"` pseudo-capability and `embedText` billing shim
  are gone — both paths key the canonical `embeddings` capability.
- `internal/rag/embedding.go`: `createEmbeddings`/`createEmbeddingBatch`
  call `executor.Execute` instead of `provider.CreateEmbeddings`. Batches
  are bounded by item count (2048) AND summed per-input unit estimate
  (1<<20, inside the executor's embeddings bound) rather than assuming a
  fixed-size batch fits every route. Count/index/dimension validation,
  duplicate/missing-index checks and context cancellation are preserved.
- Qdrant point IDs are now real UUIDs (`newPointID()`); the durable chunk
  row id stays a payload reference — the previous `ragchunk_*` id was
  rejected by live Qdrant.
- `cmd/enclava/main.go`: `rag.NewService` receives the shared
  `inferenceSvc` executor; the jobs runner now also registers
  `rag.OperationEmbed` so enqueued embed jobs actually dispatch.
- `internal/rag/service_test.go`, `internal/connectors/rag_integration_test.go`:
  fixtures compose the real `inference.Service` over postgres repos with a
  deterministic fake `inference.Upstream`, a seeded embeddings endpoint
  (`model:embed-test` binding) and an input-unit price — routing, policy
  and pricing run for real.
- `internal/rag/qdrant_integration_test.go`: requires
  `QDRANT_INTEGRATION_URL` (fatal if missing) + `ENCLAVA_TEST_DATABASE_URL`.
  `TestEmbeddingExecutorIntegration` (opt-in via
  `OPENAI_CONTRACT_BASE_URL`/`OPENAI_CONTRACT_API_KEY`, optional
  `ENCLAVA_TEST_EMBEDDING_MODEL`) runs the full live path: real executor
  with secret-resolved credential → real embedding upstream → real Qdrant
  upsert and search.

## Verification performed

- `ENCLAVA_TEST_DATABASE_URL=postgres://enclava:***@127.0.0.1:5432/enclava
  QDRANT_INTEGRATION_URL=http://127.0.0.1:6333 go test -count=1
  ./internal/rag ./internal/connectors ./cmd/enclava` — all pass.
- `TestEmbeddingExecutorIntegration` run against a local
  OpenAI-compatible stub at `OPENAI_CONTRACT_BASE_URL`: upload → fenced
  embed job → real `Execute` dispatch over HTTP → Qdrant upsert → search
  returns the document (PASS).
- `go test ./internal/httpx ./internal/chatbot ./internal/inference
  ./internal/jobs` — pass (no regressions in dependents).
- `go build ./...`, `go vet` on changed packages — clean.

## Notes

- A live third-party embedding endpoint was not exercised (no external
  credentials available); the executor path was verified end-to-end
  against a protocol-identical stub upstream.
- `internal/connectors/service_test.go` was already unformatted at base;
  left untouched.
