---
phase: 01-application-and-inference
plan: "31"
subsystem: api
tags: [concurrency, admission, proxy, billing, postgres, streaming]

requires:
  - phase: 01-30
    provides: shared inference executor, priced reservations, owned /v1/responses lifecycle
provides:
  - "inference.Admission: three bounded process-local pools (execute / manage / fan-out) with FIFO queues, per-tenant sublimits and explicit ErrOverloaded"
  - "stream permits tied to body close: active slot held until the response body actually closes, not when headers return"
  - "httpx inferenceGate: bounded /v1 in-flight count, caller queue and in-flight body bytes (known and unknown/chunked lengths)"
  - "INFERENCE_* env surface wiring active/queue/tenant/inflight/byte limits into the executor and proxy gate"
  - "two-process integration proof: shared PostgreSQL budget admission, summed process-local limits, disconnect/kill liability retention"
affects: [01-32, 01-34, agents, tests/contract, tests]

actuals:
  tasks: 2
  commits: 2

tech-stack:
  added: []
  patterns: [bounded FIFO admission pools, permit-on-body-close, gated unknown-length bodies, process-local vs authoritative-shared admission split]

key-files:
  created:
    - "internal/inference/admission.go"
    - "internal/inference/admission_test.go"
    - "tests/proxy_concurrency_test.go"
  modified:
    - "internal/inference/service.go"
    - "internal/inference/service_test.go"
    - "internal/httpx/middleware.go"
    - "internal/httpx/inference.go"
    - "internal/httpx/app.go"
    - "internal/config/config.go"
    - "cmd/enclava/main.go"
    - "internal/testdb/apptest.go"
    - "internal/httpx/identities_test.go"
    - "internal/httpx/openai_contract_test.go"
    - "internal/httpx/responses_contract_test.go"
    - "internal/httpx/root_test.go"
    - "internal/modules/extract/handlers_test.go"

key-decisions:
  - "Admission pools are strictly process-local; summing across processes is the documented semantics — authoritative shared admission stays in PostgreSQL reservations (CON-02), never in a distributed limiter."
  - "Three pools instead of one: manage (retrieve/cancel/delete) has reserved capacity so cancellation is never starved behind saturated model streams; fan-out (JobID-bound calls) has its own pool so nested embeddings/model work can never deadlock an outer execute permit."
  - "Streaming bodies bypass the attempt timeout (it would kill valid held streams); stream lifetime is bounded by the adapter's idle/wall limits, and the permit lives in accountingStream until body close."
  - "Unknown/chunked request bodies are charged per arriving byte against the byte pool via gatedBody; known Content-Length pre-charges at gate entry. Oversize per-request bodies reject at enter, not by blocking the queue."
  - "Queued cancellation abandons the waiter without dispatching upstream; a grant racing a cancellation is dropped and handed to the next waiter — exactly-once release everywhere."
  - "Overload is explicit 429 (overloaded), distinct from budget_exceeded — rejected callers never hold a reservation."
  - "envAdapter marks operator-configured cleartext http:// upstreams as authorized-private, mirroring runtimeProviderConfig's AuthorizedPrivate marking, so env endpoint discovery works against private/local upstreams."

patterns-established:
  - "Permit-to-body ownership: an execution permit is released in accountingStream.finalize (EOF or Close), so early header return never frees an active slot and disconnect propagates through context cancellation to Close."
  - "Two-tier admission: HTTP gate bounds request/byte ingress; executor pools bound upstream dispatch capacity. Capacity rejection is observable and separate from budget rejection."
  - "Test observability: testdb.AppHandle captures child stdout/stderr into a locked buffer, dumped on failure; upstream stub reports observed concurrency/streamsSeen for barrier-driven assertions."

requirements-completed: [CON-01, CON-02, INF-05, JOB-03, JOB-04]

verification:
  - "go test -race -count=1 ./internal/inference ./internal/httpx — PASS (admission barrier tests: per-tenant sublimit, queued-cancel-leaves-no-leaks, abandoned body, manage-under-saturation, fan-out non-deadlock)"
  - "ENCLAVA_PROXY_INTEGRATION=1 go test ./tests -run TestProxyTwoProcess — PASS: mixed workload settles real units, held stream saturates appA (second queues, third rejects 429, manage stays live), appB admits its own stream, queued cancel never dispatches, disconnect marks unknown liability, shared budget race accepted=1 / budgetDenied=5 / overloaded=2 with total liability never exceeding the seeded limit, appB kill leaves reservation held, exact usage evidence in usage_records"
  - "go test -count=1 ./... — PASS (all packages; tests/contract exercised against the live composed app at :18444 with stub-chat/stub-embed models)"
  - "ENCLAVA_PROXY_INTEGRATION=1 unset => test fails (prerequisite failure, not skip)"

issues:
  - "Pre-existing bug fixed in-flight: NewService had dropped secrets: deps.Secrets so credential resolution always denied — restored."
  - "Pre-existing bug fixed in-flight: Execute's deferred attempt-timeout cancel killed live streaming bodies on return — streams now dispatch on the request context."
  - "Pre-existing bug fixed in-flight: Acquire fast path ignored the PerTenant sublimit when a slot was free — now checked."
  - "Pre-existing bug fixed in-flight: env provider adapter never authorized private/cleartext origins, so discovery on local upstreams failed — envAdapter mirrors endpoint authorization."
---
