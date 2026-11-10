---
id: VERIFY-2026-11-11-ENVIRONMENT-BUNDLE
artifact: verification
status: evidence
last_reviewed: 2026-11-11
related: [ADR-042, UC-12, UC-06, UC-18]
---

# One Fleet bundle per Environment, versioned secrets — issue #67

## Scope

Branch `features/refactor_fleet`. Decision:
[ADR-042](../architecture/decisions/ADR-042-environment-bundle-versioned-secrets.md).
Scenario: [environment-bundle](../../e2e/scenarios/environment-bundle/README.md).

## Automated checks

- Unit tests added: Vault versioned path (reuse equal version, CAS write,
  deleted version not reused, workload cleanup); fixed-identity VSO rendering;
  coordinator Environment units (merge, retained secret of the last successful
  release, Job naming and ordering, secret preparation, failing-workload
  attribution, pruning objects of failed revisions, preparation not blocked by
  a published broken workload and never kept as a release, rollback keeping the
  migrate unit); Fleet not-ready object parsing; migration template rendering;
  Console store lock (Developer read-only, `SECRET_STORE_LOCKED`).
- Packages `vault`, `kubernetes`, `delivery` (application and Fleet adapter),
  `templatedriver`, `gitpublication`, HTTP, `appdeletion` pass. `deployment`,
  `pending` and `configuration` fail exactly the same tests as unmodified `main`
  in this environment (render bundle/kubectl stub baseline); no new failure.
- Frontend typecheck, lint (existing PlatformSettingsPage warning), 1009 tests
  and build pass. Documentation checker passes.

## Kind run

A Console built from the branch ran in its own namespace on `kind-k8s-4f`, seeded
with `seed-api.mjs` (Git, cluster account, Vault store, Fleet CD) and
`seed-definitions.mjs` (cluster, namespace, PostgreSQL, Redis, storage, and a
workload Definition whose template source is this branch's standard template).
Every business action went through the Console as Admin or Developer; workloads
were authored in the form (no file import). See the run table in the issue
report for operation IDs and timings.

Final clean run `bundle3` (2026-11-10 19:03–19:42 UTC) on the last build:

| # | Criterion | Result |
|---|---|---|
| 1 | First Deploy of four workloads | SUCCEEDED in one Deploy; `<workload>-config-v1` at Vault version 1; app checks PASS |
| 2 | Image only | same VSS/Secret/ServiceAccount/VaultAuth UIDs and Vault version |
| 3 | Secret change | Vault version 2, `-config-v2`, Pods rolled, v1 retained |
| 4 | Rollout failure after a secret change | Old worker Pod kept `worker-config-v2`; deleting it (UID precondition) brought a Ready replacement on v2 |
| 5 | Unused secret | Rollback release left only `-config-v2`; every other backend/worker VSS and Secret deleted and garbage collected |
| 6 | Job | `backend-migrate-<hash>` Complete before the backend Deployment existed; forced Fleet resync re-ran no Job |
| 7 | One broken workload | Deploy PARTIAL: Console named worker (`ROLLOUT_NOT_READY`), backend SUCCEEDED, frontend Pods unchanged, jobs still processed |
| 8 | Rollback | Restored `-config-v2` reusing the Vault version (no new write) |
| 9 | Acceptance app | frontend/backend/worker/scheduled, PostgreSQL, Redis, 1Gi PVC: checks PASS, job DONE, PVC value kept after Pod recreation, scheduled receipt with the PVC hash |
| 10 | Deletion | `bundle2`: schedule suspended through Preview/Deploy, Application deleted; namespace, Fleet GitRepo and workload Vault paths gone |

During `bundle3` the first `fail` attempt hit a deadline-cut Git read (fixed,
item 3 below); the Environment was recovered from Settings and the stage rerun.
The delivery Git repository history is retained per ADR-033.

## Defects found and fixed during the run

1. A migration Job needs Secrets the app unit had not published yet: secret
   preparation before `migrate`.
2. Fleet's aggregate `ErrApplied` carried no objects: failing workload parsed
   from Fleet's message; a failed Helm apply keeps the old applied deployment,
   so the deadline concludes it.
3. A read cut off by the readiness deadline held the Environment instead of
   failing it (Deploy and recovery).
4. Read-only Git commands are retried on any failure except rejected
   credentials/certificates.
5. Objects created only by failed revisions leaked: a release sweep after Ready.
6. A secret preparation counted as the last successful release.
7. Rollback refused because of the migrate unit: rollback keeps it unchanged.

## Findings outside this scope

- An Application whose deletion stopped with an interrupted operation cannot be
  recovered (Settings refuses changes while deleting) nor retried (the deletion
  refuses an Environment with another operation). `bundle1`, which went through
  intermediate builds and a removed Fleet Role, is left in that state.
