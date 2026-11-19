---
id: VERIFY-20261118-INTEGRATED-ROUTES-EDITOR
artifact: verification-record
status: current
last_reviewed: 2026-11-18
---

# Integrated Gateway/routes and Definition editor — issues #84/#87

The user approved combining the two trial branches before acceptance. Merge
`35cbc638` joins `features/definition-code-editor` at `96d59826` into
`features/Httproute`, incorporating main `725541ef`. It does not merge either
feature into main. Earlier isolated verification records retain their original
source attribution; they are not combined-run evidence.

## Integration and focused checks

Gateway Echo/legacy hydration is inside the shared editor's hydrate callback;
opening establishes the editor context first. Reset clears both editor and
Gateway/reference state. The Gateway heading/class controls and the code editor's
busy navigation guard remain. English/Vietnamese message catalogs were merged
as dictionaries with matching shared values and no duplicate keys.

32 focused Definition editor/Endpoint/mapping tests pass, including three new
integration cases for Echo Gateway, legacy Gateway and reference-only Endpoint.
Focused planning/catalog/provisioning/Kubernetes/template-driver Go packages and
Go build pass. Frontend typecheck/lint/build, Node syntax, documentation checker
and whitespace checks pass. Existing PlatformSettingsPage lint and bundle-size
warnings remain. No diagram changed; local PlantUML is unavailable.

## Preserved trial installation

The existing task Console is rebuilt from the merged source with its own Docker
PostgreSQL/Vault volumes. Two private custom-format database backups were checked
with pg_restore's archive listing while the old backend writer was stopped, before
owner setup. The normal released owner upgrade advances schema v10→v11; runtime
does not migrate. Installation, Organization, local user, connection, Application
and active-resource identities/status/versions match the selective pre-upgrade
snapshot. Authenticated catalog/application API responses match exactly before
any deliberate edits. Existing password still authenticates. No released
migration, marker, checksum or Terraform state was modified.

Native checks after upgrade confirm the existing one PostgreSQL/PVC is shared by
both workloads, authenticated SELECT 1 and both init clients succeed, two routes
have the expected hosts/Services/parents/listener and Accepted/ResolvedRefs, and
recorded resource/Gateway UIDs remain unchanged. No shared Gateway, Traefik,
cluster, DNS or TLS objects are changed. The separate #87 sample stack is retained.

## Integrated browser editor

The browser opens the exact Tailscale Console URL, authenticates and reads real
catalog APIs. For legacy Gateway/Endpoint it round-trips form→YAML→form read-only,
retaining driver, reference class, values and version. For the owned Echo review
Gateway and reference Endpoint it edits the display name in the form, confirms
JSON, supplies YAML, returns to the form and saves, then saves JSON. Each Save
adds exactly one proposed revision. Driver/account/input pins, criteria,
provision and active revision remain unchanged; no new Definitions are created.
Other Definitions and all Applications remain identical; no deploy request occurs
in the isolated editor check. Echo/Endpoint names now identify the review pair.

The first attempt completed all four round-trips/Saves but its final Application
snapshot assertion detected draftVersion changing because a separate workload
update harness had been started concurrently. This was a verification scheduling
error. Its failed log/receipt are retained. After workload authoring finished,
the complete editor check ran alone and passed, including the unchanged full
Application snapshot. The two review Definitions consequently have four proposed
name-only revisions, latest version 5, with their original active revision and
running Environment pins preserved. Legacy versions remain 1.

A second read-only browser check covers the actual cluster, Terraform namespace,
workload template and Terraform PostgreSQL Definitions on the integrated Console.
Form→JSON→YAML→form→JSON preserves the full document, including source/account
pins, and exact catalog/Application snapshots remain unchanged. No Save, new
fixture, version activation or deployment is performed by that check.

UI references are the existing signed-in Definition editor, namespace/provider
forms and AppShell/shared styles. Visual inspection confirms class selectors,
collapsed technical details and full-width code panel coexist with the existing
shell/spacing. Optional help remains collapsed; no additional explanation banner,
shell redesign or navigation change was introduced. Private screenshots preserve
both forms and code. Access was tested from this host through the actual tailnet
URL; this is not evidence for every other device's ACL.

## Runtime update and full-suite checks

Both native and Score workload updates were authored before one Environment
Preview/Deploy; it succeeded for both. SQL/init/route checks passed afterward.
The first native Pod restart was rejected by Kubernetes because the recorded
resourceVersion changed during ownership verification; no deletion bypassed the
precondition. After re-reading the stable Pod and rechecking the ownership chain,
the next guarded attempt succeeded. Replacement readiness, SQL/init/routes and
preserved PostgreSQL/PVC/Gateway/Deployment/route UIDs passed again. Browser
confirmed the exact Console Application and both real nginx web URLs over
Tailscale. The review Application and its data remain available for acceptance;
this update/restart is not a new create/delete lifecycle claim. Earlier cleanup
evidence stays attributed to the previous source.

Fresh full candidate `35cbc638` and main `725541ef` checks completed on the
explicitly authorized kind cluster, each with independent PostgreSQL 16,
REQUIRE_POSTGRES/REQUIRE_TERRAFORM, Go 1.25.14 and Terraform 1.9.8. Sources were
exact Git archives; later E2E/docs commits leave backend/frontend trees identical.
Both Go builds pass. Full frontend on Node 22.23.4 Bookworm passes
**116 files / 1102 tests**, with install/typecheck/lint/build all 0.

| Backend result | Candidate | Main |
|---|---:|---:|
| Passing top-level tests | 1603 | 1590 |
| Failing top-level tests | 103 | 103 |
| Passing test/subtest events | 2530 | 2478 |
| Failing test/subtest events | 143 | 143 |
| Skipped test/subtest events | 8 | 8 |
| Failed packages | 20 | 20 |

The complete failure and skip sets match exactly. No candidate-only or main-only
failure/skip, and no missing main passing top-level test. The 13 added passing
top-level tests cover namespace inheritance, Gateway Echo/legacy validation,
preexisting Gateway verification and template HTTPRoute intent. This execution
has no test timeout. All existing failures remain failures; this is a
baseline-red full-backend result, not a green gate or an assertion that failed
tests completed all their assertions. No fixture or assertion was relaxed.

The disposable namespace was deleted with owner/UID/resourceVersion conditions;
its absence was confirmed and the receipt records cleanup=true. Review stacks,
apps, data and access forwards remain running. Both issues remain open pending
user acceptance; no merge to main or retained-main rollout was performed.

## Identical backend failure inventory

| Package | Failing top-level tests |
|---|---:|
| `orchestrator/examples/acceptance-app/backend` | 10 |
| `orchestrator/examples/acceptance-app/internal/jobcache` | 14 |
| `orchestrator/examples/acceptance-app/internal/jobstore` | 1 |
| `orchestrator/examples/acceptance-app/worker` | 3 |
| `orchestrator/internal/adapters/postgres` | 5 |
| `orchestrator/internal/adapters/templatesources` | 1 |
| `orchestrator/internal/application/catalog` | 2 |
| `orchestrator/internal/application/configuration` | 1 |
| `orchestrator/internal/application/deployment` | 12 |
| `orchestrator/internal/application/pending` | 10 |
| `orchestrator/internal/application/preview` | 11 |
| `orchestrator/internal/application/provisioning` | 10 |
| `orchestrator/internal/application/tfexec` | 1 |
| `orchestrator/internal/bootstrap` | 1 |
| `orchestrator/test/conformance` | 2 |
| `orchestrator/test/e2e` | 12 |
| `orchestrator/test/integration/v03run` | 3 |
| `orchestrator/test/integration/v04run` | 2 |
| `orchestrator/test/integration/v2cleanup` | 1 |
| `orchestrator/test/integration/v2run` | 1 |

<details>
<summary>Exact top-level failures, identical on candidate and main</summary>

- `orchestrator/examples/acceptance-app/backend TestBackendBadRequestsDoNotTouchTheCache`
- `orchestrator/examples/acceptance-app/backend TestBackendCacheHitMissRefillThroughTheAPI`
- `orchestrator/examples/acceptance-app/backend TestBackendDeniedCacheOperationFailsDiagnosticsWhilePingAndProbeWork`
- `orchestrator/examples/acceptance-app/backend TestBackendHealthyLookupOfOtherJobDoesNotHideDeniedJobs`
- `orchestrator/examples/acceptance-app/backend TestBackendPingAllowedButProbeDeniedFails`
- `orchestrator/examples/acceptance-app/backend TestBackendPooledRedisAuthConnectionsRequireScopedKillAndRecoverFromPostgres`
- `orchestrator/examples/acceptance-app/backend TestBackendRedisOutageFailsDiagnosticsButKeepsDurableResult`
- `orchestrator/examples/acceptance-app/backend TestBackendUnmarkedCompletedJobIsNotVerifiedUntilMarked`
- `orchestrator/examples/acceptance-app/backend TestBackendWorkerWriteFailureStaysVisibleAfterRedisRecovers`
- `orchestrator/examples/acceptance-app/backend TestBackendWrongRedisPasswordCannotPass`
- `orchestrator/examples/acceptance-app/internal/jobcache TestCacheEntryExpiresToMiss`
- `orchestrator/examples/acceptance-app/internal/jobcache TestCacheFailuresAreClassifiedAndNeverMisses`
- `orchestrator/examples/acceptance-app/internal/jobcache TestCacheOutageIsUnreachableNotMiss`
- `orchestrator/examples/acceptance-app/internal/jobcache TestCachePutGetAuthenticatedWithDeterministicTTL`
- `orchestrator/examples/acceptance-app/internal/jobcache TestCacheTLSClientToPlainServerFails`
- `orchestrator/examples/acceptance-app/internal/jobcache TestCacheTLSRejectsWrongHostname`
- `orchestrator/examples/acceptance-app/internal/jobcache TestCacheTLSVerifiesChainAndHostname`
- `orchestrator/examples/acceptance-app/internal/jobcache TestLookupCompletedFailsFastWhenCacheIsDown`
- `orchestrator/examples/acceptance-app/internal/jobcache TestResolverCacheOutageServesDurableResultButReportsFailure`
- `orchestrator/examples/acceptance-app/internal/jobcache TestResolverCorruptEntryIsOverwrittenFromDatabase`
- `orchestrator/examples/acceptance-app/internal/jobcache TestResolverHitThenExpiredMissReadsSameRowAndRefills`
- `orchestrator/examples/acceptance-app/internal/jobcache TestResolverInvalidBindingReportsConfigFailure`
- `orchestrator/examples/acceptance-app/internal/jobcache TestResolverPendingIsNeverFrozenByTheCache`
- `orchestrator/examples/acceptance-app/internal/jobcache TestResolverWrongPasswordIsFailureNotMiss`
- `orchestrator/examples/acceptance-app/internal/jobstore TestStoreWithReservedCharacterPassword`
- `orchestrator/examples/acceptance-app/worker TestWaitForCacheFailsClosed`
- `orchestrator/examples/acceptance-app/worker TestWorkerCacheFailureAfterCommitKeepsOneCompletedJob`
- `orchestrator/examples/acceptance-app/worker TestWorkerCommitsToPostgreSQLThenWritesThroughRedis`
- `orchestrator/internal/adapters/postgres TestBaselineEnforcesCurrentConstraints`
- `orchestrator/internal/adapters/postgres TestUpgradeBackupRestore`
- `orchestrator/internal/adapters/postgres TestUpgradeHTTPLoginPreviewDeploy`
- `orchestrator/internal/adapters/postgres TestVersionSevenPreservesConnectionsAndAdmitsGit`
- `orchestrator/internal/adapters/postgres TestVersionSixCapturesPopulatedFiveAndProtectsSnapshots`
- `orchestrator/internal/adapters/templatesources TestInspectRejectsUnsafeSourcesWithoutFetching`
- `orchestrator/internal/application/catalog TestListDefinitionCapabilities_PlatformPairsExistingClusterAndWorkload`
- `orchestrator/internal/application/catalog TestWorkloadRegistration_TypedPlacementInputsAndCompanions`
- `orchestrator/internal/application/configuration TestRenameWithReferencesPreservesAliasesManyToOneAndOtherFields`
- `orchestrator/internal/application/deployment TestCredentialBackedTarget_RemovalAfterRestart`
- `orchestrator/internal/application/deployment TestDeploymentHistoryHidesUnsafeFailureReasons`
- `orchestrator/internal/application/deployment TestEnvironmentWritesDuringAdmittedDeployAreRejected`
- `orchestrator/internal/application/deployment TestFailedWorkloadRemovalKeepsOriginalOwnershipDeployment`
- `orchestrator/internal/application/deployment TestPostgresRemove_CommitsSetStatusAndMarkerAtomically`
- `orchestrator/internal/application/deployment TestRemoveDeliveredAlsoRemovesTheApplicationsSecretDeliveryObjects`
- `orchestrator/internal/application/deployment TestRemoveDeliveredRefusesAWrongGenerationAndProtectsOthers`
- `orchestrator/internal/application/deployment TestRemoveDeliveredRetryUsesOriginalOwnerAfterRemovalSnapshotChanged`
- `orchestrator/internal/application/deployment TestRemoveDeliveredUsesTheClaimAndEachOriginalTarget`
- `orchestrator/internal/application/deployment TestRemoveWorkload_PreservesSharedDatabaseAndMarksLastReference`
- `orchestrator/internal/application/deployment TestUnreferencedMarking_StaysInOrganizationAndEnvironmentScope`
- `orchestrator/internal/application/deployment TestWorkloadRemovalAlsoRemovesItsSecretDeliveryObjects`
- `orchestrator/internal/application/pending TestDesiredPlansDraftsBeforeDeployAndTheUnchangedAppliedState`
- `orchestrator/internal/application/pending TestDesiredReadsOneSnapshotAgainstConcurrentWriters`
- `orchestrator/internal/application/pending TestIntentEvidenceStaysInternal`
- `orchestrator/internal/application/pending TestPendingPreviewReadsOneSnapshot`
- `orchestrator/internal/application/pending TestPendingPreviewReadsOneSnapshotOnPostgres`
- `orchestrator/internal/application/pending TestPreviewIntentSameDefinitionKeyChangedFallbackIsUpdate`
- `orchestrator/internal/application/pending TestPreviewIntentUpdateRetainUnverifiedForExistingIdentity`
- `orchestrator/internal/application/pending TestRequiredRendererDeployRendersWithRealCLIAfterOutputs`
- `orchestrator/internal/application/pending TestRequiredRendererMissingTieAndUnavailableFailBeforeResources`
- `orchestrator/internal/application/pending TestRequiredRendererPreviewIsPurePinsAndDeployRendersAfterOutputs`
- `orchestrator/internal/application/preview TestPostgresPreview_RepeatableReadAndNoWrites`
- `orchestrator/internal/application/preview TestPreview_BaselineEvidenceMissingIsUnverifiedAndStoreFailureFails`
- `orchestrator/internal/application/preview TestPreview_BaselinePlansAreCapturedInsideTheSnapshot`
- `orchestrator/internal/application/preview TestPreview_CatalogErrorsNeverEchoed`
- `orchestrator/internal/application/preview TestPreview_DeltaInvariantAndContainerResources`
- `orchestrator/internal/application/preview TestPreview_MatchesDeployPlan`
- `orchestrator/internal/application/preview TestPreview_NoChangeHasEmptyDelta`
- `orchestrator/internal/application/preview TestPreview_NoRuntimeMutation`
- `orchestrator/internal/application/preview TestPreview_PublicViewRedactsWithoutChangingPlan`
- `orchestrator/internal/application/preview TestPreview_ReadsOneConsistentSnapshot`
- `orchestrator/internal/application/preview TestPreview_StrictQuery`
- `orchestrator/internal/application/provisioning TestDeployWithUnknownTerraformOutcomeHoldsTheEnvironment`
- `orchestrator/internal/application/provisioning TestProvision_ClusterReadinessFailureBlocksEveryConsumer`
- `orchestrator/internal/application/provisioning TestProvision_EKSCreatedInTheSameExecutionDrivesNamespaceDatabaseAndWorkload`
- `orchestrator/internal/application/provisioning TestProvision_EKSIsReusedNotDuplicatedByALaterDeployment`
- `orchestrator/internal/application/provisioning TestProvision_MissingRequiredOutputBlocksTheConsumerAndIsRecorded`
- `orchestrator/internal/application/provisioning TestProvision_PostgresOutputContractEquivalentAcrossPlacements`
- `orchestrator/internal/application/provisioning TestProvision_ReusesEnvironmentScopedVPCAndEKS`
- `orchestrator/internal/application/provisioning TestProvision_UsableClusterLetsConsumersRunAndIsCheckedEachDeployment`
- `orchestrator/internal/application/provisioning TestProvision_V2ExecutorGetsPinsIdentityAndProviderInstances`
- `orchestrator/internal/application/provisioning TestProvision_V2RuntimeSensitiveOutputsAreStoredAsReferences`
- `orchestrator/internal/application/tfexec TestDestroyUsesRetainedHistoricalReleaseNeverTheCurrentOne`
- `orchestrator/internal/bootstrap TestRealProfileAcceptsOnlyTheFullRealWiring`
- `orchestrator/test/conformance TestPlannerMatchesChallengeFixtures`
- `orchestrator/test/conformance TestProductPlannerRejectsCriteriaLessDefinition`
- `orchestrator/test/e2e TestConfigurationDTOIsRedactedAndVariableCRUDNeverResolvesRegistry`
- `orchestrator/test/e2e TestEnvironmentSettingsStoreSelectionConflictsAndBusyStateOverHTTP`
- `orchestrator/test/e2e TestHTTPDeploymentEndToEnd`
- `orchestrator/test/e2e TestHTTPDeploymentHistoryScopeFilterPlanningFailureAndSafeJSON`
- `orchestrator/test/e2e TestHTTPDeploymentReadsOnPostgres`
- `orchestrator/test/e2e TestHTTPDeploymentReadsRequireScopedSessionAndHideResolvedInputs`
- `orchestrator/test/e2e TestHTTPUsesTheConfiguredRunID`
- `orchestrator/test/e2e TestPendingDeployPartialFailureCanRetryOnlyRemainingWorkload`
- `orchestrator/test/e2e TestPreviewRefusesIneligibleRootsWithAllowlistedReasons`
- `orchestrator/test/e2e TestSavedUndeployedDraftIsPlannedIntoExactRequirementsBeforeAnyProbe`
- `orchestrator/test/e2e TestStaleStateIsRejectedBeforeAnyEffect`
- `orchestrator/test/e2e TestUC12UC16HTTPDraftFlow`
- `orchestrator/test/integration/v03run TestCleanupRefusesToSignalForeignAuxiliaryProcessIdentity`
- `orchestrator/test/integration/v03run TestCleanupWaitsForRunOwnedVaultProcessAndPortAbsence`
- `orchestrator/test/integration/v03run TestSupervisorStopsActualOwnedServiceAfterRunnerDeathAndDeadline`
- `orchestrator/test/integration/v04run TestCleanupRefusesUncertainAuxiliaryOwnershipAndRetainsRecovery`
- `orchestrator/test/integration/v04run TestCleanupStopsExactOwnedAuxiliaryAndVerifiesServiceAbsence`
- `orchestrator/test/integration/v2cleanup TestDeliveryCleansAppliedTargetsWithoutLegacyEnvironmentBinding`
- `orchestrator/test/integration/v2run TestTheScenarioScriptRecordsExactlyTheAssertionsTheRunnerRequires`

</details>

Skipped test/subtest events (same eight on both):

- `orchestrator/internal/adapters/delivery/fleet TestFleetOwnedClusterHTTPSPublicationAndWithdrawal`
- `orchestrator/internal/adapters/delivery/fleet TestFleetTopologyKindReadOnly`
- `orchestrator/internal/adapters/kubernetes TestOpenObjectLifecycleOnKind`
- `orchestrator/internal/adapters/kubernetes TestOwnedClusterMetadataHandoffAllowsNextFleetSSA`
- `orchestrator/internal/adapters/templatedriver TestRuntimeDefaultsOnOwnedKind`
- `orchestrator/internal/application/accountdiag TestRealKindProbes`
- `orchestrator/test/integration/v03run TestCleanupUsesActualPrivateXvfbLaunchRecord`
- `orchestrator/test/integration/v03run TestSharedXvfbLauncherWorksForCallerWithoutPrivateDirectory`

## User-approved test catalog cleanup

After handoff the user authorized deletion of all test Applications/data while
keeping the current Gateway/Endpoint pair. At cleanup entry all five task
Applications were already DELETED, no Environment operation was claimed, and all
12 runtime records linked to the six obsolete Definitions were UNREFERENCED.
There were no Terraform instance references to those 12 records. Native reads
confirmed no remaining task Application namespaces/PVs and unchanged shared
Gateway UID. This cleanup did not perform Application deletion or mutate Kubernetes.

The task backend writer was stopped and a new private custom-format PostgreSQL
backup was checked with pg_restore's archive listing. The product has no
Definition delete API. One bounded owner transaction removed the six explicitly
listed obsolete test Definition rows and 12 retired runtime records. It cleared
46 nullable deployment-resource links to those retired records, preserving the
historical deployment rows and their Definition keys/input/output snapshots.
Foreign keys and triggers stayed enabled. The transaction asserted all remaining
Definitions, all immutable revision snapshots and all revision policies were
unchanged. No schema/migration, Terraform state, credential or immutable revision
was edited/deleted. Backup, SQL plan, transaction log and receipts remain private.

The retained pair is `httproute84-mwkl4ysg-gateway` (Echo) and
`httproute84-mwkl4ysg-endpoint`. Because its original test Application is deleted,
the existing Console YAML/JSON editor changed only each matching criterion from
that Application ID to the default empty criterion. Save created revision 6;
the existing Versions UI promoted each r6 with generation-CAS. Driver, account,
reference values and all infrastructure inputs stayed unchanged. This makes the
single pair usable by new test Applications. All four cluster/namespace/workload/
PostgreSQL Definitions remain API-identical; no new Application or Definition was
created. Immutable old revisions/policies remain as historical records for the
removed test families; those families no longer appear in the selectable catalog.

A first browser login after backend restart saw HTTP 502 because the existing
frontend proxy retained its previous backend address. Restarting only the task
frontend refreshed that connection. The next complete browser check passed:
real API data, exactly six catalog cards including one Gateway and one Endpoint,
both revisions promoted, no active Applications. Screenshots record the cleaned
catalog and both defaults. The actual Tailscale Console URL was checked again;
services and Console forwards remain available. The previous workload URLs no
longer represent retained running apps because the user deleted those apps.

No product source changed. Validation is the bounded SQL assertions, exact API
comparisons, real browser/UI operations and native read-only inventory/UID checks;
the earlier full product suite remains attributed to its unchanged source.

## Explicit default classes and clear names — user-approved follow-up

On the same feature branch the user approved names `Gateway Traefik có sẵn` and
`Endpoint qua Gateway`, each with the explicit criterion `{"class":"default"}`.
The existing Console code editor saved each as r7; the Versions UI promoted both
with generation-CAS. Form↔code round-trip, exact name/criterion, unchanged keys,
driver/account/infrastructure inputs and Endpoint reference to
`k8s-gateway.default` were verified. Other four Definitions remain API-identical.
This changes trial configuration only, not product source or shared Gateway.

A temporary Application tested one Environment Preview containing both saved
native and Score workload drafts. Preview succeeded with one Gateway node and
two Endpoint nodes, all class default and selecting the retained r7 Definitions.
No Deploy was submitted. The temporary Application was deleted through the
product confirmation/scope/admission/polling flow; deletion SUCCEEDED and the
active Application list is empty again. Final browser check confirms six catalog
cards, the new names and active revisions over the actual Tailscale URL.

Initial harness attempts are retained: an overly long temporary Application name
was rejected with HTTP400/name before creation; a shorter name succeeded. A
separate Definition Matching Preview request revealed its current driver allowlist
accepts Terraform/template/existing-cluster only, so Echo/Endpoint are rejected.
That separate API limitation is not changed or reported as passing here. Actual
Environment Preview, using the current planner, supplied the selection evidence.
No full product suite was rerun because no product source changed; the prior full
run remains source-attributed. Focused validation is UI/code/API invariance,
real Environment Preview, product cleanup, native read-only checks and docs checks.
