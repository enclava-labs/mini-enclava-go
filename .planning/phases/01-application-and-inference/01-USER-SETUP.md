# Phase 01: User Setup Required

**Generated:** 2026-10-19
**Phase:** 01-application-and-inference (plan 01-02)
**Status:** Incomplete

Complete these items for the transport layer to reach a real endpoint. The executor automated all code changes; these items require human access to endpoint infrastructure and credentials.

## Environment Variables

| Status | Variable | Source | Add to |
|--------|----------|--------|--------|
| [ ] | `OPENAI_COMPATIBLE_BASE_URL` / `PRIVATEMODE_BASE_URL` | Authorized customer-local OpenAI-compatible endpoint URL (operator-approved origin) | runtime env |
| [ ] | `OPENAI_COMPATIBLE_API_KEY` / `UPSTREAM_OPENAI_API_KEY` | Endpoint credentials for the approved local endpoint | runtime env |
| [ ] | `TINFOIL_API_KEY` | Tinfoil dashboard → API keys | runtime env |
| [ ] | `TINFOIL_ENCLAVE` | Tinfoil dashboard → enclave hostname | runtime env |
| [ ] | `TINFOIL_REPO` | Tinfoil dashboard → repo for attestation binding | runtime env |

## Notes

- Private/cleartext base URLs are refused by the adapter until the endpoint is explicitly approved (`WithAllowPrivateEndpoint`); wiring endpoint approval to `main.go` belongs to the inference executor plan. Metadata/link-local addresses are refused even when approved.
- A required Tinfoil verification failure denies dispatch; there is no downgrade to ordinary HTTP.

## Verification

After configuring a real endpoint, verify with:

```bash
go test -count=1 ./internal/llm
INFERENCE_PROVIDER=openai_compatible OPENAI_COMPATIBLE_BASE_URL=<approved> \
  OPENAI_COMPATIBLE_API_KEY=<key> go run ./cmd/enclava
```

Expected results:
- `GET /v1/models` proxied to the approved endpoint returns real model IDs.
- Tinfoil: attestation verified status reports real enclave/repo/digest.
