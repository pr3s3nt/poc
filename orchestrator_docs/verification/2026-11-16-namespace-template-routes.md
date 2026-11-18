---
id: VERIFY-20261116-NAMESPACE-TEMPLATE-ROUTES
artifact: verification-record
status: current
last_reviewed: 2026-11-16
---

# Namespace placement and template HTTPRoute — issue #84

Feature branch `features/Httproute`, after the user's requested integration of
Score `621fa61d`. Product implementation `1a33bed7`, hardened at `d5293723`;
full-check collector `dc9d8b49`; live acceptance runner `b57897f8`.
Required behavior: [ADR-043](../architecture/decisions/ADR-043-namespace-placement-template-routes.md)
and [contract](../architecture/contracts/namespace-template-routes.md).
No user acceptance, main merge, retained-installation upgrade or release tag is
claimed by this execution record.

## Sources and boundaries

- Release repository `orchestrator-workload-templates`, feature commit
  `5407998b0e08dc0da51c19c28517dfa191e7a3ee`: pinned and inspected by the task
  installation. Its bundle/HTTPRoute template bytes equal the product copies.
  Terraform module bytes are unchanged; MongoDB sample binds through namespace.
- Separate task Docker Console with fresh PostgreSQL/Vault volumes; existing
  retained Console and Score review data were not upgraded or edited.
- Existing `kind-k8s-4f` cluster, retained Traefik/Gateway and listener used as
  preexisting infrastructure. No Gateway/controller/DNS/TLS object was mutated,
  adopted or deleted. No cloud operation or SQL schema change was performed.
- Private receipts, screenshots, tool logs and scoped credentials remain outside
  Git. Trial URLs and local access/restart details belong to the private handoff,
  never the public issue.

## Real application execution

Using [the scenario](../../e2e/scenarios/namespace-template-routes/README.md):

1. Through Console forms, registered a separate `k8s-gateway` Definition with
   whole cluster reference and a separate endpoint Definition referencing its
   gateway output. Existing Gateway readiness was verified by read-only adapter.
2. Imported and saved native and Score workloads through Console before Preview.
   Both have a web container, ordered database-check init container, Service and
   endpoint, and share one PostgreSQL resource. One Environment Preview/Deploy
   included both workloads and succeeded through real Git/Fleet reconciliation.
3. Native assertions found the database, Bound PVC, both web Deployments and both
   HTTPRoutes in the product namespace, with the exact intended Service port,
   hostname/path and existing Gateway parent/listener. Routes report Accepted and
   ResolvedRefs. An authenticated TCP `SELECT 1` and both init clients succeeded.
4. Browser opened both real nginx workloads through the Gateway over Tailscale.
   This checks application content, not a Console HTML shell or mocked backend.
5. Saved both updated drafts before one Environment Deploy. Both succeeded and
   show the updated environment variable. Namespace, PostgreSQL, PVC, Gateway,
   Deployment and HTTPRoute UIDs were unchanged.
6. Recreated only the native web Pod after namespace/app/ReplicaSet/controller UID
   checks and API UID/resourceVersion preconditions. A new ready Pod appeared;
   init clients, authenticated SQL, route status and all preserved resource UIDs
   passed again. Both applications remained browser reachable.
7. Marked both workloads for removal through Console confirmations, then deployed
   both removals together. Deployment/HTTPRoute absence was checked before
   removing only the recorded setup RoleBinding/Role with label/UID/API guards.
8. Console Application deletion succeeded on its first admitted operation:
   PostgreSQL DESTROYED, then namespace DESTROYED. Native reads confirmed namespace
   and backing PV absence; the preexisting Gateway UID remained unchanged.
9. A fresh review application deployed the same native/Score workloads and
   released endpoint names. Namespace/database/PVC UIDs differ from the cleaned
   application; the Gateway UID matches. It remains running for user acceptance.

Technical setup of namespace-local delivery access is explicitly separate from
the above product outcomes. The role helper never creates/adopts the namespace.
An initial harness attempt dismissed the browser confirmation dialog by default;
no removal was submitted. The runner now accepts only the named workload's
confirmation. Another restart harness check initially expected a route-only
label on the Pod and stopped before mutation; it now verifies the real workload
labels and ReplicaSet-to-Deployment ownership chain. Failed attempt evidence is
retained privately; no product guard or acceptance assertion was weakened.

UI reference was the signed-in existing Score review Console's Resource
Definitions, AppShell, shared styles/tokens and Button. Final browser inspection
opened both stored Gateway and endpoint forms without saving; their Definition
versions/values were unchanged. The forms retain the shared layout, compact
field labels and collapsed optional help; endpoint shows only the Gateway
reference instead of duplicating Gateway policy. No extra instructional block or
unrelated shell/navigation redesign was introduced. The final Console Application
and both workload URLs were checked again through Tailscale at handoff; the task
stack, database/PVC, transports and forwards remain running. Private handoff
records login-file location, restart commands and retained data boundaries.

## Validation and limits

Focused namespace provider/pin, Gateway registration/readiness/account version,
template endpoint intent/route guards and legacy endpoint checks passed. Backend
build passed. Frontend focused authoring checks passed (94 tests, then 12 changed
mapping/endpoint tests); typecheck/lint/build passed with existing lint and bundle
size warnings. Docs/whitespace checks passed and the sequence PNG was generated
with PlantUML 1.2025.2 (the local docs checker separately reports no PlantUML on PATH).

Full checks completed on isolated receipt-owned kind namespaces. Backend sources
were exact Git archives: candidate `dc9d8b49`, main `cd40a792`. Both used Go
1.25.14, Terraform 1.9.8, independent real PostgreSQL 16, REQUIRE_POSTGRES=1,
REQUIRE_TERRAFORM=1, `go test -json -p 2 -parallel 4 -count=1 -timeout 30m ./...`
and `go build ./...`.

| Backend top-level test outcome | Candidate | Main |
|---|---:|---:|
| Pass | 1587 | 1568 |
| Fail | 103 | 103 |
| Skip | 8 | 8 |
| Failed packages | 20 | 20 |
| Build exit code | 0 | 0 |

The exact failing test-name sets match, including nested tests (143 failing
events per side); both set differences are empty. Failed-package and skipped-test
sets also match. Candidate adds 19 passing top-level tests across Score and this
feature; no main passing test is absent. The suite remains red, not a full green
gate. Representative baseline failures include released-schema tests referring
to removed `applications.subdomain`, legacy renderer/acceptance fixture mismatch,
duplicate catalog fixture IDs and unsafe-source fixture expectations. No failure
or guard was removed, skipped or relaxed to obtain this comparison.

The same eight skips on both sides are six separately opted-in live conformance
tests (owned Fleet/metadata, object lifecycle, topology, runtime defaults and
account diagnostics) without their explicit environment, plus two historical
Xvfb launch/cleanup tests because Xvfb is absent. These are recorded as skipped,
not passed; the new real native/Score browser lifecycle above is separate
evidence and does not claim coverage of those conformance scenarios.

Candidate backend source tree is `dcf08075b7e2f0f8d0b27fbab92ae1f674046324`;
frontend tree is `7fc631d437f102cc4632146b8c89865444a81da0`. E2E/docs-only follow-up
commits do not change those product trees.

Initial full harness execution had malformed shell-generated result JSON; its
`finally` cleanup confirmed namespace absence. No missing results count as pass.
The corrected collector's first frontend run passed 1061 tests but failed six
cleanup-shell unit tests because the Alpine Node image lacks bash (spawn returned
null status). Typecheck/lint/build passed. A separate full frontend rerun uses
Node 22.23.4 Bookworm, including bash, from commit `c02dc4dc` with the identical
frontend tree: all 113 files / 1067 tests passed, as did install/typecheck/lint/
build. Assertions and fixtures were unchanged. The initial red result remains
recorded. The collector process was interrupted after checks completed; guarded
resume collected the existing results without rerunning tests, verified namespace
UID and namespace/Pod owner labels, then confirmed cleanup. Both full-check namespaces are
absent; all collected logs/receipts remain private.

The previously known `TestDeployWithUnknownTerraformOutcomeHoldsTheEnvironment`
failure was independently reproduced on main `cd40a792`: renderer unavailable
before its operation assertion. This does not make the full backend suite green.

## Identical failing test inventory

The following 103 top-level failures occur on both candidate and main. Packages
with build/no-test-level failures are also present in the 20-package result set;
private JSONL preserves every nested failure and diagnostic.

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
