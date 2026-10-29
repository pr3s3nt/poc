---
id: VERIFY-2026-10-28-WORKLOAD-SHARED-RESOURCES
artifact: verification-record
status: current
last_reviewed: 2026-10-29
---

# Default shared workload resources

Scope: [UC-16 BR-28–30](../usecase/UC-16/specification.md), new/existing
resource selection, generated IDs, advanced private scope and lossless existing
identities. HTTP/schema/planner scopes are unchanged.

## Local UI regression evidence

[Scenario](../../e2e/scenarios/workload-shared-resources/README.md) ran against
`backend/test/fixtures/deployment-review`: product UI/HTTP handlers with
in-memory storage and **fake execution adapters**. No Preview/Deploy, retained
database mutation, Kubernetes or AWS operation was performed.

Assertions passed: new shared ID with no manual input; worker uses API database
under a different local alias and locks configuration; saved edits preserve ID;
other Application and production cannot select staging resources; explicit
advanced private omits ID. No browser JavaScript errors.

VI/EN at 1512×980 and 1280×900, expanded/collapsed sidebar: no page overflow.
Sources inspected: AppShell, tokens/app styles, Button, Advanced, shared Dialog,
WorkloadEditorPage and existing dependency controls. Browser reference before
edits and final screenshots were inspected. Shell, cards and footer retain the
product layout. The saved-resource selector says Current resource rather than
Create new; redundant provisioning help was removed. The scope summary remains
visible when advanced is closed. These are product screens with synthetic data,
not evidence of real database provisioning.

Private artifacts outside Git: `/tmp/orch-shared-resource-review/before.png`,
`/tmp/orch-shared-resource-review/final/` (eight locale/viewport/sidebar screenshots
and `assertions.json`), and browser logs. Fixture password/session is not included
in the record or Git. Task-owned fixture process is stopped after checks; its
in-memory applications are discarded. No cluster cleanup is needed.

## Automated checks

- Frontend typecheck, lint and production build passed. Build retains the
  existing large-chunk advisory.
- Full frontend suite: `npm test -- --maxWorkers=2`, 80 files / 936 tests passed.
  The initial unconstrained run hit worker setup timeouts under host load;
  no assertion was weakened. Limiting concurrency resolved the timeouts.
- Focused workload suite: 7 files / 106 tests passed, covering new/inline reuse,
  locked params, private and imported identities, unavailable/conflicting
  projections, stale selection, generated IDs on internal HTTP and Save retry.
- After the final selector review, the three affected test files passed again:
  61 tests; the rebuilt Console browser scenario also passed all six assertions.
- Documentation checker, diff whitespace checks, changed runner syntax checks
  and PlantUML `-checkonly` passed.

The initial handoff only established local UI behavior. The user then required
real-data acceptance. The kind form runner now requires explicit selection of
both backend resources and compares their generated IDs/class/params before
Deploy; it cannot silently create replacements if reuse is missing. Go
product/schema code is unchanged, so no backend full suite was repeated.

## Real-data acceptance follow-up

The user requested real adapters and data after the initial UI handoff. The
owned-kind form/deletion harness runs the built Console/backend, Terraform,
PostgreSQL/Redis, Vault/VSO and the backend/worker/frontend acceptance app.
All application mutations use browser controls. Checks include five application
health checks, a database-backed job processed by the worker, recorded Redis
writes, valid update with retained database/cache/PVC UIDs and product deletion.

The first run `sr1028a` was interrupted before workload deployment completed.
The backend logged heartbeat I/O timeout at 2026-10-28 16:58:37 UTC, and later
restarted. The browser runner no longer existed when the user requested status
on 2026-10-29. PostgreSQL/Redis StatefulSets and PVCs existed, but no application
Deployment existed. This run is not a real-data PASS. Safe operation/resource
status and original runner logs were retained under the private run directory.
Ownership labels and the run manifest were checked before removing only that
run's Compose containers/volumes/network, workload Vault, kind cluster and image
tags; absence was verified. No retained installation was reset or modified.

Retry `sr1029b` uses a new owned stack after verified cleanup. Its evidence lives
outside Git in `/home/thanhnt1/.local/state/orch-shared-resource-live/`.


## Real kind PASS — 2026-10-29

Retry `sr1029b` completed successfully (browser 02:44:19–02:49:47 UTC,
cleanup exit 0). Product commit: `34fa602467ab61b91a4adce8357850304062f89f`;
runner assertions were strengthened in the follow-up commit. Application
`91057ead-78e3-4909-ac03-42999bdcad21`, staging, three workload Deployments,
exactly one PostgreSQL and one Redis StatefulSet with two Bound PVCs.

- PostgreSQL shared ID: `resource-e1fbf3c26f01fda91bd6baf85d40413a`.
- Redis shared ID: `resource-5a47596be0796c48bfaba39ad8d9dbb8`.
- Backend creates resources in the form without typing IDs; worker explicitly
  selects both existing backend resources. Desired ID/class/params match.
- UI Preview/Deploy succeeds for backend, worker and frontend. The acceptance
  app's backend connection/environment/secret/database/cache checks all PASS.
- API-created job 1 reaches DONE with worker identity. After the valid backend
  update, job 1 remains DONE and job 2 also reaches DONE. Redis evidence reports
  `workerWrites.ok=2`, `failed=0`, `unverified=0`, six cache hits and one refill,
  with no unrecovered failure. No intercepted responses or fake adapters.
- Database/Redis/PVC UIDs remain unchanged across the update; only the backend
  has an update in the second Preview/Deploy. Runtime ordinary env value is read
  from the Deployment and matches the saved update.

| Resource | UID retained across update |
|---|---|
| PostgreSQL `postgres-094abbd20db3b7f6` | `7b90b092-0f79-4a30-8948-82bd5d7a3d00` |
| Redis `redis-8f22a86a8eb31e3f` | `92c414a4-dea7-4286-8060-ec60b2cad507` |
| PostgreSQL PVC | `dd0309e9-29c5-4b93-9fdc-10e8d4fb52c1` |
| Redis PVC | `8bfa02ac-7ad9-477e-8ba7-823b7bb32a79` |

Whole-Application deletion through UI SUCCEEDED: all three owned Terraform
instances DESTROYED, namespace/PVs absent, tombstone/history retained, exact
Application Secret/delivery bundle paths return Vault 404. Name/subdomain reuse
gets a fresh Application ID; the empty replacement is also deleted through UI.
The cluster remains available during product cleanup. The harness then removes
only its owned cluster, Compose containers/volumes, workload Vault and image tags;
`cleanup.txt` ends `final_status=0`. Independent native reads confirm the owned
cluster/Compose containers/volumes absent. Other retained installations remain.

Evidence root: `/home/thanhnt1/.local/state/orch-shared-resource-live/run-sr1029b/`.
`form/result.json` holds 18 passed assertions and no browser errors;
`form/shared-resource-identities.json`, `form/objects.json`,
`form/checks-after-2-jobs.json`, `form/deletion.json`, `cleanup.txt` and
`secret-scan.txt` provide identity/data/absence evidence. Screenshots and local
videos remain outside Git. The final application screenshot was visually
inspected: all five checks PASS, two DONE jobs attributed to the same worker.
The evidence secret scan passes; no credential, state or browser session is
committed or uploaded. This verifies real local kind behavior, not AWS.

Follow-up checks: changed runner `node --check`, shared E2E helpers 6/6 tests,
documentation checker and `git diff --check` pass. No product code changed after
the previous frontend validation, so that full suite was not repeated.
