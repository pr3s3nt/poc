---
id: VERIFY-2026-11-10-ROLLOUT-RECOVERY-PREREQUISITE
artifact: verification
status: current
last_reviewed: 2026-11-10
related: [UC-06]
---

# Recovery of a first Rollout interrupted after its Prerequisite — issue #48

## Scope

Branch `features/rollout-recovery-prerequisite` from `main` `1fb0bcd9`. The
read-only first-Rollout absence proof is on `main` since `3631ea05` (issue #49;
[contract](../architecture/contracts/delivery-reconciliation.md#first-rollout-absence-recovery--issues-49-48)).
Commit `696a368d` adds the missing evidence; the kind scenario is
[rollout-recovery-prerequisite](../../e2e/scenarios/rollout-recovery-prerequisite/README.md).

## Automated checks

- Fleet adapter: each remaining Rollout trace keeps the hold: manifest or metadata
  publication, live object, Bundle, BundleDeployment, Helm release, and a missing
  Prerequisite pin. Each case first proves the clean Rollout absent.
- Application recovery: only the Rollout is settled FAILED; the acknowledged
  Prerequisite keeps status, version, receipt and inventory; nothing is replayed;
  the Environment is released and a later user Deploy completes.
- Delivery and Fleet packages pass; documentation check passes.

## Kind run

An isolated Console `orch-issue48` was installed from `issue48-696a368d` and
seeded through the API. Application `prereq-issue48`, workload `payment` with
Service port 8080, was authored through the Console form.

| Step | Result |
|---|---|
| Deploy once; backend Pod force-deleted when the Prerequisite was ACKNOWLEDGED and the Rollout RUNNING without receipt | Operation INTERRUPTED, Environment held; Rollout unpublished |
| Recover from Settings with stopped-worker confirmation | RECOVERED; Rollout FAILED unpublished; Prerequisite intent unchanged (version 3, same receipt and controller UID); Service UID unchanged; no Deployment applied; Environment released |
| Deploy again | SUCCEEDED with the same Fleet controller UID and Service UID |
| Delete the Application | SUCCEEDED, namespace absent |

The recovery banner reads "Khôi phục đã hoàn tất"; the API outcome also asks to
preview before retrying, which the banner does not show.

## Cleanup

The Application was deleted through the Console. The isolated Console namespace
is retained for review until acceptance; the delivery Git repository is retained
per ADR-033.
