---
phase: 01-application-and-inference
reviewed: 2026-10-19T18:47:41Z
depth: standard
files_reviewed: 58
files_reviewed_list:
  - cmd/enclava/main.go
  - docs/go-rewrite-contract.md
  - docs/go-rewrite-route-matrix.md
  - go.mod
  - go.sum
  - internal/artifacts/reconcile.go
  - internal/artifacts/store.go
  - internal/artifacts/store_test.go
  - internal/auth/linking.go
  - internal/auth/oidc.go
  - internal/auth/oidc_test.go
  - internal/auth/principal.go
  - internal/auth/service.go
  - internal/auth/types.go
  - internal/chatbot/service.go
  - internal/chatbot/service_test.go
  - internal/httpx/app.go
  - internal/httpx/budgets.go
  - internal/httpx/budgets_test.go
  - internal/httpx/chatbot.go
  - internal/httpx/chatbot_test.go
  - internal/httpx/dashboard.go
  - internal/httpx/dashboard/templates/login.tmpl
  - internal/httpx/identities_test.go
  - internal/httpx/openai_contract_test.go
  - internal/httpx/platform_api.go
  - internal/httpx/responses.go
  - internal/httpx/responses_contract_test.go
  - internal/httpx/root_test.go
  - internal/httpx/sessionauth.go
  - internal/inference/policy.go
  - internal/inference/service.go
  - internal/inference/types.go
  - internal/inference/usage.go
  - internal/jobs/repository.go
  - internal/jobs/runner.go
  - internal/jobs/runner_test.go
  - internal/jobs/service.go
  - internal/jobs/service_test.go
  - internal/llm/model_cache.go
  - internal/llm/model_cache_test.go
  - internal/llm/openai_adapter.go
  - internal/llm/openai_adapter_test.go
  - internal/llm/prober.go
  - internal/llm/provider.go
  - internal/llm/tinfoil_provider.go
  - internal/llm/tinfoil_provider_test.go
  - internal/modules/extract/handlers_test.go
  - internal/store/postgres/artifact_repo.go
  - internal/store/postgres/artifact_repo_test.go
  - internal/store/postgres/identity_repo.go
  - internal/store/postgres/jobs_repo.go
  - internal/store/postgres/jobs_repo_test.go
  - internal/store/postgres/migrations/007_oidc_link_states.sql
  - internal/store/postgres/migrations/008_response_bindings.sql
  - internal/store/postgres/response_bindings_repo.go
  - internal/store/postgres/response_bindings_repo_test.go
  - tests/contract/openai_sdk_contract_test.go
findings:
  critical: 1
  warning: 0
  info: 0
  total: 1
status: issues_found
---
# Phase 01: Code Review Report

**Reviewed:** 2026-10-19T18:47:41Z
**Depth:** standard
**Files Reviewed:** 58
**Status:** issues_found

## Summary

The phase scope was reviewed against the project contract, with particular attention to the durable artifact finalization path. One durability failure can allow an artifact to be acknowledged as finalized without confirming its directory entry reached stable storage.

## Critical Issues

### CR-01: Directory sync errors are ignored during artifact finalization

**File:** `internal/artifacts/store.go:302`
**Issue:** `Finalize` calls `fsyncDir(dir)` after renaming the temp file, but that helper (lines 467-471) silently ignores failure to open or sync the directory. Finalize then commits the database row as ready. If directory synchronization fails, the rename's directory entry is not confirmed durable; a crash can therefore leave a committed ready row whose bytes are missing, violating the artifact contract that durable bytes exist before ready metadata is acknowledged and risking data loss.
**Fix:** Make `fsyncDir` return its open/sync error and abort finalization before `FinalizeCommit` when it fails. Keep the failure path consistent with reconciliation so a renamed file with a still-writing row is safely cleaned up or retried.

---

_Reviewed: 2026-10-19T18:47:41Z_
_Reviewer: the agent (gsd-code-reviewer)_
_Depth: standard_
