---
id: RUNBOOK-ACCEPTANCE-STORAGE-SCHEDULE
artifact: operations-runbook
status: current
last_reviewed: 2026-10-29
---

# Acceptance application: PVC and CronJob

This workload extension verifies [UC-16 WC-04/05/11/12](../usecase/UC-16/workload-catalog-specification.md)
using the existing acceptance application. It does not change Orchestrator
schema or product lifecycle requirements. Existing PostgreSQL/Redis/job checks
remain independent and required.

## Intended acceptance flow

Backend opts in with `ACCEPTANCE_STORAGE_DIR=/data` mounted from a managed PVC.
Without this setting the extra panel reports disabled and old app behavior stays
available. A fixed application-owned file stores one bounded test value; the API
does not accept file paths. Writes use temporary file, sync and atomic rename.
An empty directory means no value written; never claim PVC persistence from a
successful write/read alone. The app does not inspect Kubernetes or claim UID.

The scheduled workload is a separate Go binary/image. Each invocation reads the
backend's stored value/hash through its Service, writes one completed receipt in
PostgreSQL and exits 0 only after commit. No Redis dependency or polling loop.
Retries with the same `HOSTNAME` within an `ACCEPTANCE_SCHEDULE_RUN` scope reuse
the receipt; a different hash for that identity fails. The binary never submits
jobs to the asynchronous worker queue. Database credentials use the existing
shared PostgreSQL bindings. Backend's app-local migration creates the receipt
table; scheduled starts only after the backend is ready and a value is written.

The acceptance frontend shows a bounded file form, observed hash and last 20
receipts for the configured run. API errors are visible, not empty success.
Receipts establish that a scheduled process read the backend file and committed
to PostgreSQL; Console live Job history and UID reads separately establish actual
CronJob ownership/completion. No guessed next-run or synthetic success.

## Runtime procedure

Build acceptance images with the existing build script; scheduled is the fourth
binary. Register platform through [setup](../../scripts/bootstrap/README.md).
For example, from the repository root:

```bash
bash backend/test/integration/build-images.sh <tag>
```

Images must be available to the selected cluster before deployment. For an
explicitly authorized local kind runtime, load all four images, including
`acceptance-scheduled:<tag>`, into that exact cluster. For other clusters, publish
to the intended registry and use its image prefix. Building on the host alone
does not make images available on Kubernetes nodes.

Deploy the existing frontend/backend/worker app using form authoring; add storage
to Backend with Add volume mount, select container main and `/data`, bind the
same shared PostgreSQL/Redis as before. Set the storage directory and a unique
`ACCEPTANCE_SCHEDULE_RUN` on backend. Open the acceptance page and write a unique
non-secret test value. Verify hash/value through the page.

Create an independent Scheduled workload using `acceptance-scheduled:<tag>`:
`BACKEND_URL` references backend Service, `ACCEPTANCE_SCHEDULE_RUN` matches backend,
PGHOST/PGPORT/PGDATABASE/PGUSER/PGPASSWORD reference the same shared database.
Start with schedule `* * * * *`, timezone UTC, concurrency Forbid. Preview/Deploy
through Console. Observe a real Job completion and a matching receipt in the app.

Record run-owned Application/namespace, Pod owner UID and PVC UID before a Pod
recreation; delete only the verified owned Pod with UID precondition. Confirm
backend returns the same file hash/value, PVC UID unchanged, and a later receipt
appears. Repeat a valid image/config update. Suspend via Preview/Deploy and wait
for active Jobs to finish before removal. Remove compute and verify storage
retention; explicitly delete data through the product only when cleanup is in
scope. Preserve preexisting workloads/volumes. No cluster/cloud run is implied by
building the images or by this runbook.

The executable [observation scenario](../../e2e/scenarios/acceptance-storage-schedule/README.md)
writes/reads through the deployed acceptance app and waits for a genuine receipt.
It does not create or delete Kubernetes resources or prove restart retention by
itself. Runtime and cleanup require separate authorization and ownership evidence.

## Application-local data/API

`POST /api/storage` accepts `{value}` (1..4096 UTF-8 bytes, nonblank);
`GET /api/storage` returns configured/present, value, SHA-256 and process identity.
Only public test data belongs here, never credentials. Errors contain no paths or
raw filesystem/provider errors. No configured directory is created automatically.

`GET /api/scheduled-runs` lists the last 20 completed receipts in backend's run
scope. The app-local `acceptance_scheduled_runs` table has run_id/invocation
composite primary key, value_sha256, completed_at; same-key retries are idempotent.
This table belongs to the acceptance workload database, not Orchestrator's
versioned database baseline. No existing jobs table or historical rows are reset.
