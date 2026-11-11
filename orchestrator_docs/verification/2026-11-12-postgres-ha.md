---
id: VERIFY-2026-11-12-POSTGRES-HA
artifact: verification-record
status: current
last_reviewed: 2026-11-12
---

# PostgreSQL HA — issue #75, acceptance evidence

Task branch `features/postgres-ha` in both repositories. No merge/tag or
Definition release promotion. Scope follows
[ADR-043](../architecture/decisions/ADR-043-postgres-ha-cnpg.md) and
[CNPG runbook](../operations/postgres-ha-cnpg.md).

## Completed checks

- Product docs commit `9908cb28`; backend initial commit `dba4503c`.
- Release module commit `129da42d3a562cc7bd0534380c9cac8b8265468b`:
  Terraform 1.9.8 fmt -check, validate and test: five runs pass. Mock provider
  covers all eight outputs, connection encoding, sensitivity, instance bounds
  and configured storage. No production infrastructure is proved by these tests.
- Focused Go tests runner/readiness/tfexec pass using the Docker Go toolchain.
- CNPG 1.30.1 release manifest installed server-side on authorized kind context;
  operator became 1/1 Ready, CRD present. No existing workload was modified.
- Direct Terraform module run in task namespace created two instances and two
  bound 1Gi PVCs. Client using auth Secret connected via both host and uri;
  wrote/read a row and confirmed replication to standby before failure.
- Deleted primary Pod: standby promoted and client wrote/read through the rw
  Service within **5.09 seconds**. Original row survived; two rows present.
- Reapply: no changes, same Cluster UID and both rows preserved.
- Browser signed in through the actual Tailscale Console route, read live
  resource metadata, and inspected the exact release module commit successfully.
  Private origin, screenshot and response evidence stay outside Git.

## Product acceptance on the feature stack

User approved the shared Type extension on 2026-11-12. Console UI added
`asyncUri` as optional string/secret, preserving previous outputs. HA Definition
`staging-k8s-4f-postgres-ha` was saved with all eight mappings on the shared
Console and the task stack. Ref is `721b154d95e461cbc6431bdcd911f555874b3733`;
module content is unchanged from `129da42`. The issue's example source
Definition is absent on this installation; `git-dev-postgres` is its existing
staging/in-cluster equivalent, read but not modified.

A dedicated Docker stack uses backend commit `ab051509`, unchanged frontend
from base `b1445707`, real PostgreSQL/Vault, scoped cluster credentials, shared
Vault/Fleet/Git and CNPG. No fake adapter or seeded workload replaced browser
actions. Setup reused ordinary API bootstrap with task-owned catalog/accounts;
Application, workload, resources, store selection, Preview/Deploy and deletion
use Console UI. [Runner](../../e2e/scenarios/postgres-ha/README.md).

- One workload `client` declares both `postgres` class `ha` and `in-cluster`.
  Both were saved before one Environment Preview/Deploy. Preview selected HA
  Definition; the exact review-scoped operation succeeded.
- HA Cluster had two Ready instances. Client used host and URI outputs to
  read/write; in-cluster database connection/write also passed.
- After standby replication was checked, deleting the primary Pod promoted
  standby and restored client reads/writes in **5.369 seconds**. Original row
  survived and the second row was written through the rw Service.
- Saving identical configuration produced empty Preview and disabled Deploy,
  as designed by the existing Console. No disabled mutation was bypassed.
  A real second Deploy changed only `REDEPLOY_PROBE` on the client, leaving
  both databases' inputs unchanged; HA Cluster UID and both databases' rows
  remained unchanged. Direct Terraform no-op apply above tests actual empty
  apply. The first runner assumed an empty Preview would contain a client
  change; that assertion was corrected, and its failed attempt was retained.

## Limits and pending gates


Provider 2.38.0 strips server-side UID from manifest object state. See
[IMP-043](../implementation/deviations.md#imp-043--cnpg-cluster-uid-absent-from-terraform-manifest-state).
Guard accepts only state-proven UID chains; no bypass or inferred ownership.
Direct module child ownerReferences match the real Cluster UID, but that does
not supply the missing historical state proof.

Full frontend checks in isolated kind namespace passed: typecheck, lint,
complete Vitest suite (107 files / 1,027 tests) and build. The first backend
run was superseded after final GVK/absence fixes; its 10-minute per-package
limit was below the documented main baseline duration. Final source was checked
with `go test -p 2 -parallel 4 -timeout 20m ./...`, real PostgreSQL 16 and
`REQUIRE_POSTGRES=1`: 13 packages fail, without timeout. Changed packages
readiness/runner/tfexec pass. `go build ./...` passed separately.
A pristine `origin/main` (`b1445707`) run in the same environment is in progress
to compare exact failures. This is not a passing full-suite claim.
Direct module Terraform destroy removed both managed resources; authoritative
queries confirmed Cluster/Pod/PVC/Secret/Service absence, namespace absence and
zero remaining PVs referring to that namespace. This proves module teardown,
not product cleanup. Operator is retained while acceptance remains pending.
Full-test namespace cleanup and backend results must be added before closing
this record. No ready-for-acceptance claim is made.

## Product cleanup and retained access

Application deletion destroyed HA Cluster, single-instance postgres and
namespace through the existing resource cleanup flow. Its first attempt then
failed `SECRET_PURGE_FAILED` because the task-owned Vault port-forward lost its
Pod sandbox after a shared Vault Pod replacement. Infrastructure results were
already DESTROYED; no guard was bypassed. Restoring the forward and clicking
Retry on Console completed the same deletion at attempt 2 (SUCCEEDED).
Authoritative queries confirm namespace, CNPG Cluster, Pods, PVCs, Services and
all PVs claiming the run namespace are absent. The recorded namespace/Cluster
UIDs and original cleanup inventory were preserved across the retry.

The dedicated feature stack, its volumes/catalog and CNPG operator are retained
for review. Trial Application/workloads and their cluster resources were removed.
The Console's postgres catalog route was opened through the actual Tailscale
feature hostname; final verification checks its loaded list and saved mappings.
Private runtime scripts/config/evidence/passwords remain outside Git. The shared
Console additionally retains the approved optional Type output and task HA
Definition. Existing in-cluster Definition and unrelated workloads were not
modified. No merge, tag v0.6.0 or Definition tag promotion has occurred.
