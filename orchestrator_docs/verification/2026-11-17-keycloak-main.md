---
id: VERIFY-KEYCLOAK-MAIN-20261117
artifact: verification-record
status: current
last_reviewed: 2026-11-17
---

# Keycloak SSO — accepted main integration, issue #77

The user requested merge, then explicitly chose to keep local login on the main
Console so they can deploy Keycloak later. Merge `57ecc971` joins main
`ab37e2d4` and feature `96a51f6f`; it is pushed. Seven translation/documentation
conflicts preserve both main's accepted features and the SSO additions. The
feature branch is deleted locally and remotely. No company realm was changed.

The [feature verification](2026-11-13-keycloak-sso.md) records real Keycloak SSO,
three-role authorization and the optional personal local Admin. The main
installation deliberately remains in local mode: its rollout checks below do
not claim that SSO is enabled or verified against a main-installation provider.

## Fresh integration validation

Full checks ran on the merged product sources in a separate kind namespace,
with a fresh PostgreSQL fixture (max_locks_per_transaction=512), Redis and
Terraform requirements enabled. The same full backend command ran on pinned
main `ab37e2d4` in the same environment. No product assertion was removed.

- Full frontend: **1,077 tests / 114 files pass** with one worker; typecheck,
  lint and build pass. Two four-worker runs each failed the existing
  observability test `pauses at the latest followed interval without rewinding
  received logs`. Its five-test file passes independently on both trees;
  main's full four-worker run passes all 1,072 tests. Observability sources are
  identical between the trees. The passing serial run does not establish that
  this timing instability is fixed. Existing lint/bundle warnings remain.
- Backend build passes. Full backend remains red: **2,485 passing, 102 failing,
  8 skipped test/subtest events**; baseline has 2,451 passing, 102 failing and
  the same eight skipped events. All **68 failing top-level test names in 15
  packages** also fail on main; additional failing names: zero.
- PostgreSQL reaches the same 30-minute package timeout in
  `TestUpgradePreservesStateAndRuntimeIsolation` on both trees. Later assertions
  in that package are not claimed as executed. OIDC populated-v10 upgrade,
  repeated setup, rollback, retained identities and runtime privilege isolation
  pass both in the full run before timeout and in an independent focused run.
- The initial disposable PostgreSQL loopback fixture admitted wrong passwords;
  `TestStoreWithReservedCharacterPassword` failed on both trees. After enforcing
  password authentication in this fixture, both focused jobstore package runs
  pass. Raw full counts above retain that initial failure.
- Full auth/OIDC/HTTP race checks pass. Bootstrap's race run retains the existing
  `TestRealProfileAcceptsOnlyTheFullRealWiring` failure, also reproduced in the
  full main baseline. Bootstrap helpers: 94 pass; E2E helpers: 6 pass.
- Docs/whitespace checks, migration immutability guard and seven guard tests
  pass. PlantUML is not installed locally, so renderer syntax checking is skipped.
  Eight preexisting opt-in backend tests lack live-kind/Xvfb prerequisites;
  their skip names match main. No OIDC test is skipped.

These results are not a green full-backend gate. Raw logs, source archive,
comparison receipts and browser evidence are retained privately outside Git.

## Main rollout and preservation

Both images were built from the merged product sources and deployed as
`orchestrator-backend:main-57ecc971` and
`orchestrator-frontend:main-57ecc971`. Preflight found six visible Applications,
no active/recovering/interrupted operation, configured Git and the existing
READY default CD binding.

Writers were quiesced before a custom-format PostgreSQL backup. An isolated
restore succeeds and contains 14 persisted Application rows, including deleted
history; this is distinct from the six visible API Applications. Vault was
stopped while credential/state-key/Vault/cache storage was archived privately.
Owner setup then upgraded v10 to v11 before ordinary runtime was started.
The main database and PVCs were retained; no reset or manual marker change ran.
The backend still has no owner credential environment entry or mount.

Both deployments are Ready. Before/after comparison preserves the installation
ID, both local User IDs/organization/role/status records, all six visible
Application identities, nine Resource Types and 33 Definition versions/driver
input hashes. The restored and upgraded databases both retain 14 Application
rows. External account count stays zero. No OIDC configuration was added.

Through the exact Tailscale MagicDNS Console URL, Chromium completes local login
as the existing Platform Engineer, displays all six Application cards and reads
the real same-origin Applications/session APIs with HTTP 200. Auth configuration
reports local mode and the session reports the local provider. An initial browser
attempt while the retained forward reconnected after Pod replacement received
502; the stable post-rollout retry passes. The existing shell/navigation is
visually preserved. No test Application or workload was created by this smoke.
This access was checked from the development host, not a second tailnet device.
Exact URLs, screenshots and credentials stay in private evidence/user handoff.

## Cleanup and retained services

The temporary backup Pod was removed with matching run label, UID and
resourceVersion preconditions; its absence is verified. The completed owner
setup Job remains as the upgrade audit record. Main services, the existing
Tailscale Serve/forward and retained data remain available. Restart/exposure
instructions are in the [kind Console runbook](https://github.com/pr3s3nt/poc/blob/087b1cae896c11b3762962c3fd79bd54890446ea/deploy/kind-k8s-4f/README.md).

The dedicated Keycloak preview containers are stopped after acceptance; their
volumes and private inputs are preserved. No `down -v` or credential deletion
ran. The [SSO runbook](../operations/keycloak-sso.md) and retained private access
notes describe restarting that separate preview. Other feature stacks and user
tmux sessions are preserved.

The full-check namespace is deleted only after verifying its original UID and
run label; a successful Kubernetes lookup confirms absence. Loaded main images
remain because the retained Console uses them. No cluster or unrelated workload
was deleted. No preexisting user-owned source change was present or overwritten.

<details>
<summary>Failing top-level tests, all reproduced on pinned main</summary>

`orchestrator/examples/acceptance-app/internal/jobstore`

- `TestStoreWithReservedCharacterPassword`

`orchestrator/internal/adapters/postgres`

- `TestBaselineEnforcesCurrentConstraints`
- `TestVersionSixCapturesPopulatedFiveAndProtectsSnapshots`

`orchestrator/internal/adapters/templatesources`

- `TestInspectRejectsUnsafeSourcesWithoutFetching`

`orchestrator/internal/application/catalog`

- `TestListDefinitionCapabilities_PlatformPairsExistingClusterAndWorkload`
- `TestWorkloadRegistration_TypedPlacementInputsAndCompanions`

`orchestrator/internal/application/configuration`

- `TestRenameWithReferencesPreservesAliasesManyToOneAndOtherFields`

`orchestrator/internal/application/deployment`

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

`orchestrator/internal/application/pending`

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

`orchestrator/internal/application/preview`

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

`orchestrator/internal/application/provisioning`

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

`orchestrator/internal/application/tfexec`

- `TestDestroyUsesRetainedHistoricalReleaseNeverTheCurrentOne`

`orchestrator/internal/bootstrap`

- `TestRealProfileAcceptsOnlyTheFullRealWiring`

`orchestrator/test/conformance`

- `TestPlannerMatchesChallengeFixtures`
- `TestProductPlannerRejectsCriteriaLessDefinition`

`orchestrator/test/e2e`

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

`orchestrator/test/integration/v2cleanup`

- `TestDeliveryCleansAppliedTargetsWithoutLegacyEnvironmentBinding`

`orchestrator/test/integration/v2run`

- `TestTheScenarioScriptRecordsExactlyTheAssertionsTheRunnerRequires`

</details>

<details>
<summary>Existing skipped opt-in tests</summary>

- `TestCleanupUsesActualPrivateXvfbLaunchRecord`
- `TestFleetOwnedClusterHTTPSPublicationAndWithdrawal`
- `TestFleetTopologyKindReadOnly`
- `TestOpenObjectLifecycleOnKind`
- `TestOwnedClusterMetadataHandoffAllowsNextFleetSSA`
- `TestRealKindProbes`
- `TestRuntimeDefaultsOnOwnedKind`
- `TestSharedXvfbLauncherWorksForCallerWithoutPrivateDirectory`

</details>
