---
id: VERIFY-20261118-FIRST-RUN-SETUP
artifact: verification-record
status: current
last_reviewed: 2026-11-18
---

# First-run setup — issue #89

Feature branch `features/setup`; no main merge or main Console rollout.
Behavior: [UC-00 SETUP](../usecase/UC-00/first-run-setup.md).
Operation: [first-run guide](../operations/first-run-setup.md).

## Implemented outcome and real browser evidence

Operator-token initialization creates the Organization and first Admin atomically
and then closes. Existing installations can open the platform guide without
initialization mode. Git URL/ref/root are user inputs, with no fixed source or
resource list; discovery, prospective-Type validation, source preparation and
confirmed insert-only import include ordinary workload Type/template Definitions.

Separate Docker projects preserve a blank Console for the user's first-run trial
and an initialized Console for platform/Git review. Both exact Tailscale URLs were
opened in Chromium and their real same-origin setup/progress APIs checked. Actual
access URLs and private credential/evidence paths are supplied only in the user
handoff, not this public record. Checks were from the development host through
Tailscale; no other-device/ACL claim is made. Services and volumes are retained.

Browser initialized Organization/Admin, signed in, visited existing Git, Kubernetes,
Secret Store and CD forms, and discovered the supplied reference Git repository at
commit `8aed1e05170a003719919baf63917a9a15b88feb`. It imported **10 Types and 8
Definitions**, including a real workload Git template and Terraform modules.
Repeat preview/import returned REUSED for all 18 documents; no new revision or
business identity was required. A backend image update/restart and page reload
preserved accounts, source metadata, catalog data and closed initialization.

Cluster/MySQL/MongoDB Definitions were excluded from this reference run because
they need a target Driver Account or environment-approved image substitutions.
The wizard exposes those values for the operator; they were not guessed. Optional
HA Definition import is metadata validation, not proof of an installed CNPG
operator. No Application/workload deployment, controller installation, cloud
provisioning or main installation mutation was performed. The guide correctly
continues to show unsaved connection steps as incomplete.

## UI reference and content review

Reused current AppShell/navigation/styles, Button, PasswordInput, LanguageSelect
and the existing Git/account/Secret Store/CD forms. The running main sign-in page
was viewed first; the new setup and guide were then inspected visually in Chromium.
Identity, shell and common navigation patterns were retained. Primary initialize/
import actions use the existing primary button. Long configuration/JSON and help
remain in disclosures; loading, errors, saved counts and per-document results stay
visible. No unrelated screen redesign was included.

## Focused checks

- Initialization authorization, concurrent claims, durable restart and existing
  Organization refusal passed; real PostgreSQL with two independent pools proves
  one winner and atomic Organization/Admin persistence using runtime grants.
- Bootstrap defers seeds and restores the saved Organization identity. HTTP setup
  closure and platform authentication, Git HTTPS snapshot/ref/traversal, prospective
  Type validation without business writes, reuse/conflict and Terraform authoring
  version regression tests passed. Affected Go tests also passed with `-race` and
  real PostgreSQL.
- 57 focused Console auth/App/shell/routes/setup tests passed, covering explicit
  confirmation, Types-before-Definitions ordering, stale-review invalidation and
  conflict blocking.
- Documentation checker, diff whitespace check, shell syntax, Node scenario syntax
  and resolved Compose configuration checks passed. PlantUML is unavailable locally;
  the checker reported its syntax check skipped; no PlantUML file changed.

## Full checks

Backend candidate `715d37aa` is compared with `origin/main` at `1463d8cb` using
real PostgreSQL and Terraform in an isolated kind namespace. Final frontend source
`413d4581` has the identical backend tree and only adjusts primary button tone plus
canonical documentation. A separate full frontend run checks that final source.

Final frontend: **117 files / 1104 tests passed**; install, typecheck, lint and build
all exit 0. One existing hooks warning remains in PlatformSettingsPage; Vite reports
the existing large-chunk warning. An earlier full frontend run on `715d37aa` also
passed all checks and the same 1104 tests.

| Full backend result | Candidate | Main baseline |
|---|---:|---:|
| Passing top-level tests | 1611 | 1603 |
| Failing top-level tests | 103 | 103 |
| Skipped top-level tests | 8 | 8 |
| `go test ./...` exit | 1 | 1 |
| `go build ./...` exit | 0 | 0 |

The complete failure and skip sets (including subtests) match exactly. No
candidate-only failure/skip, no main-only failure/skip and no missing baseline
passing top-level test were found. All eight new top-level setup tests pass.
This is **baseline-red full-backend evidence**, not a green backend gate. No
assertion, fixture or existing test was weakened to obtain the comparison.

Both isolated verification namespaces were deleted with owner/UID checks and
absence confirmed. The separate full frontend receipt also records cleanup=true.
The full backend/frontend checker was reused from the prior namespace-template
scenario without copying it; its historical run-ID prefix does not change this
run's source attribution or task-owned namespace. Private JSONL logs, comparison,
receipts and browser evidence remain outside Git. Later handoff documentation and
the standalone Compose build-race fix do not change either tested product tree;
resolved Compose configuration and shell/docs checks cover those local changes.

## Added passing backend tests

- `orchestrator/internal/adapters/postgres`: `TestSetupAcrossPostgresBackendsHasOneWinner`.
- `orchestrator/internal/adapters/terraform/registry`: `TestSetupDiscoveryRecognizesContentAndPinsMovedBranch`.
- `orchestrator/internal/application/catalog`: `TestSetupPreviewReusesTerraformDefinitionWithoutAuthoredVersion`.
- `orchestrator/internal/application/catalog`: `TestSetupPreviewUsesProspectiveTypesWithoutBusinessWrites`.
- `orchestrator/internal/application/installationsetup`: `TestExistingOrganizationAndDisabledSetupCannotClaim`.
- `orchestrator/internal/application/installationsetup`: `TestInitializationAuthorizationRaceAndRestart`.
- `orchestrator/internal/bootstrap`: `TestSetupBootstrapDefersSeedAndRestoresIdentity`.
- `orchestrator/internal/delivery/http`: `TestSetupHTTPClosesAfterInitializationAndEnforcesPlatformRole`.

## Exact baseline failure inventory

<details>
<summary>103 failing top-level tests, identical on candidate and main</summary>

### `orchestrator/examples/acceptance-app/backend`

- `TestBackendBadRequestsDoNotTouchTheCache`
- `TestBackendCacheHitMissRefillThroughTheAPI`
- `TestBackendDeniedCacheOperationFailsDiagnosticsWhilePingAndProbeWork`
- `TestBackendHealthyLookupOfOtherJobDoesNotHideDeniedJobs`
- `TestBackendPingAllowedButProbeDeniedFails`
- `TestBackendPooledRedisAuthConnectionsRequireScopedKillAndRecoverFromPostgres`
- `TestBackendRedisOutageFailsDiagnosticsButKeepsDurableResult`
- `TestBackendUnmarkedCompletedJobIsNotVerifiedUntilMarked`
- `TestBackendWorkerWriteFailureStaysVisibleAfterRedisRecovers`
- `TestBackendWrongRedisPasswordCannotPass`

### `orchestrator/examples/acceptance-app/internal/jobcache`

- `TestCacheEntryExpiresToMiss`
- `TestCacheFailuresAreClassifiedAndNeverMisses`
- `TestCacheOutageIsUnreachableNotMiss`
- `TestCachePutGetAuthenticatedWithDeterministicTTL`
- `TestCacheTLSClientToPlainServerFails`
- `TestCacheTLSRejectsWrongHostname`
- `TestCacheTLSVerifiesChainAndHostname`
- `TestLookupCompletedFailsFastWhenCacheIsDown`
- `TestResolverCacheOutageServesDurableResultButReportsFailure`
- `TestResolverCorruptEntryIsOverwrittenFromDatabase`
- `TestResolverHitThenExpiredMissReadsSameRowAndRefills`
- `TestResolverInvalidBindingReportsConfigFailure`
- `TestResolverPendingIsNeverFrozenByTheCache`
- `TestResolverWrongPasswordIsFailureNotMiss`

### `orchestrator/examples/acceptance-app/internal/jobstore`

- `TestStoreWithReservedCharacterPassword`

### `orchestrator/examples/acceptance-app/worker`

- `TestWaitForCacheFailsClosed`
- `TestWorkerCacheFailureAfterCommitKeepsOneCompletedJob`
- `TestWorkerCommitsToPostgreSQLThenWritesThroughRedis`

### `orchestrator/internal/adapters/postgres`

- `TestBaselineEnforcesCurrentConstraints`
- `TestUpgradeBackupRestore`
- `TestUpgradeHTTPLoginPreviewDeploy`
- `TestVersionSevenPreservesConnectionsAndAdmitsGit`
- `TestVersionSixCapturesPopulatedFiveAndProtectsSnapshots`

### `orchestrator/internal/adapters/templatesources`

- `TestInspectRejectsUnsafeSourcesWithoutFetching`

### `orchestrator/internal/application/catalog`

- `TestListDefinitionCapabilities_PlatformPairsExistingClusterAndWorkload`
- `TestWorkloadRegistration_TypedPlacementInputsAndCompanions`

### `orchestrator/internal/application/configuration`

- `TestRenameWithReferencesPreservesAliasesManyToOneAndOtherFields`

### `orchestrator/internal/application/deployment`

- `TestCredentialBackedTarget_RemovalAfterRestart`
- `TestDeploymentHistoryHidesUnsafeFailureReasons`
- `TestEnvironmentWritesDuringAdmittedDeployAreRejected`
- `TestFailedWorkloadRemovalKeepsOriginalOwnershipDeployment`
- `TestPostgresRemove_CommitsSetStatusAndMarkerAtomically`
- `TestRemoveDeliveredAlsoRemovesTheApplicationsSecretDeliveryObjects`
- `TestRemoveDeliveredRefusesAWrongGenerationAndProtectsOthers`
- `TestRemoveDeliveredRetryUsesOriginalOwnerAfterRemovalSnapshotChanged`
- `TestRemoveDeliveredUsesTheClaimAndEachOriginalTarget`
- `TestRemoveWorkload_PreservesSharedDatabaseAndMarksLastReference`
- `TestUnreferencedMarking_StaysInOrganizationAndEnvironmentScope`
- `TestWorkloadRemovalAlsoRemovesItsSecretDeliveryObjects`

### `orchestrator/internal/application/pending`

- `TestDesiredPlansDraftsBeforeDeployAndTheUnchangedAppliedState`
- `TestDesiredReadsOneSnapshotAgainstConcurrentWriters`
- `TestIntentEvidenceStaysInternal`
- `TestPendingPreviewReadsOneSnapshot`
- `TestPendingPreviewReadsOneSnapshotOnPostgres`
- `TestPreviewIntentSameDefinitionKeyChangedFallbackIsUpdate`
- `TestPreviewIntentUpdateRetainUnverifiedForExistingIdentity`
- `TestRequiredRendererDeployRendersWithRealCLIAfterOutputs`
- `TestRequiredRendererMissingTieAndUnavailableFailBeforeResources`
- `TestRequiredRendererPreviewIsPurePinsAndDeployRendersAfterOutputs`

### `orchestrator/internal/application/preview`

- `TestPostgresPreview_RepeatableReadAndNoWrites`
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

### `orchestrator/internal/application/provisioning`

- `TestDeployWithUnknownTerraformOutcomeHoldsTheEnvironment`
- `TestProvision_ClusterReadinessFailureBlocksEveryConsumer`
- `TestProvision_EKSCreatedInTheSameExecutionDrivesNamespaceDatabaseAndWorkload`
- `TestProvision_EKSIsReusedNotDuplicatedByALaterDeployment`
- `TestProvision_MissingRequiredOutputBlocksTheConsumerAndIsRecorded`
- `TestProvision_PostgresOutputContractEquivalentAcrossPlacements`
- `TestProvision_ReusesEnvironmentScopedVPCAndEKS`
- `TestProvision_UsableClusterLetsConsumersRunAndIsCheckedEachDeployment`
- `TestProvision_V2ExecutorGetsPinsIdentityAndProviderInstances`
- `TestProvision_V2RuntimeSensitiveOutputsAreStoredAsReferences`

### `orchestrator/internal/application/tfexec`

- `TestDestroyUsesRetainedHistoricalReleaseNeverTheCurrentOne`

### `orchestrator/internal/bootstrap`

- `TestRealProfileAcceptsOnlyTheFullRealWiring`

### `orchestrator/test/conformance`

- `TestPlannerMatchesChallengeFixtures`
- `TestProductPlannerRejectsCriteriaLessDefinition`

### `orchestrator/test/e2e`

- `TestConfigurationDTOIsRedactedAndVariableCRUDNeverResolvesRegistry`
- `TestEnvironmentSettingsStoreSelectionConflictsAndBusyStateOverHTTP`
- `TestHTTPDeploymentEndToEnd`
- `TestHTTPDeploymentHistoryScopeFilterPlanningFailureAndSafeJSON`
- `TestHTTPDeploymentReadsOnPostgres`
- `TestHTTPDeploymentReadsRequireScopedSessionAndHideResolvedInputs`
- `TestHTTPUsesTheConfiguredRunID`
- `TestPendingDeployPartialFailureCanRetryOnlyRemainingWorkload`
- `TestPreviewRefusesIneligibleRootsWithAllowlistedReasons`
- `TestSavedUndeployedDraftIsPlannedIntoExactRequirementsBeforeAnyProbe`
- `TestStaleStateIsRejectedBeforeAnyEffect`
- `TestUC12UC16HTTPDraftFlow`

### `orchestrator/test/integration/v03run`

- `TestCleanupRefusesToSignalForeignAuxiliaryProcessIdentity`
- `TestCleanupWaitsForRunOwnedVaultProcessAndPortAbsence`
- `TestSupervisorStopsActualOwnedServiceAfterRunnerDeathAndDeadline`

### `orchestrator/test/integration/v04run`

- `TestCleanupRefusesUncertainAuxiliaryOwnershipAndRetainsRecovery`
- `TestCleanupStopsExactOwnedAuxiliaryAndVerifiesServiceAbsence`

### `orchestrator/test/integration/v2cleanup`

- `TestDeliveryCleansAppliedTargetsWithoutLegacyEnvironmentBinding`

### `orchestrator/test/integration/v2run`

- `TestTheScenarioScriptRecordsExactlyTheAssertionsTheRunnerRequires`

</details>

## Skipped full-suite checks

Six tests require explicit owned live Fleet/Kubernetes probe environments and
two require Xvfb, which the full-check containers do not provide. Their complete
skip sets are identical to main; they are not counted as passed checks. The task
did not add cloud/controller/workload deployment to its acceptance scope.

- `orchestrator/internal/adapters/delivery/fleet`: `TestFleetOwnedClusterHTTPSPublicationAndWithdrawal`.
- `orchestrator/internal/adapters/delivery/fleet`: `TestFleetTopologyKindReadOnly`.
- `orchestrator/internal/adapters/kubernetes`: `TestOpenObjectLifecycleOnKind`.
- `orchestrator/internal/adapters/kubernetes`: `TestOwnedClusterMetadataHandoffAllowsNextFleetSSA`.
- `orchestrator/internal/adapters/templatedriver`: `TestRuntimeDefaultsOnOwnedKind`.
- `orchestrator/internal/application/accountdiag`: `TestRealKindProbes`.
- `orchestrator/test/integration/v03run`: `TestCleanupUsesActualPrivateXvfbLaunchRecord`.
- `orchestrator/test/integration/v03run`: `TestSharedXvfbLauncherWorksForCallerWithoutPrivateDirectory`.
