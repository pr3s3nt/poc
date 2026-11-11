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
- HCL AST comparison confirms exact existing variable types/defaults/sensitivity
  except the approved CNPG image default; only instances=2 is added. All eight
  output formulas/sensitivity match the original module. The original
  terraform/postgres tree is unchanged.
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
Console and the task stack. Runtime used ref
`721b154d95e461cbc6431bdcd911f555874b3733`; after a README clarification about
unreleased commit references, both Definitions were updated through Console
CAS to `c7288077b6a81e0ad1d0504629fdd5d6dc37ed81`. The entire module directory
and inspected content digest are unchanged from `129da42`/`721b154`; eight
mappings, variables, provider binding and criteria were checked unchanged. The issue's example source
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

## Limits and full-check baseline

Provider 2.38.0 strips server-side UID from manifest object state. See
[IMP-043](../implementation/deviations.md#imp-043--cnpg-cluster-uid-absent-from-terraform-manifest-state).
Guard accepts only state-proven UID chains; no bypass or inferred ownership.
Direct module child ownerReferences match the real Cluster UID, but that does
not supply the missing historical state proof. An additional read-only probe
using the task backend Checker and the actual pre-destroy module state passed
real Inspect/VerifyAbsent after cleanup: exact namespace, CNPG Cluster and
Secret queries returned authoritative NotFound, including the manifest with no
ID/UID. The temporary probe binary was removed from the task container; no
credentials or raw state were printed or committed.

Full frontend checks in isolated kind namespace passed: typecheck, lint,
complete Vitest suite (107 files / 1,027 tests) and build. The first backend
run was superseded after final GVK/absence fixes; its 10-minute per-package
limit was below the documented main baseline duration. Final source was checked
with `go test -p 2 -parallel 4 -timeout 20m ./...`, real PostgreSQL 16 and
`REQUIRE_POSTGRES=1`. An early 13-package failure result included a runner error:
`platform-catalog` was omitted by the source-copy list, breaking catalog export
fixtures. The package passed once the full fixture tree was restored. This
harness result is not counted as a product/main baseline failure. The runner now
receives the full tracked tree via `git archive`; full corrected task checks and
a pristine `origin/main` (`b1445707`) run use that complete structure. Main
completed with 13 failing packages. The corrected task run also completed
with the **same 13 packages and exactly the same failing test/subtest sets**:
no task-only package or failing test. Neither run timed out. Full task build
returned exit 0; changed readiness/runner/tfexec packages pass. The full backend
gate remains red; parity with main does not turn it into a passing full suite.

| Package under backend | Representative baseline failure (both trees) |
|---|---|
| internal/adapters/postgres | TestBaselineEnforcesCurrentConstraints; TestVersionSixCapturesPopulatedFiveAndProtectsSnapshots; TestUpgradeBackupRestore; TestVersionSevenPreservesConnectionsAndAdmitsGit; TestUpgradeHTTPLoginPreviewDeploy |
| internal/adapters/templatesources | TestInspectRejectsUnsafeSourcesWithoutFetching |
| internal/application/catalog | TestListDefinitionCapabilities_PlatformPairsExistingClusterAndWorkload; TestWorkloadRegistration_TypedPlacementInputsAndCompanions |
| internal/application/configuration | TestRenameWithReferencesPreservesAliasesManyToOneAndOtherFields |
| internal/application/deployment | TestCredentialBackedTarget_RemovalAfterRestart |
| internal/application/pending | TestRequiredRendererMissingTieAndUnavailableFailBeforeResources |
| internal/application/preview | TestPreview_NoRuntimeMutation |
| internal/application/provisioning | TestProvision_PostgresOutputContractEquivalentAcrossPlacements |
| internal/bootstrap | TestRealProfileAcceptsOnlyTheFullRealWiring |
| test/conformance | TestPlannerMatchesChallengeFixtures; TestProductPlannerRejectsCriteriaLessDefinition |
| test/e2e | TestUC12UC16HTTPDraftFlow |
| test/integration/v2cleanup | TestDeliveryCleansAppliedTargetsWithoutLegacyEnvironmentBinding |
| test/integration/v2run | TestTheScenarioScriptRecordsExactlyTheAssertionsTheRunnerRequires |

Direct module Terraform destroy removed both managed resources; authoritative
queries confirmed Cluster/Pod/PVC/Secret/Service absence, namespace absence and
zero remaining PVs referring to that namespace. Product cleanup is separately
verified below. Both isolated full-test namespaces were deleted with owner
checks and UID preconditions, followed by authoritative absence verification.
No preexisting test image was removed. Operator is retained for user review.
No main deployment/merge, release tag or acceptance waiver is implied by parity
with the existing main failures. The missing UID limitation remains IMP-043.

Documentation checker, Node syntax check for the E2E runner and diff whitespace
checks pass. Local PlantUML rendering was skipped because PlantUML is not
installed; no diagrams were changed.

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
The Console catalog route was opened through the actual Tailscale feature
hostname; browser checks confirmed its loaded list, reopened HA editor, all eight
saved mappings, final immutable pin and staging/ha criteria.
Private runtime scripts/config/evidence/passwords remain outside Git. The shared
Console additionally retains the approved optional Type output and task HA
Definition. Existing in-cluster Definition and unrelated workloads were not
modified. No merge, tag v0.6.0 or Definition tag promotion has occurred.
