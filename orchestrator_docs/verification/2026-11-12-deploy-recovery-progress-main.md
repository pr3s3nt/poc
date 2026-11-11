---
id: VERIFY-2026-11-12-DEPLOY-RECOVERY-PROGRESS-MAIN
artifact: verification
status: current
last_reviewed: 2026-11-12
---

# Deploy/Recovery progress — accepted main integration

The user requested merging issue #69 into main on 2026-11-12. This supersedes
the earlier awaiting-acceptance status. HA/topology discussion remains design
exploration only; no topology product change is included.

## Integration and checks

Candidate `7f80dfef` integrates main `b1445707`, retaining #67/#68/#72 behavior.
Conflicts kept both sets of UI messages/styles/docs and typed projections.
The lease context exposes both the annotation lease and progress reporter.
Annotation/progress/heartbeat writers share one lock, and progress persists the
merged annotation snapshot. This prevents #68 Pod wait diagnostics and #69
progress overwriting each other. Concurrent + Stage regression coverage passed
with race checks. Deploy/Recovery decisions and ownership guards are unchanged.

Full checks ran in an isolated kind namespace, with independent real PostgreSQL
16 servers for candidate and pristine main, Node 22, PostgreSQL 16 clients,
Terraform 1.9.8 and Redis. Both used `REQUIRE_POSTGRES=1 REQUIRE_TERRAFORM=1`,
`GOMAXPROCS=4 go test -p 2 -parallel 4 -count=1 -timeout 30m ./...`.

| Check | Candidate | Main baseline |
|---|---|---|
| Failing top-level Go tests | 70 | 70 |
| All named failures including subtests | 104 | 104 |
| Failing packages | 14 | 14 |
| Difference in failing names/packages | none | none |
| Backend build | pass | not repeated |
| Full frontend | 108 files / 1,034 tests pass | not repeated |
| Frontend typecheck/lint/build | pass | not repeated |

Full backend remains red; this is not a full-green gate. Neither final log has
connection/shared-memory exhaustion codes. Scoped Go delivery/Fleet/HTTP suites,
progress/lease race suites, 73 focused UI tests, migration immutability, docs and
whitespace checks passed. Existing lint/chunk-size advisories remain.

Failing packages on both revisions:

- `orchestrator/internal/adapters/postgres`
- `orchestrator/internal/adapters/templatesources`
- `orchestrator/internal/application/catalog`
- `orchestrator/internal/application/configuration`
- `orchestrator/internal/application/deployment`
- `orchestrator/internal/application/pending`
- `orchestrator/internal/application/preview`
- `orchestrator/internal/application/provisioning`
- `orchestrator/internal/application/tfexec`
- `orchestrator/internal/bootstrap`
- `orchestrator/test/conformance`
- `orchestrator/test/e2e`
- `orchestrator/test/integration/v2cleanup`
- `orchestrator/test/integration/v2run`

<details>
<summary>Same top-level failing tests on candidate and main</summary>

- `TestBaselineEnforcesCurrentConstraints`
- `TestConfigurationDTOIsRedactedAndVariableCRUDNeverResolvesRegistry`
- `TestCredentialBackedTarget_RemovalAfterRestart`
- `TestDeliveryCleansAppliedTargetsWithoutLegacyEnvironmentBinding`
- `TestDeployWithUnknownTerraformOutcomeHoldsTheEnvironment`
- `TestDeploymentHistoryHidesUnsafeFailureReasons`
- `TestDesiredPlansDraftsBeforeDeployAndTheUnchangedAppliedState`
- `TestDesiredReadsOneSnapshotAgainstConcurrentWriters`
- `TestDestroyUsesRetainedHistoricalReleaseNeverTheCurrentOne`
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

## Main rollout receipt

The accepted merge includes this check record and documentation status updates;
product source is identical to tested candidate `7f80dfef`. Product images were
built from that candidate. The merge SHA, retained-data backup/restore, owner
setup, runtime image tags, exact browser checks and cleanup outcome are recorded
in the [issue #69 rollout comments](https://github.com/pr3s3nt/poc/issues/69).
This pre-rollout record alone does not claim the cluster has been upgraded.

Earlier technical browser observations and the blocked trial cleanup are in the
[original verification](2026-11-11-deploy-recovery-progress.md). Those automated
observations were not counted as user-driven acceptance. Private backups,
credentials, logs and infrastructure URLs remain outside Git/public comments.
