---
id: VERIFY-ACCEPTANCE-PVC-CRON-LOCAL-20261029
artifact: verification-record
status: current
last_reviewed: 2026-10-29
---

# Acceptance workload storage and schedule support: local verification

User clarified PVC (persistent volume), not AWS VPC, and approved extending the
acceptance app for PVC/CronJob testing. Implemented scope is described in the
[runbook](../operations/acceptance-storage-schedule.md). No Kubernetes workload
deployment was requested or performed in this change.

## Implemented and checked

- Backend's opt-in fixed file stores a bounded public test value with atomic
  write/rename and SHA-256. Configured-but-missing/unwritable storage returns an
  error; no directory is auto-created. Reads reject paths escaping the configured
  root. Requests cannot supply arbitrary file paths.
- One-shot Go scheduled binary reads the actual backend value, verifies the
  hash and commits a scoped receipt to the same workload PostgreSQL database.
  Retry of run/invocation/hash is idempotent; conflicting hash fails. It exits
  after one invocation and does not alter the worker queue/cache counters.
- Backend's existing acceptance-local migration creates the separate receipt
  table without resetting jobs. SQL reopen/repeated-migration checks preserve
  old jobs and receipts. Orchestrator schema/migrations are unchanged.
- Acceptance page retains its existing plain HTML checks/job form and adds
  storage form/hash and recent receipts. Browser reference used the prior
  frontend source built through an external Go overlay; original and updated
  pages were opened/captured against real local backend services. Long hashes
  wrap inside the existing page width and timestamps use UTC. Review retained
  only the test form, observations and one refresh link; no extra help panel.
- The new Playwright observation scenario writes only with explicit `--write`.
  After a restart it reads the old expected value first, then requires a new
  invocation receipt. It cannot regenerate missing data or accept an old receipt.

## Real local services and browser

Private evidence is under `/tmp/acceptance-storage-schedule-local-20261029`,
outside Git. Owned disposable PostgreSQL/Redis containers and native Go processes
ran on loopback only. No existing Console, acceptance Application, database,
namespace, kubeconfig or Kubernetes resources were modified.

Local browser checks passed: empty file → visible form write → matching hash →
scheduled receipt; existing job submits and completes through PostgreSQL/worker/
Redis, with all five deployment checks PASS. Backend was stopped and relaunched
with a new HOSTNAME and the same owned local directory; old value/hash remained.
The executable E2E observation scenario passed without `--write`, waiting for a
new receipt after restart. The scheduled binary was actually executed by a local
supervisor every two seconds; **this is not a Kubernetes CronJob**. The directory
is a real local filesystem directory, **not a Kubernetes PVC**. Actual PVC UID,
CronJob/Job owner UID, scheduling, suspend and cleanup acceptance remain to run
under the separate cluster procedure.

Two initial smoke setup attempts failed before app execution because the Redis
image entrypoint dropped privileges and could not read its owner-only test
configuration. The owned container was cleaned after each attempt. The final
local fixture invokes redis-server directly as root solely to read its private
configuration. Two subsequent browser smoke runs passed, including the final
UI wrapping change. Owned smoke containers and app processes were verified
absent after cleanup. Credentials stayed private; no raw secret bytes in evidence.

## Validation

- Focused acceptance suite against fresh real PostgreSQL and Redis:
  `REQUIRE_POSTGRES=1 REQUIRE_REDIS=1 go test -count=1 -json ./examples/acceptance-app/...`:
  97 test/subtest passes, zero failures and zero skips.
- Full backend `go test -p 2 ./...` with its own real PostgreSQL and required
  Redis was attempted: acceptance packages passed, but the overall run failed.
  `TestUpgradeBackupRestore` lacked client-tool/container configuration and the
  unchanged tfexec package exceeded Go's default 10-minute package timeout.
  This is not recorded as a full-suite pass; tests after the tfexec timeout
  may not have executed.
- Focused recheck of `TestUpgradeBackupRestore` with
  `ORCHESTRATOR_POSTGRES_TEST_CONTAINER` and
  `TestDestroyedRecordFailureHoldsTheEnvironment` (the test active at timeout)
  passed against a new disposable PostgreSQL: three test/subtest passes,
  zero failures/skips. The latter completed in 9.58 seconds in isolation.
  Both full-run and recheck PostgreSQL containers were verified absent after
  cleanup. No unrelated production/test code was changed to address these
  validation-environment limitations.
- Backend `go build ./...` passed.
- Four static scratch images built with tag `pvc-cron-20261029`:
  acceptance-frontend/backend/worker/scheduled. Build helper now works from the
  repository root as well as backend by selecting the Go module explicitly.
- Script syntax/help, `bash -n`, documentation checker and diff whitespace checks
  pass. The React Console did not change, so no npm product suite was required.

No deploy or image load/push to Kubernetes/registry occurred. Images and source
are ready for the authorized runtime phase; the existing Tailscale Console is
unchanged. Local evidence does not certify live persistence/scheduling behavior.
