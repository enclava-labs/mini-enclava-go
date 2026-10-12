# OpenAI API Compatibility Audit

Audit date: 2026-06-25

Sources:

- OpenAI API Reference overview: https://developers.openai.com/api/reference/overview
- OpenAI OpenAPI spec path list: https://github.com/openai/openai-openapi/blob/master/openapi.yaml

## Implemented Gateway Shape

The Go gateway exposes OpenAI-compatible routes under both `/api/v1` and `/v1`.
Usage records are canonicalized to `/api/v1/...`, and endpoint allowlists treat
the two prefixes as equivalent.

Typed local handlers cover the routes where Enclava must enforce detailed local
behavior:

- `GET /models`
- `GET /models/{model_id}`
- `DELETE /models/{model_id}` through upstream passthrough
- `POST /chat/completions`
- non-`POST` `/chat/completions` routes through upstream passthrough
- `POST /embeddings`
- non-`POST` `/embeddings` routes through upstream passthrough when explicitly
  proxy-enabled
- documented Responses routes:
  - `POST /responses`
  - `GET /responses/{response_id}`
  - `DELETE /responses/{response_id}`
  - `GET /responses/{response_id}/input_items`
  - `POST /responses/{response_id}/cancel`
  - `POST /responses/input_tokens`
  - `POST /responses/compact`

Unknown or newer `/responses/...` subroutes fall back to generic passthrough
when the key has both `responses` and `openai.proxy`.

## Generic Passthrough Coverage

API keys with `openai.proxy` can proxy `GET`, `POST`, `PUT`, `PATCH`, and
`DELETE` for these OpenAI route families:

- `/assistants`
- `/audio`
- `/batches`
- `/chat/completions`
- `/chatkit`
- `/completions`
- `/containers`
- `/conversations`
- `/costs`
- `/embeddings`
- `/evals`
- `/files`
- `/fine_tuning`
- `/images`
- `/models`
- `/moderations`
- `/organization`
- `/projects`
- `/realtime`
- `/responses`
- `/skills`
- `/threads`
- `/uploads`
- `/usage`
- `/vector_stores`
- `/videos`

The current OpenAI API reference families are covered by typed handlers plus
those passthrough prefixes:

- Responses API: `/responses`, `/conversations`
- Platform APIs: `/audio`, `/videos`, `/images`, `/embeddings`,
  `/evals`, `/fine_tuning`, `/batches`, `/files`, `/uploads`, `/models`,
  `/moderations`
- Vector Stores: `/vector_stores`
- ChatKit: `/chatkit`
- Containers: `/containers`
- Skills: `/skills`
- Realtime: `/realtime`
- Administration: `/organization`, plus `/projects` compatibility aliases from
  the OpenAPI spec
- Chat Completions: `/chat/completions`
- Legacy APIs: `/assistants`, `/threads`, `/completions`

The API reference also documents webhook event schemas. The OpenAPI path list
does not expose a `/webhooks` REST route to proxy, so there is no gateway route
to add for that section.

## Compatibility Boundary

The gateway preserves methods, paths, query strings, JSON and multipart bodies,
binary responses, SSE responses, upstream status codes, selected upstream
response headers, `OpenAI-*` request headers, `Idempotency-Key`, and
`X-Client-Request-Id`.

Endpoint-specific semantics remain the upstream provider's responsibility for
generic passthrough routes. Enclava still applies API-key auth, scopes, endpoint
allowlists, source IP allowlists, model restrictions when a model can be
extracted from the request body, and budget checks before mutating passthrough
methods.
