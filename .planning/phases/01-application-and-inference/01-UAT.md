---
status: testing
phase: 01-application-and-inference
source: [01-VERIFICATION.md]
started: 2026-10-19T18:55:00Z
updated: 2026-10-19T18:55:00Z
---

# Phase 01 UAT — Human Verification

One item from 01-VERIFICATION.md requires a human-run test against real infrastructure before this phase can be marked complete.

## Current Test

number: 1
name: Exercise a real confidential/attested inference endpoint end to end (INF-01, INF-05)
expected: |
  Configure a real confidential endpoint (operator-bound origin + credential reference + attestation, e.g. Tinfoil-style) in /dashboard/providers; verify assurance shows ACTIVE with a real attestation timestamp; make one chat completion and one Responses call through it; confirm settlement rows and attestation timestamps were recorded.
awaiting: user response

## Tests

### 1. Real confidential/attested inference endpoint end to end
expected: Configure a real confidential endpoint with operator-bound origin/credentials and attestation; verify ACTIVE assurance in provider administration, make chat and Responses calls, and confirm settlement plus attestation timestamps.
why_human: No confidential/attested upstream endpoint exists in this environment; protocol-equivalent plain-HTTP stubs prove transport and policy behavior but cannot prove a real provider attestation/assurance flow.
result: [pending]

## Summary

total: 1
passed: 0
issues: 0
pending: 1
skipped: 0
blocked: 0

## Gaps

None recorded beyond the pending test above.
