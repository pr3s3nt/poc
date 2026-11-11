---
id: VERIFY-2026-11-11-STUCK-STATEFULSET
artifact: verification
status: evidence
last_reviewed: 2026-11-11
related: [UC-03, UC-06, UC-08]
---

# Stuck StatefulSet recovery and Active Definition revision — issue #68

## Scope

Branch `features/statefulset-stuck-pod-recovery` (contains #67 from
`features/refactor_fleet`). Rules: [UC-08 BR-11](../usecase/UC-08/specification.md),
[UC-06 CD-11](../usecase/UC-06/specification.md),
[UC-03 DV-01/04/06](../usecase/UC-03/definition-versions.md).
Scenario: [stuck-statefulset-recovery](../../e2e/scenarios/stuck-statefulset-recovery/README.md).

## Automated checks

- New unit tests: stuck Pod evaluation (11 cases, safe reasons); readiness with a
  fake controller (fixed revision replaces and becomes Ready, no new revision only
  reports, missing delete permission only reports, deadline-cut query keeps the
  reason); Fleet recovery with a fake kubectl (replace, no fix/foreign revision,
  missing permission, failed Pod query keeps earlier reasons); coordinator
  recovery while waiting; operation `Annotate` survives `Stage`; operations
  `podWaits` projection; `DATA_MIGRATION_REQUIRED` classification; unselected
  Environment follows Active, succeeded Environment stays pinned; Preview newer
  revision notice; Console Pod wait panel and Preview notice.
- Real PostgreSQL definition-version integration tests pass.
- `catalog` (2), `pending` (9), `preview` (10), `deployment` (11) and
  `bootstrap` (2) fail exactly the same tests as `features/refactor_fleet` in
  this environment; no new failure. Frontend typecheck, lint (existing warning),
  tests and build pass. Documentation checker passes.

## Kind run

A Console built from the branch ran in namespace `orch-issue68` on `kind-k8s-4f`,
seeded with `seed-api.mjs` (Git, cluster account, Vault store, Fleet CD) and
`seed-definitions.mjs`. Developer actions went through the Console; PE Definition
edits/promotion used the Admin session's Console API as setup.

| Stage | Issue scenario | Result |
|---|---|---|
| `pg-bad` | 1 | Wrong PostgreSQL digest promoted (r2); Deploy FAILED after the readiness deadline; Console showed `ImagePullBackOff` with the image; no Pod replaced |
| `pg-fix` | 1 | Fixed image promoted (r3); never-deployed app resolved r3 without selection; Deploy SUCCEEDED in 66 s; Console showed the replaced `postgres-…-0`; PVC identity unchanged; Pod Ready on the fixed digest |
| `pinned` | 4 | r4 promoted after success: app kept r3; Settings showed "Có bản mới"; Preview named r4 as the newer default |
| `guard` | 5 | Selecting r4 in Settings: Preview refused with 409 `DATA_MIGRATION_REQUIRED` and the migration message; selection discarded |
| `sts-bad` | 2 | StatefulSet workload with a missing image through Fleet: Deploy FAILED; Console showed `ImagePullBackOff` with the image |
| `sts-same` | 3 | Fleet path offers no unchanged redeploy after a failed release (the published revision is kept, ADR-042) |
| `sts-fix` | 2 | Image fixed in the form: Deploy SUCCEEDED; `store-0` replaced and running the fixed image; wait list cleared |
| `pg-same` | 3 | With r4 (wrong digest) as default, a new app failed; deploying again unchanged failed with `NOT_READY` and the reason, no Pod replaced, no replacement shown |

After the run the Definition default was promoted back to r3 (correct image).
`cleanup` deleted three test Applications through the Console. The fourth
(`pg-same`, whose PostgreSQL never became Ready) stays `DELETING`: cleanup
refuses it because no `UNREFERENCED` root exists for a never-ready resource.
That rule is unchanged by this branch and by #67; it is reported separately.

## Defects found and fixed during the run

1. After success the wait list kept the last stuck entry: a ready poll now
   records an empty list.
2. A readiness query cut off by the deadline turned an observed not-ready result
   into `READINESS_UNAVAILABLE` and cleared the reason: incomplete polls keep the
   previous reasons and the deadline concludes `NOT_READY` with them.
3. A running database refusal showed the generic invalid-request text: it now has
   the `DATA_MIGRATION_REQUIRED` code and message.
