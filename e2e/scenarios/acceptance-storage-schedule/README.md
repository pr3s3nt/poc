# Acceptance PVC and scheduled receipt observations

Verifies the deployed acceptance app's actual file value/hash and a **new**
scheduled invocation receipt through browser UI. Uses the acceptance workload's
page, not the Orchestrator Console; no Console login helper is needed here.
Canonical procedure: [PVC/CronJob runbook](../../../orchestrator_docs/operations/acceptance-storage-schedule.md),
[WC requirements](../../../orchestrator_docs/usecase/UC-16/workload-catalog-specification.md).

Prerequisites: Node 22.15+/24+, `cd e2e && npm ci && npx playwright install chromium`,
deployed new acceptance frontend/backend, a managed volume mounted on backend,
PostgreSQL and genuine scheduled workload as configured in the runbook. Backend
and scheduled must share `ACCEPTANCE_SCHEDULE_RUN` and PostgreSQL binding.
URL is the acceptance frontend (or its existing port-forward), **not** Console.
No fake backend response or synthetic receipt is used. The runtime must be
owned/authorized for testing; the script does not grant deployment rights.

From repo root, write a public test value through the visible form:

```bash
node e2e/scenarios/acceptance-storage-schedule/run.mjs \
  --url http://localhost:18481 --run-id test-20261029 \
  --value retained-test-20261029 --write --delay-ms 1000
```

Default browser is visible; use `--headless` for automation. `--delay-ms` is
0..10000 (default 800); `--wait-ms` 1000..900000 (default 180000). No password or
secret belongs in `--value`. The value is 1..4096 UTF-8 bytes.

After an independently authorized, UID-verified Pod recreation or image update,
run the same command **without `--write`**. It must read the old value before
waiting for a new receipt; it cannot silently overwrite missing data to pass.
This proves the app observations, not the PVC UID or Kubernetes CronJob owner.
The operator/Console E2E must separately record PVC UID, Pod owner/UID, actual
CronJob → Job UID ownership/completion, suspend/remove and cleanup assertions.

Assertions: run scope matches, file value/hash matches, and a previously unseen
invocation with the same hash appears in the latest 20 receipts. Existing
receipts cannot satisfy the wait. Failure is exit 1; no automatic repair/retry
mutation. Only explicit `--write` overwrites the fixed app test file. No saved
Console draft, cluster mutation, DB reset, Job trigger, resource creation or
cleanup is performed. Browser closes in `finally`; no session/trace/video is
saved. File and receipts are intentionally retained for follow-up checks.

Local checks of Go components are documented in the acceptance app README.
Passing local tests is not evidence of a deployed PVC/CronJob lifecycle.
