---
id: VER-2026-11-13-USER-GUIDE-ENV
artifact: verification-record
status: evidence
last_reviewed: 2026-11-13
---

# Console User Guide: environment variables — issue #82

## Scope

Product commit `ca0bb9ab`, branch `features/user-guide-env`, based on main
`cd40a792`. [GUIDE-01–03](../usecase/UC-16/user-guide-specification.md) adds an
authenticated User Guide entry for all roles and only the environment-variable
article. No backend, parser, API, Secret, workload or deployment behavior changed.
No merge or retained Console rollout performed; awaiting user acceptance.

## Focused and frontend validation

- 65 focused tests pass: routes, all-role collapsed sidebar, Developer deep-link,
  session gate and independence from Application load failure, VI/EN contents,
  examples parsed by the actual envfile parser, existing App and parser coverage.
- Local typecheck/lint/production build, documentation checker, diff and Node
  syntax checks pass. No diagrams were changed; local docs checker reports that
  PlantUML is not installed.
- Full frontend runs in a dedicated namespace on the authorized kind cluster:
  typecheck, lint, **113 files / 1069 tests**, production build pass. All fixture
  paths were included. Existing PlatformSettingsPage effect-ref warning and
  bundle chunk-size notice remain.
- Initial pod dependency installation failed with registry DNS `EAI_AGAIN` and
  npm's exit-handler error before checks ran. The lockfile-installed local Linux
  dependency tree was copied into the isolated pod; full checks then passed.
  No assertions were weakened or skipped.

## Browser and visual review

Real task Docker stack, PostgreSQL and Vault, with real session and Applications
API checks. Per user direction, the review uses Windows Tailscale Serve and its
published MagicDNS URL, proxying the task frontend. The exact URL was opened in
Chromium and is supplied only in private handoff. No request mocks were used.

`e2e/scenarios/user-guide/run.mjs` passes sidebar/Enter navigation, contents and
Secret anchor focus, synthetic marker example and independent Secret boundary,
deep-link/refresh/back/forward, active named collapsed link, VI/EN through the
language selector, 1280/1512 widths without page overflow, no browser errors or
content mutations, and anonymous deep-link sign-in gate. Synthetic instructions
were read; no Secret was created and no workload deploy was performed.

Reference: the real signed-in baseline Applications screen plus AppShell, shared
styles/tokens and Button. Final screenshots preserve shell, typography, colors,
active navigation and content panels. The guide publishes one article with a
short task contents list; no future-topic placeholders or duplicated introductory
banner. Technical explanations stay in the guide rather than expanding authoring
forms. Example values are explicitly synthetic. Screenshots, sessions and logs
remain outside Git. Initial runner setup corrections selected one Playwright
installation and matched the language selector's full accessible name; the final
runner passes with unchanged product behavior.

## Full backend and baseline comparison

All 144 package completions were captured across the full invocation and its
resume. Full backend is **not green**: 71 observed top-level test failures across
14 packages, plus a timeout-only package (15 failed packages total). PostgreSQL
and tfexec hit the 10-minute package cap, so their remaining tests are not proven
complete. Six existing opt-in kind runtime checks were skipped; no skip was added
by this task. `go build ./...` passes.

The initial host exec stream ended with status 143 while the remote test process
was still alive. The isolated run was stopped; 67 captured package completions
were preserved and the other 77 packages resumed with receipts written inside
the pod. Both logs are retained. No failed completed package was retried to erase
its result; the final union retains all observed failures and both timeouts.

Backend source tree equals main exactly:
`8ef1765aaece71644123b90f808fa46edc1aee50`. A separate baseline checkout and
PostgreSQL instance in the same isolated test namespace reproduce the failed
checks. Main was not edited to hide these failures.

A separate main run reproduces **all 71 observed top-level test failures**,
with no extra or missing top-level failure in the targeted comparison. This
comparison does not claim that complete main packages were rerun or that
the two package-level timeouts were independently reproduced. Source identity
and matched failures establish the checked boundary; they do not turn the
full backend run green.

| Package | Failed test reproduced on main |
|---|---|
| `internal/adapters/postgres` | `TestBaselineEnforcesCurrentConstraints` |
| `internal/adapters/postgres` | `TestVersionSixCapturesPopulatedFiveAndProtectsSnapshots` |
| `internal/adapters/templatesources` | `TestInspectRejectsUnsafeSourcesWithoutFetching` |
| `internal/adapters/terraform` | `TestAWSModulesInitValidateReadonlyLock` |
| `internal/adapters/terraform` | `TestAWSModulesPlanPolicyWithMockProvider` |
| `internal/adapters/terraform` | `TestKubernetesModulesInitValidateReadonlyLock` |
| `internal/adapters/terraform` | `TestKubernetesStatefulModulesPlan` |
| `internal/adapters/terraform` | `TestNamespaceModulePlan` |
| `internal/application/catalog` | `TestListDefinitionCapabilities_PlatformPairsExistingClusterAndWorkload` |
| `internal/application/catalog` | `TestWorkloadRegistration_TypedPlacementInputsAndCompanions` |
| `internal/application/configuration` | `TestRenameWithReferencesPreservesAliasesManyToOneAndOtherFields` |
| `internal/application/deployment` | `TestCredentialBackedTarget_RemovalAfterRestart` |
| `internal/application/deployment` | `TestDeploymentHistoryHidesUnsafeFailureReasons` |
| `internal/application/deployment` | `TestEnvironmentWritesDuringAdmittedDeployAreRejected` |
| `internal/application/deployment` | `TestFailedWorkloadRemovalKeepsOriginalOwnershipDeployment` |
| `internal/application/deployment` | `TestPostgresRemove_CommitsSetStatusAndMarkerAtomically` |
| `internal/application/deployment` | `TestRemoveDeliveredAlsoRemovesTheApplicationsSecretDeliveryObjects` |
| `internal/application/deployment` | `TestRemoveDeliveredRefusesAWrongGenerationAndProtectsOthers` |
| `internal/application/deployment` | `TestRemoveDeliveredRetryUsesOriginalOwnerAfterRemovalSnapshotChanged` |
| `internal/application/deployment` | `TestRemoveDeliveredUsesTheClaimAndEachOriginalTarget` |
| `internal/application/deployment` | `TestRemoveWorkload_PreservesSharedDatabaseAndMarksLastReference` |
| `internal/application/deployment` | `TestUnreferencedMarking_StaysInOrganizationAndEnvironmentScope` |
| `internal/application/deployment` | `TestWorkloadRemovalAlsoRemovesItsSecretDeliveryObjects` |
| `internal/application/pending` | `TestDesiredPlansDraftsBeforeDeployAndTheUnchangedAppliedState` |
| `internal/application/pending` | `TestDesiredReadsOneSnapshotAgainstConcurrentWriters` |
| `internal/application/pending` | `TestIntentEvidenceStaysInternal` |
| `internal/application/pending` | `TestPendingPreviewReadsOneSnapshot` |
| `internal/application/pending` | `TestPendingPreviewReadsOneSnapshotOnPostgres` |
| `internal/application/pending` | `TestPreviewIntentSameDefinitionKeyChangedFallbackIsUpdate` |
| `internal/application/pending` | `TestPreviewIntentUpdateRetainUnverifiedForExistingIdentity` |
| `internal/application/pending` | `TestRequiredRendererDeployRendersWithRealCLIAfterOutputs` |
| `internal/application/pending` | `TestRequiredRendererMissingTieAndUnavailableFailBeforeResources` |
| `internal/application/pending` | `TestRequiredRendererPreviewIsPurePinsAndDeployRendersAfterOutputs` |
| `internal/application/preview` | `TestPostgresPreview_RepeatableReadAndNoWrites` |
| `internal/application/preview` | `TestPreview_BaselineEvidenceMissingIsUnverifiedAndStoreFailureFails` |
| `internal/application/preview` | `TestPreview_BaselinePlansAreCapturedInsideTheSnapshot` |
| `internal/application/preview` | `TestPreview_CatalogErrorsNeverEchoed` |
| `internal/application/preview` | `TestPreview_DeltaInvariantAndContainerResources` |
| `internal/application/preview` | `TestPreview_MatchesDeployPlan` |
| `internal/application/preview` | `TestPreview_NoChangeHasEmptyDelta` |
| `internal/application/preview` | `TestPreview_NoRuntimeMutation` |
| `internal/application/preview` | `TestPreview_PublicViewRedactsWithoutChangingPlan` |
| `internal/application/preview` | `TestPreview_ReadsOneConsistentSnapshot` |
| `internal/application/preview` | `TestPreview_StrictQuery` |
| `internal/application/provisioning` | `TestDeployWithUnknownTerraformOutcomeHoldsTheEnvironment` |
| `internal/application/provisioning` | `TestProvision_ClusterReadinessFailureBlocksEveryConsumer` |
| `internal/application/provisioning` | `TestProvision_EKSCreatedInTheSameExecutionDrivesNamespaceDatabaseAndWorkload` |
| `internal/application/provisioning` | `TestProvision_EKSIsReusedNotDuplicatedByALaterDeployment` |
| `internal/application/provisioning` | `TestProvision_MissingRequiredOutputBlocksTheConsumerAndIsRecorded` |
| `internal/application/provisioning` | `TestProvision_PostgresOutputContractEquivalentAcrossPlacements` |
| `internal/application/provisioning` | `TestProvision_ReusesEnvironmentScopedVPCAndEKS` |
| `internal/application/provisioning` | `TestProvision_UsableClusterLetsConsumersRunAndIsCheckedEachDeployment` |
| `internal/application/provisioning` | `TestProvision_V2ExecutorGetsPinsIdentityAndProviderInstances` |
| `internal/application/provisioning` | `TestProvision_V2RuntimeSensitiveOutputsAreStoredAsReferences` |
| `internal/bootstrap` | `TestRealProfileAcceptsOnlyTheFullRealWiring` |
| `test/conformance` | `TestPlannerMatchesChallengeFixtures` |
| `test/conformance` | `TestProductPlannerRejectsCriteriaLessDefinition` |
| `test/e2e` | `TestConfigurationDTOIsRedactedAndVariableCRUDNeverResolvesRegistry` |
| `test/e2e` | `TestEnvironmentSettingsStoreSelectionConflictsAndBusyStateOverHTTP` |
| `test/e2e` | `TestHTTPDeploymentEndToEnd` |
| `test/e2e` | `TestHTTPDeploymentHistoryScopeFilterPlanningFailureAndSafeJSON` |
| `test/e2e` | `TestHTTPDeploymentReadsOnPostgres` |
| `test/e2e` | `TestHTTPDeploymentReadsRequireScopedSessionAndHideResolvedInputs` |
| `test/e2e` | `TestHTTPUsesTheConfiguredRunID` |
| `test/e2e` | `TestPendingDeployPartialFailureCanRetryOnlyRemainingWorkload` |
| `test/e2e` | `TestPreviewRefusesIneligibleRootsWithAllowlistedReasons` |
| `test/e2e` | `TestSavedUndeployedDraftIsPlannedIntoExactRequirementsBeforeAnyProbe` |
| `test/e2e` | `TestStaleStateIsRejectedBeforeAnyEffect` |
| `test/e2e` | `TestUC12UC16HTTPDraftFlow` |
| `test/integration/v2cleanup` | `TestDeliveryCleansAppliedTargetsWithoutLegacyEnvironmentBinding` |
| `test/integration/v2run` | `TestTheScenarioScriptRecordsExactlyTheAssertionsTheRunnerRequires` |

Existing runtime-gated skips (not runtime acceptance evidence):

- `TestCleanupUsesActualPrivateXvfbLaunchRecord`.
- `TestFleetOwnedClusterHTTPSPublicationAndWithdrawal`.
- `TestFleetTopologyKindReadOnly`.
- `TestOpenObjectLifecycleOnKind`.
- `TestOwnedClusterMetadataHandoffAllowsNextFleetSSA`.
- `TestRealKindProbes`.
- `TestRuntimeDefaultsOnOwnedKind`.
- `TestSharedXvfbLauncherWorksForCallerWithoutPrivateDirectory`.

## Cleanup and retained review data

The check namespace was deleted after validating its recorded UID, ownership
label and all contained pod labels, with UID/resourceVersion delete preconditions.
A successful API lookup confirmed namespace absence. Both candidate and baseline
PostgreSQL/test pods were removed. No retained platform workload was deployed,
changed or removed.

The isolated feature Docker stack and its database/Vault/cache volumes remain
for user acceptance. Windows Serve forwards to the feature frontend; restart
script and private test password-file location are in the private handoff. No
user-owned changes existed at task start, and no unrelated stack/session/data
was removed. The public issue contains no private URLs or infrastructure details.
