---
id: VERIFY-2026-11-13-KEYCLOAK-SSO
artifact: verification-record
status: current
last_reviewed: 2026-11-16
---

# Keycloak SSO and personal local test Admin — issue #77

Product candidate `9856613c`, branch `features/keycloak`, based on main
`9d80dbe4`. This is a feature preview awaiting acceptance, not a main rollout or
company Keycloak integration. The user explicitly requested a personal local
login on the same Console while every employee continues through Keycloak.

## Product checks

Real dedicated Keycloak 26.4.7 and PostgreSQL browser/API acceptance passed for
DEVELOPER, PLATFORM_ENGINEER and ADMIN, missing-role rejection, backend/UI role
gates, existing-session promotion/demotion, disabled employees and logout.
The personal local Admin uses the same Console without any provider request;
other local accounts are denied and employee-management APIs remain blocked.
Its own password form is available. A deliberately incorrect current-password
request verifies local password validation without changing the real password.
Temporary test role/enablement mutations are restored and verified through the
Keycloak API, and browser sessions are signed out in cleanup.

Issuer/subject identities, opaque HttpOnly sessions and client-scoped role mapping
are exercised. Backend restart invalidates old external sessions; SSO login again
preserves the User ID. Provider credentials are only in backend memory. This is a
single-backend slice; durable distributed provider sessions/HA are not implemented.
Local login is an explicitly configured single ACTIVE Admin in the pinned
Organization, disabled by default; it is not employee local-account provisioning.

Schema version 11 populated-v10 upgrade, repeated execution, rollback, identity
preservation and runtime privilege isolation pass focused real PostgreSQL tests.
Older released migrations/references are unchanged. Auth/OIDC/HTTP/config focused
race tests and frontend auth/shell tests passed. The scoped dependency scan found
GO-2026-4945 in the initially selected JOSE version; it was upgraded to 4.1.4.
The final auth/OIDC scan reports zero reachable known vulnerabilities, with other
non-reachable imported/required-module findings retained in private logs. This is
not a security audit of the whole platform.

## Full verification

The previous runner pods stopped when the cluster restarted; their final log files
in ephemeral storage were lost. Saved baseline and browser evidence was retained.
Full checks were therefore restarted on the unchanged product commit in the
same run-owned namespace, with logs streamed to durable private host storage.
A first resumed backend attempt could not fetch three new Go modules because
cluster DNS failed; it was stopped and is explicitly incomplete. The checksum
verified module cache was copied to the runner, offline dependency resolution and
build passed, and a new complete run was started. No test assertion was removed.

Full run completed on 2026-11-13; durable evidence was checked again on
2026-11-16. Backend build passed. Full backend remains red: **2,506 passing,
105 failing and 8 skipped test/subtest result events** across the suite. There
are 71 failing top-level tests in 15 packages; every failing top-level test also
fails on the pinned main baseline `9d80dbe4`. Additional failing test names: zero.
This is not a green full-suite gate. No baseline failure was waived or converted
to a passing test.

The eight skips are existing opt-in live-cluster/Xvfb tests with prerequisites
not supplied to these namespace-contained runners, matching the main baseline.
PostgreSQL, Redis and Terraform requirements were enabled. No new auth/OIDC
test was skipped. PostgreSQL fixtures used max_locks_per_transaction=512 to
avoid the shared-lock exhaustion reproduced in earlier concurrent checks.
The v11 migration tests, concurrent fresh setup and schema-drift checks pass
on this fixture. Focused main tests on the same fixture reproduce the historical
subdomain-column, template_sources fingerprint, pg_restore and upgraded-deploy
failures; they are not counted as v11 upgrade success.

Full frontend: **1,039 tests in 109 files pass**, with typecheck, lint and build.
The existing PlatformSettingsPage lint warning and bundle-size warning remain.
Bootstrap helper tests (94), E2E helper tests (6), docs checker, diff check and
migration guard/seven guard tests pass. PlantUML is unavailable; renderer syntax
validation was not run, while the documentation checker completed.

<details>
<summary>Failing backend tests, all reproduced on main</summary>

`orchestrator/internal/adapters/postgres`

- `TestBaselineEnforcesCurrentConstraints`
- `TestUpgradeBackupRestore`
- `TestUpgradeHTTPLoginPreviewDeploy`
- `TestVersionSevenPreservesConnectionsAndAdmitsGit`
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

`orchestrator/test/integration/v03run`

- `TestBrowserLeaderExitKeepsReceiptUntilOrphanGroupIsReaped`

`orchestrator/test/integration/v2cleanup`

- `TestDeliveryCleansAppliedTargetsWithoutLegacyEnvironmentBinding`

`orchestrator/test/integration/v2run`

- `TestTheScenarioScriptRecordsExactlyTheAssertionsTheRunnerRequires`

</details>

<details>
<summary>Existing skipped opt-in tests</summary>

- `TestFleetOwnedClusterHTTPSPublicationAndWithdrawal`
- `TestFleetTopologyKindReadOnly`
- `TestOwnedClusterMetadataHandoffAllowsNextFleetSSA`
- `TestOpenObjectLifecycleOnKind`
- `TestRuntimeDefaultsOnOwnedKind`
- `TestRealKindProbes`
- `TestCleanupUsesActualPrivateXvfbLaunchRecord`
- `TestSharedXvfbLauncherWorksForCallerWithoutPrivateDirectory`

</details>

## UI and access

Reference was the existing Console SignInPage, AppShell, shared Button,
PasswordInput, LanguageSelect, field/error styles and shared UI design. The old
login was opened in Chromium before modification. The final form preserves SSO
as primary and places existing local fields in one disclosure. No new shell or
employee-management flow was added. The content review found the disclosure
label, fields and action sufficient; policy detail stays in the runbook.

Both login paths and their real API sessions were verified with Chromium through
the exact Tailscale feature URL; the complete real-provider scenario and local
Console login were rechecked at handoff on 2026-11-16. A changed authenticated Tailscale node address
made the old URL unavailable on resume. A separate task-owned preview was
prepared at the current address; old database/Keycloak volumes remain intact,
with their frontend and Keycloak stopped to avoid duplicate proxy routers.
No old-account migration, retained-data reset or company realm change was made.
Access was checked from the development host, not from a second tailnet device.
Actual infrastructure URLs, passwords, complete logs and browser images stay in
private evidence and the user handoff, not public issues or this repository.

The current preview stack, feature proxy/forward and volumes are retained for
acceptance. Restart/lifecycle instructions are in the
[runbook](../operations/keycloak-sso.md). Private access notes record the current
Compose project/domain and password-file paths. The unwanted separate local-only
preview was removed after the same-Console clarification.

The temporary kind namespace was deleted on 2026-11-16 after verifying its
expected UID and issue/run ownership labels. A successful Kubernetes API lookup
with ignore-not-found confirmed namespace absence. The cluster, shared services
and feature preview were retained. Cleanup receipt and the complete test logs
remain in private evidence.

No preexisting user changes were present or overwritten. No other feature stack,
user tmux session, shared cluster service or current kube context was changed.
Issue remains open and the branch remains unmerged. Full-suite failures are not
reported as a pass; acceptance and required fresh checks precede any later merge.
