---
id: VER-2026-09-28-UC03-UC16-RESOURCE-INPUTS
artifact: verification-evidence
status: current
last_reviewed: 2026-09-28
---

# UC-03/UC-16 resource inputs and matching — 2026-09-28

Local checks on the current checkout passed:

- `cd backend && go test ./... && go build ./...`, including new planner
  tests that use a non-seeded Application ID for both `internal-k8s` and
  `aws-eks`, and a Definition profile validation test.
- `cd frontend && npm run typecheck && npm run lint && npm test && npm run build`,
  including form tests for required PostgreSQL inputs, Score `params` output,
  and preserving params on edit.
- `python3 scripts/check_docs.py` and `git diff --check`.

No kind or AWS deployment was run for this change. The acceptance claim is
limited to local contract/UI/planner tests, not live PostgreSQL provisioning.
