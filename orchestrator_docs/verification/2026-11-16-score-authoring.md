---
id: VER-2026-11-16-SCORE-AUTHORING
artifact: verification-record
status: evidence
last_reviewed: 2026-11-16
---

# Score authoring verification — issue #83

Feature branch `features/score`; base `cd40a792`. Not merged, not accepted, and no
retained main installation rollout. Scope is standard Score input plus a supported
Humanitec-format extension/profile, retaining existing infrastructure algorithms.
See [contract](../architecture/contracts/score-workload.md) and
[implementation](../implementation/score-workload.md).

## Focused and full checks

Focused workloadspec, workloadconfig, planner and HTTP suites pass, along with
Go build. Console checks include 61 existing related tests, three new document
tests and a final 74-test authoring check after visual refinement. Typecheck,
lint, production build, documentation checker and whitespace checks pass.
Both changed UC-16 PlantUML diagrams passed check-only validation using the
PlantUML 1.2025.2 container; the host has no PlantUML executable. Existing
PlatformSettingsPage lint effect-ref and frontend chunk-size warnings remain.

Full checks ran on the authorized kind cluster in a dedicated namespace, with
separate PostgreSQL 16 servers for candidate and main, max_locks_per_transaction
1024/max_connections 512, Go 1.25.14, Node 22.23.4 and Terraform 1.9.8.
REQUIRE_POSTGRES=1, REQUIRE_TERRAFORM=1, GOMAXPROCS=4;
`go test -json -p 2 -parallel 4 -count=1 -timeout 30m ./...`, then `go build ./...`.
After the final explicit/native CronJob suspend-overlap guard, the entire
candidate backend suite and build were repeated in a fresh isolated namespace
with the same tools/settings. The unchanged pinned-main baseline above was
reused; exact failure/skip sets still match, there are no package timeouts and
the final candidate backend snapshot matches the final backend files exactly.

| Check | Candidate | Main |
|---|---|---|
| Completed packages | 144 | 144 |
| Top-level failing tests | 70 | 70 |
| Failing tests including subtests | 104 | 104 |
| Failing packages | 14 | 14 |
| Backend build | pass | pass |
| Package timeouts | none | none |
| Additional candidate failures/skips | none | reference |
| Full frontend on kind | 113 files / 1063 tests pass | affected timing test compared |

The backend suite is **not green**. Exact failure names, package failures and
existing opt-in skips match main; none was removed or relaxed to obtain parity.
The first full frontend run hit an EffectivenessPage timing assertion under load.
Candidate and main both passed that unchanged nine-test file independently;
the full rerun passed. After moving parsed internals into a collapsed disclosure,
the final full frontend run, typecheck/lint/build also passed with two workers.
All earlier failure logs remain in private evidence.

<details>
<summary>Same top-level backend failures on candidate and main</summary>

- `orchestrator/internal/adapters/postgres::TestBaselineEnforcesCurrentConstraints`
- `orchestrator/internal/adapters/postgres::TestUpgradeBackupRestore`
- `orchestrator/internal/adapters/postgres::TestUpgradeHTTPLoginPreviewDeploy`
- `orchestrator/internal/adapters/postgres::TestVersionSevenPreservesConnectionsAndAdmitsGit`
- `orchestrator/internal/adapters/postgres::TestVersionSixCapturesPopulatedFiveAndProtectsSnapshots`
- `orchestrator/internal/adapters/templatesources::TestInspectRejectsUnsafeSourcesWithoutFetching`
- `orchestrator/internal/application/catalog::TestListDefinitionCapabilities_PlatformPairsExistingClusterAndWorkload`
- `orchestrator/internal/application/catalog::TestWorkloadRegistration_TypedPlacementInputsAndCompanions`
- `orchestrator/internal/application/configuration::TestRenameWithReferencesPreservesAliasesManyToOneAndOtherFields`
- `orchestrator/internal/application/deployment::TestCredentialBackedTarget_RemovalAfterRestart`
- `orchestrator/internal/application/deployment::TestDeploymentHistoryHidesUnsafeFailureReasons`
- `orchestrator/internal/application/deployment::TestEnvironmentWritesDuringAdmittedDeployAreRejected`
- `orchestrator/internal/application/deployment::TestFailedWorkloadRemovalKeepsOriginalOwnershipDeployment`
- `orchestrator/internal/application/deployment::TestPostgresRemove_CommitsSetStatusAndMarkerAtomically`
- `orchestrator/internal/application/deployment::TestRemoveDeliveredAlsoRemovesTheApplicationsSecretDeliveryObjects`
- `orchestrator/internal/application/deployment::TestRemoveDeliveredRefusesAWrongGenerationAndProtectsOthers`
- `orchestrator/internal/application/deployment::TestRemoveDeliveredRetryUsesOriginalOwnerAfterRemovalSnapshotChanged`
- `orchestrator/internal/application/deployment::TestRemoveDeliveredUsesTheClaimAndEachOriginalTarget`
- `orchestrator/internal/application/deployment::TestRemoveWorkload_PreservesSharedDatabaseAndMarksLastReference`
- `orchestrator/internal/application/deployment::TestUnreferencedMarking_StaysInOrganizationAndEnvironmentScope`
- `orchestrator/internal/application/deployment::TestWorkloadRemovalAlsoRemovesItsSecretDeliveryObjects`
- `orchestrator/internal/application/pending::TestDesiredPlansDraftsBeforeDeployAndTheUnchangedAppliedState`
- `orchestrator/internal/application/pending::TestDesiredReadsOneSnapshotAgainstConcurrentWriters`
- `orchestrator/internal/application/pending::TestIntentEvidenceStaysInternal`
- `orchestrator/internal/application/pending::TestPendingPreviewReadsOneSnapshot`
- `orchestrator/internal/application/pending::TestPendingPreviewReadsOneSnapshotOnPostgres`
- `orchestrator/internal/application/pending::TestPreviewIntentSameDefinitionKeyChangedFallbackIsUpdate`
- `orchestrator/internal/application/pending::TestPreviewIntentUpdateRetainUnverifiedForExistingIdentity`
- `orchestrator/internal/application/pending::TestRequiredRendererDeployRendersWithRealCLIAfterOutputs`
- `orchestrator/internal/application/pending::TestRequiredRendererMissingTieAndUnavailableFailBeforeResources`
- `orchestrator/internal/application/pending::TestRequiredRendererPreviewIsPurePinsAndDeployRendersAfterOutputs`
- `orchestrator/internal/application/preview::TestPostgresPreview_RepeatableReadAndNoWrites`
- `orchestrator/internal/application/preview::TestPreview_BaselineEvidenceMissingIsUnverifiedAndStoreFailureFails`
- `orchestrator/internal/application/preview::TestPreview_BaselinePlansAreCapturedInsideTheSnapshot`
- `orchestrator/internal/application/preview::TestPreview_CatalogErrorsNeverEchoed`
- `orchestrator/internal/application/preview::TestPreview_DeltaInvariantAndContainerResources`
- `orchestrator/internal/application/preview::TestPreview_MatchesDeployPlan`
- `orchestrator/internal/application/preview::TestPreview_NoChangeHasEmptyDelta`
- `orchestrator/internal/application/preview::TestPreview_NoRuntimeMutation`
- `orchestrator/internal/application/preview::TestPreview_PublicViewRedactsWithoutChangingPlan`
- `orchestrator/internal/application/preview::TestPreview_ReadsOneConsistentSnapshot`
- `orchestrator/internal/application/preview::TestPreview_StrictQuery`
- `orchestrator/internal/application/provisioning::TestDeployWithUnknownTerraformOutcomeHoldsTheEnvironment`
- `orchestrator/internal/application/provisioning::TestProvision_ClusterReadinessFailureBlocksEveryConsumer`
- `orchestrator/internal/application/provisioning::TestProvision_EKSCreatedInTheSameExecutionDrivesNamespaceDatabaseAndWorkload`
- `orchestrator/internal/application/provisioning::TestProvision_EKSIsReusedNotDuplicatedByALaterDeployment`
- `orchestrator/internal/application/provisioning::TestProvision_MissingRequiredOutputBlocksTheConsumerAndIsRecorded`
- `orchestrator/internal/application/provisioning::TestProvision_PostgresOutputContractEquivalentAcrossPlacements`
- `orchestrator/internal/application/provisioning::TestProvision_ReusesEnvironmentScopedVPCAndEKS`
- `orchestrator/internal/application/provisioning::TestProvision_UsableClusterLetsConsumersRunAndIsCheckedEachDeployment`
- `orchestrator/internal/application/provisioning::TestProvision_V2ExecutorGetsPinsIdentityAndProviderInstances`
- `orchestrator/internal/application/provisioning::TestProvision_V2RuntimeSensitiveOutputsAreStoredAsReferences`
- `orchestrator/internal/application/tfexec::TestDestroyUsesRetainedHistoricalReleaseNeverTheCurrentOne`
- `orchestrator/internal/bootstrap::TestRealProfileAcceptsOnlyTheFullRealWiring`
- `orchestrator/test/conformance::TestPlannerMatchesChallengeFixtures`
- `orchestrator/test/conformance::TestProductPlannerRejectsCriteriaLessDefinition`
- `orchestrator/test/e2e::TestConfigurationDTOIsRedactedAndVariableCRUDNeverResolvesRegistry`
- `orchestrator/test/e2e::TestEnvironmentSettingsStoreSelectionConflictsAndBusyStateOverHTTP`
- `orchestrator/test/e2e::TestHTTPDeploymentEndToEnd`
- `orchestrator/test/e2e::TestHTTPDeploymentHistoryScopeFilterPlanningFailureAndSafeJSON`
- `orchestrator/test/e2e::TestHTTPDeploymentReadsOnPostgres`
- `orchestrator/test/e2e::TestHTTPDeploymentReadsRequireScopedSessionAndHideResolvedInputs`
- `orchestrator/test/e2e::TestHTTPUsesTheConfiguredRunID`
- `orchestrator/test/e2e::TestPendingDeployPartialFailureCanRetryOnlyRemainingWorkload`
- `orchestrator/test/e2e::TestPreviewRefusesIneligibleRootsWithAllowlistedReasons`
- `orchestrator/test/e2e::TestSavedUndeployedDraftIsPlannedIntoExactRequirementsBeforeAnyProbe`
- `orchestrator/test/e2e::TestStaleStateIsRejectedBeforeAnyEffect`
- `orchestrator/test/e2e::TestUC12UC16HTTPDraftFlow`
- `orchestrator/test/integration/v2cleanup::TestDeliveryCleansAppliedTargetsWithoutLegacyEnvironmentBinding`
- `orchestrator/test/integration/v2run::TestTheScenarioScriptRecordsExactlyTheAssertionsTheRunnerRequires`

</details>

## Real Console and runtime

A separate Docker PostgreSQL/Vault/backend/frontend stack was used. The exact
feature Tailscale origin was opened in Playwright, authenticated and checked with
real same-origin APIs. No mocked backend was used for reachability or runtime.
Credentials, source configuration and screenshots remain outside Git.

Through Console, the task Application imported a Deployment with PostgreSQL
resource bindings and a CronJob from Score plus extension. Export/reimport
produced the same normalized workload, pin and defaults snapshot. Both drafts
were saved before one Environment Preview and Deploy.

The first request failed with ROLLOUT_NOT_READY because the existing Fleet subject
initially lacked namespace-local RBAC. The bootstrap namespace-access helper then
created only app-labelled Role/RoleBinding in the product-created namespace;
controller/cluster permissions were unchanged. Fleet subsequently became Ready,
but the failed operation was not relabelled successful. Its evidence is retained.

After confirming the operation was terminal, an unchanged Preview correctly had
no workload changes because the delivered desired state was already applied. A
valid Score update then changed both workloads, saved both drafts and successfully
Previewed/Deployed both together through the real Console. The second operation
returned SUCCEEDED. Setup RBAC was verified/reused before observation.

Native checks passed: Deployment available, resolved PGHOST, application
pg_isready marker, authenticated SQL SELECT 1, exact CronJob schedule/timezone and
a completed Job using the updated command. Deployment, CronJob, PostgreSQL
StatefulSet and PVC UIDs were preserved across the update. This proves actual
resources and execution, not only rendered YAML or bootstrap setup.

## Visual comparison and limits

References: live signed-in Console, AppShell, tokens/app styles, Button, existing
Workload editor and import pattern. The changed UI retains shell, navigation,
colors and form groups. The full normalized document/default snapshot was moved
into an accessible details disclosure; file choices, parsed workload name and
Save/export remain visible. No unrelated screen was redesigned.

The latest backend image was also opened on the exact Tailscale review route;
both persisted drafts exported and reimported with unchanged normalized data,
frozen template pins and defaults. No source re-registration was necessary.

The supported subset is explicit: valid but unsupported Score/extension fields
fail with safe codes rather than being dropped. This does not claim complete
Score implementation coverage or full Humanitec compatibility. No Job profile,
Helm/score-k8s runtime, Terraform/graph/matching/CD change or schema reset was made.

## Cleanup and acceptance

Both full-check namespace deletions verified recorded owner labels, UIDs and
resource versions; successful absence was recorded. Runtime cleanup completed through product APIs: the CronJob was suspended via
Save/Preview/Deploy, then Application deletion withdrew both workloads and the
Fleet controller and destroyed PostgreSQL/storage. Namespace cleanup initially
stopped at the setup-owned RBAC. After verifying namespace/application labels,
recorded Role/RoleBinding UIDs, expected rules/subject and completed CD/workload
withdrawal, only those two objects were removed with UID/resourceVersion
preconditions. Product deletion retry returned SUCCEEDED. Native reads verified
namespace, GitRepo controller and persistent volume absence. No foreign-resource
guard was bypassed.

Only a separate review Application with two saved drafts is retained in the
feature Console; it has not provisioned workload resources. The generated hosted
Git repository from runtime remains private audit data under current product
retention; no live GitRepo/controller watches it.

The feature Docker stack is retained for user review. Access URL and private
login/restart instructions are provided only in the user handoff, not this public
record. User acceptance is required before merge or retained main deployment.
