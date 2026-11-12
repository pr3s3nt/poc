---
id: VERIFY-CATALOG-DOCUMENTS-MAIN-20261112
artifact: verification
status: current
last_reviewed: 2026-11-12
---

# Catalog document registration — accepted main integration

The user accepted issue #76 and explicitly requested merging first, then finishing
the remaining checks and fixing any regressions. Main merge `48f28ad1` is pushed.
Backend/frontend/scripts/deploy/e2e product source is identical to tested candidate
`7733fe90`; the baseline was pristine main `9d80dbe4`. No conflict resolution or
additional product change was needed.

## Checks

Full checks used an isolated kind namespace, separate PostgreSQL 16 servers for
candidate/main, Go 1.25.14, Node 22, Terraform 1.9.8, Redis 7 and PostgreSQL 16
clients. PostgreSQL used max_locks_per_transaction=1024 and max_connections=512.
Both ran with REQUIRE_POSTGRES=1, REQUIRE_TERRAFORM=1, GOMAXPROCS=4 and
`go test -json -p 2 -parallel 4 -count=1 -timeout 30m ./...`, followed by Go build.

| Check | Candidate | Main baseline |
|---|---|---|
| Packages completed, including packages without tests | 144 | 144 |
| Failing top-level tests | 70 | 70 |
| All failing names including subtests | 104 | 104 |
| Failing packages | 14 | 14 |
| Additional candidate failures/skips/timeouts | none | reference |
| Package timeouts | none | none |
| Backend build | pass | pass |
| Full frontend | 111 files / 1049 tests pass | not repeated |
| Frontend typecheck/lint/production build | pass | not repeated |

The full backend suite is **not green**. Candidate and baseline have exactly the
same failure names and packages; this is baseline parity, not a claim that those
tests passed. The earlier feature-run missing Terraform/fixture and timeout limits
are replaced for this comparison by complete source snapshots and the tools above.

The first runner containers were interrupted by the environment (Unknown/255).
Their completed results were retained: 124 packages had terminal results on each
role, and PostgreSQL had finished without timeout. Continuation enumerated all
packages and top-level tests from the same snapshots, ran unfinished tests in the
two interrupted packages and all unstarted packages, and verified terminal results
for every planned test/package before comparing the combined logs. No completed
test was discarded, no assertion was weakened, and no candidate-only skip was added.
The merge occurred before the final continuation report, as explicitly requested.

<details>
<summary>Same top-level failing tests on candidate and baseline</summary>

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

## Retained Console rollout

Runtime images are `orchestrator-backend:main-48f28ad1` and
`orchestrator-frontend:main-48f28ad1`, retagged from the exact candidate images.
They were imported into the authorized kind cluster before rollout. Preflight
verified no active/recovering/interrupted work requiring an operator decision.

The retained installation was quiesced, a private SQL custom-format backup was
taken and restored into an isolated verification database, and the platform PVC
credential/Vault/state-key/cache directories were backed up with writers stopped.
The verification database was removed after successful restore. Existing data was
not reset. Owner setup reported that baseline/grants already existed, nothing
changed; it ran before runtime. Runtime mounts/environment contain no owner
credential. Backend/frontend are Ready. The task backup Pod was removed using
its recorded ownership/UID/resourceVersion preconditions.

The environment interruption had also removed the host forwards. The main Console
and shared feature-proxy forwards were restored without closing user sessions or
stopping other feature projects. The main forward reconnected to the new frontend
Pod after rollout. The exact Tailscale URL was opened in a real authenticated
browser with real same-origin catalog APIs; no mock or HTML-only check was used.

## Post-rollout browser checks

- Existing Type: schema download, upload into the method tab, REUSED, changed
  draft CONFLICT, keyboard/tab draft retention, original catalog unchanged.
- Existing Terraform Definition: export preserves source pin/driver inputs and
  drops system fields; upload REUSED; changed name CONFLICT; complete revision
  history and Definition list unchanged.
- Non-finite YAML stays editable, shows syntax error and sends no import POST.
- The 4 preexisting visible Application identities match preflight and remain unchanged.

The smoke reused existing catalog objects and did not add test-only Types,
Definitions or Applications to retained main. Its scope is the newly deployed
import/export surface; real provisioning/default/override/dependency validation
was already performed on the accepted feature stack in the
[feature verification](2026-11-12-catalog-documents.md).

## Cleanup and handoff

The feature branch was deleted locally and remotely after the accepted merge.
Only Docker project `poc-dangkiresource` was stopped; its volumes, private inputs
and source caches remain for tracing. Shared proxy/forwards and other projects
were preserved. The full-check namespace was deleted after its owner label and
recorded UID were verified; namespace absence was confirmed. The checkout started
clean, and no preexisting user-owned change was staged, reverted or discarded.

Documentation checker, migration immutability guard and whitespace check pass.
No SQL migration was introduced. Post-rollout reporting changes only documents;
it does not change the validated product tree or require another deployment.

Exact infrastructure URLs, credentials, backup locations, screenshots, raw logs
and comparison/resume receipts remain outside Git. The private handoff supplies
the current Console URL, password-file reference and restart commands. Browser
reachability was checked from the host, not from a second tailnet device.
