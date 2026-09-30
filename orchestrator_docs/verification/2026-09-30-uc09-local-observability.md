---
id: VER-2026-09-30-UC09-LOCAL-OBSERVABILITY
artifact: verification-evidence
status: evidence
last_reviewed: 2026-09-30
---

# UC-09 local observability — 2026-09-30

I06-04 was implemented by Claude in the project tmux session, reviewed by the
coordinator, corrected after review and independently validated on this checkout.

## Checks

- `cd backend && go test ./... && go build ./... && go vet ./...` passed.
- With `ORCHESTRATOR_POSTGRES_TEST_URL` pointing to a disposable localhost
  PostgreSQL 17 container, `go test -race -count=1 ./...`, build and vet passed.
  PostgreSQL cases ran rather than skipping. Each new adapter/HTTP contract
  test creates and drops a dedicated database.
- `cd frontend && npm run typecheck && npm run lint && npm test && npm run build`
  passed: 11 Vitest files, 36 tests.
- Changed verification scripts passed `bash -n`. Documentation/PlantUML
  validation and `git diff --check` passed after reconciliation.

## Browser verification

`bash backend/test/integration/uc09-playwright-local.sh` uses fake runtime
adapters and Chromium against the built Web Console. Independent runs passed:

| Persistence | Run ID | Result |
|---|---|---|
| Temporary JSON state | `uc09-20260930041545-23282` | Create and restart phases passed. |
| Disposable PostgreSQL | `uc09-20260930041953-29576` | Create and restart phases passed. |

The PostgreSQL run supplies `ORCH_UC09_DATABASE_URL` for an empty local test
database; the runner writes the URL to a mode-0600 file rather than an argument.

The flow signs in, deploys `backend`, records a planning failure, then redeploys
`backend` with a different manifest. It checks history, server-side failed-status
filtering, planning-failure detail without fabricated plan/workloads, resource
redaction and each successful Deployment's own manifest digest. The old workload
snapshot remains equal after redeploy and backend restart. Signing out makes
history return `401`.

## Regression coverage

- Session and Organization/Application/Environment isolation; invalid status;
  missing or malformed Deployment identifier returns `404`, including PostgreSQL.
- Consistent read-snapshot assembly without mutation/runtime calls; storage
  outages remain retryable errors rather than fake not-found responses.
- Unknown output metadata and nested secret references are redacted;
  resolved resource inputs are absent from serialized responses.
- Atomic current-workload/snapshot progress; terminal immutability and
  serialization with a racing terminal transition; defensive map copying.
- JSON reload, migration 4 replay and latest-only legacy backfill. Older runs
  without recorded workload history remain empty, not reconstructed.
- Existing configured-run-ID coverage remains, now inspecting repository data
  instead of exposing resource inputs through the read API.

Tests also exposed two existing defects corrected in this change: PostgreSQL's
ambiguous `version` column in the final Environment update, and memory store's
shared transaction flag allowing another goroutine to bypass its mutex. The
replacement transaction marker is context-scoped.

## Cleanup and limits

The coding agent's `uc09-pg-test` and independent review's
`uc09-review-20260930-0418` containers were stopped and auto-removed. Successful
browser runs removed their temporary state and stopped their backend processes.
Existing Docker containers, kind PostgreSQL 16, Kubernetes and AWS were untouched.

kind/AWS/Backstage scripts were updated for scoped authenticated reads but only
syntax-checked; no cluster/cloud execution or rollout occurred. Comparison,
streaming/live runtime refresh and production identity remain outside UC-09.
Memory read snapshots clone the full state and are intended for local/test use.
