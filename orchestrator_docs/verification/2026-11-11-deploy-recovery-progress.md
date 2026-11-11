---
id: VERIFY-2026-11-11-DEPLOY-RECOVERY-PROGRESS
artifact: verification
status: current
last_reviewed: 2026-11-11
---

# Deploy and Recovery progress — issue #69

Implemented on `features/deploy-recovery-progress`, based on main `dfa6470c`.
Not merged or user accepted. The [contract](../architecture/contracts/deployment-progress.md)
and [implementation](../implementation/deploy-recovery-progress.md) describe the
stored facts, safety projection and unchanged execution/ownership rules.

## Technical regression observations

A separate Docker Console with its own database and Vault used real kind, Git,
Fleet and PostgreSQL. The shared UI design, existing application home, Settings,
deployment detail, shell and components were the visual references. Browser
checks used the actual Tailscale feature URL and real data; screenshots and
credential files remain outside Git. These automated observations do **not**
replace the six user-driven Console acceptance cases required by the issue.

- A valid image update with PostgreSQL completed. Git publication, Fleet apply
  and readiness appeared separately, with persisted elapsed times.
- An invalid image showed its image, ImagePullBackOff and scoped events while
  more than four minutes remained. At timeout the failure hint and Environment
  unlock conclusion appeared. Restarting the feature backend preserved history.
- A crashing container showed CrashLoopBackOff, restart counts and scoped events
  before timeout. Shared-release evidence appeared once under its workload.
- Stopping only the feature backend interrupted that Deploy. After restart,
  Recovery showed the original heartbeat, then persisted 15 checks and its
  settlement conclusion. Its readiness check retained CrashLoopBackOff, eight
  restarts, six safe events and the readiness deadline. Recovery settled the
  failed deployment; it did not make the crashing workload healthy.
- Public event fields omit raw controller/event messages; Terraform disclosure
  contains structured stage/status facts, not raw CLI output. Focused tests
  reject credential-bearing URLs and retain valid OCI digest image references.
  No credential contents were used as browser evidence.
- Visual review kept the existing shell/navigation. Timeline details, extra
  events, stage logs and Recovery checks use disclosure controls; the banner
  retains the current step, warning and detail navigation.

The fresh user trial retains a saved workload/PostgreSQL draft and a valid
Preview, without a Deploy. The verified Console URL, login file path, service
restart commands and four-hour scoped namespace setup helper are in the private
handoff. Its feature stack and endpoint forwards remain running.

## Checks

Focused domain progress, operation lease, delivery/Recovery, Fleet projection,
HTTP query and Terraform adapter suites passed, including race checks and real
PostgreSQL reopen/correlation checks. They cover preservation across Stage and
restart, exact deployment correlation, release/revision ownership, UID-scoped
events, retained Recovery observations and unchanged no-replay guards.

Full Console validation on kind passed: 106 files, 1,015 tests, typecheck, lint
and build. Lint retains the preexisting PlatformSettings ref cleanup warning;
build retains the existing large-chunk advisory. Documentation checker, changed
PlantUML sequence, migration immutability guard and whitespace checks passed.
No schema migration was needed or released migration changed.

Full backend ran sequentially for pinned main `dfa6470c` and feature source
through `a952dd86` in an owned kind namespace, with real PostgreSQL 16, Redis,
Node 22, PostgreSQL 16 client tools and the complete repository/dependencies.
`REQUIRE_POSTGRES=1 GOMAXPROCS=4 go test -p 2 -count=1 -timeout 30m ./...`
failed on **both**: exactly the same 70 top-level test names in the same 13
packages. No additional named failure or failing package appeared on the
feature branch. `go build ./...` passed. This is **not** a full-green gate.

The final PostgreSQL instance used 512 connections and 1,024 locks per
transaction. Earlier underprovisioned attempts are excluded; neither final log
contains connection/shared-memory exhaustion codes. Full Console checks were
also run inside kind. Complete private logs, exit codes and the comparison are
retained outside Git.

Failing packages on both revisions:

- `orchestrator/internal/adapters/postgres`
- `orchestrator/internal/adapters/templatesources`
- `orchestrator/internal/application/catalog`
- `orchestrator/internal/application/configuration`
- `orchestrator/internal/application/deployment`
- `orchestrator/internal/application/pending`
- `orchestrator/internal/application/preview`
- `orchestrator/internal/application/provisioning`
- `orchestrator/internal/bootstrap`
- `orchestrator/test/conformance`
- `orchestrator/test/e2e`
- `orchestrator/test/integration/v2cleanup`
- `orchestrator/test/integration/v2run`

<details>
<summary>Same 70 top-level failing tests on main and feature</summary>

- `TestBaselineEnforcesCurrentConstraints`
- `TestBuildRefusesMemoryOutputSecretsWithRealAdaptersAndDurableState`
- `TestConfigurationDTOIsRedactedAndVariableCRUDNeverResolvesRegistry`
- `TestCredentialBackedTarget_RemovalAfterRestart`
- `TestDeliveryCleansAppliedTargetsWithoutLegacyEnvironmentBinding`
- `TestDeployWithUnknownTerraformOutcomeHoldsTheEnvironment`
- `TestDeploymentHistoryHidesUnsafeFailureReasons`
- `TestDesiredPlansDraftsBeforeDeployAndTheUnchangedAppliedState`
- `TestDesiredReadsOneSnapshotAgainstConcurrentWriters`
- `TestEnvironmentSettingsStoreSelectionConflictsAndBusyStateOverHTTP`
- `TestEnvironmentWritesDuringAdmittedDeployAreRejected`
- `TestFailedWorkloadRemovalKeepsOriginalOwnershipDeployment`
- `TestHTTPDeploymentEndToEnd`
- `TestHTTPDeploymentHistoryScopeFilterPlanningFailureAndSafeJSON`
- `TestHTTPDeploymentReadsOnPostgres`
- `TestHTTPDeploymentReadsRequireScopedSessionAndHideResolvedInputs`
- `TestHTTPUsesTheConfiguredRunID`
- `TestInspectRejectsUnsafeSourcesWithoutFetching`
- `TestIntentEvidenceStaysInternal`
- `TestListDefinitionCapabilities_PlatformPairsExistingClusterAndWorkload`
- `TestPendingDeployPartialFailureCanRetryOnlyRemainingWorkload`
- `TestPendingPreviewReadsOneSnapshot`
- `TestPendingPreviewReadsOneSnapshotOnPostgres`
- `TestPlannerMatchesChallengeFixtures`
- `TestPostgresPreview_RepeatableReadAndNoWrites`
- `TestPostgresRemove_CommitsSetStatusAndMarkerAtomically`
- `TestPreviewIntentSameDefinitionKeyChangedFallbackIsUpdate`
- `TestPreviewIntentUpdateRetainUnverifiedForExistingIdentity`
- `TestPreviewRefusesIneligibleRootsWithAllowlistedReasons`
- `TestPreview_BaselineEvidenceMissingIsUnverifiedAndStoreFailureFails`
- `TestPreview_BaselinePlansAreCapturedInsideTheSnapshot`
- `TestPreview_CatalogErrorsNeverEchoed`
- `TestPreview_DeltaInvariantAndContainerResources`
- `TestPreview_MatchesDeployPlan`
- `TestPreview_NoChangeHasEmptyDelta`
- `TestPreview_NoRuntimeMutation`
- `TestPreview_PublicViewRedactsWithoutChangingPlan`
- `TestPreview_ReadsOneConsistentSnapshot`
- `TestPreview_StrictQuery`
- `TestProductPlannerRejectsCriteriaLessDefinition`
- `TestProvision_ClusterReadinessFailureBlocksEveryConsumer`
- `TestProvision_EKSCreatedInTheSameExecutionDrivesNamespaceDatabaseAndWorkload`
- `TestProvision_EKSIsReusedNotDuplicatedByALaterDeployment`
- `TestProvision_MissingRequiredOutputBlocksTheConsumerAndIsRecorded`
- `TestProvision_PostgresOutputContractEquivalentAcrossPlacements`
- `TestProvision_ReusesEnvironmentScopedVPCAndEKS`
- `TestProvision_UsableClusterLetsConsumersRunAndIsCheckedEachDeployment`
- `TestProvision_V2ExecutorGetsPinsIdentityAndProviderInstances`
- `TestProvision_V2RuntimeSensitiveOutputsAreStoredAsReferences`
- `TestRealProfileAcceptsOnlyTheFullRealWiring`
- `TestRemoveDeliveredAlsoRemovesTheApplicationsSecretDeliveryObjects`
- `TestRemoveDeliveredRefusesAWrongGenerationAndProtectsOthers`
- `TestRemoveDeliveredRetryUsesOriginalOwnerAfterRemovalSnapshotChanged`
- `TestRemoveDeliveredUsesTheClaimAndEachOriginalTarget`
- `TestRemoveWorkload_PreservesSharedDatabaseAndMarksLastReference`
- `TestRenameWithReferencesPreservesAliasesManyToOneAndOtherFields`
- `TestRequiredRendererDeployRendersWithRealCLIAfterOutputs`
- `TestRequiredRendererMissingTieAndUnavailableFailBeforeResources`
- `TestRequiredRendererPreviewIsPurePinsAndDeployRendersAfterOutputs`
- `TestSavedUndeployedDraftIsPlannedIntoExactRequirementsBeforeAnyProbe`
- `TestStaleStateIsRejectedBeforeAnyEffect`
- `TestTheScenarioScriptRecordsExactlyTheAssertionsTheRunnerRequires`
- `TestUC12UC16HTTPDraftFlow`
- `TestUnreferencedMarking_StaysInOrganizationAndEnvironmentScope`
- `TestUpgradeBackupRestore`
- `TestUpgradeHTTPLoginPreviewDeploy`
- `TestVersionSevenPreservesConnectionsAndAdmitsGit`
- `TestVersionSixCapturesPopulatedFiveAndProtectsSnapshots`
- `TestWorkloadRegistration_TypedPlacementInputsAndCompanions`
- `TestWorkloadRemovalAlsoRemovesItsSecretDeliveryObjects`

</details>

After preserving evidence, the owned test namespace was deleted with a checked
run label and UID precondition; absence was verified. The separate disposable
local PostgreSQL container and its test network were removed. This cleanup is
separate from the blocked technical application's product cleanup below.

## Cleanup limitation

The separate technical application's product deletion stopped at workload
withdrawal with the existing Fleet ownership/incarnation guard. The deployment
operation is interrupted, and Recovery is refused while the application is
being deleted. No controller ownership, stored receipt, namespace or shared
RBAC was rewritten to bypass this guard. The technical application, namespace,
workload/PostgreSQL and its bounded bootstrap Role/RoleBinding therefore remain
for diagnosis; the private handoff records their exact identities. This does not
claim clean application cleanup. The user trial is a separate application.

## Remaining gate

The user must manually perform all six issue cases through Console on real kind.
No API call, fake adapter or automated script is counted as that acceptance.
Keep issue #69 open and do not merge until the user accepts. Full-suite baseline
failures and the technical cleanup limitation must remain explicit at handoff.

## User layout refinement

At the user's request, the new timeline now sits immediately below the existing
deployment map and its Deploy progress, before the deployment metadata panels.
Only the section order and owning UI design changed. The existing shell, styles
and disclosure content are reused; no extra product copy was added. A real
browser reopened the same Tailscale deployment detail and verified the two
sections are adjacent in that order, with the saved failure evidence visible.
The 55 focused deployment/timeline tests passed. Full frontend checks were
repeated on kind: 106 files / 1,015 tests, typecheck, lint and build passed.
The owned UI test namespace was removed with a checked label/UID; backend checks
were not repeated for this presentation-only
change, and the baseline failures and cleanup limitation above remain unchanged.
