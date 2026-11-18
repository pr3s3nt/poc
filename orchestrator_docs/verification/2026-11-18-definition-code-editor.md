---
id: VERIFY-20261118-DEFINITION-CODE-EDITOR
artifact: verification-record
status: current
last_reviewed: 2026-11-18
---

# Synchronized Definition code editor — issue #87

Approved proposal implemented at `04f147e3`; runner stdin fix `0ce85374`, active-tab
notice fix `18fcf579`, on features/definition-code-editor from main `725541ef`.
No merge-back, retained installation rollout or issue closure. Required behavior:
[CODE-01–06](../usecase/UC-03/specification.md#sửa-definition-bằng-code--issue-87-approved-2026-11-18).

## Result and focused checks

Existing Definition edit has Form / YAML/JSON with a shared authoring draft.
Form deltas retain unmodelled fields; code parses strictly and validates via
existing prepare. Key/type/version context and checked immutable Git pins remain
protected. A lossy form projection keeps code and a clear error. Both Saves use
the existing PUT/version-CAS; stale/network/uncertain outcomes hold draft without
replay. Explicit empty template resource references survive round-trip. No new
backend API, schema/migration, Type or deployment behavior.

16 new tests pass, covering four drivers, UI/code sync, nested advanced fields,
prototype-safe data keys, invalid syntax/identity/system fields, checked Git pin,
stale/network/uncertain Save and unmount. The final 80-test Definition-page/editor
check passes after fixing duplicate inactive-tab notices; existing cluster,
endpoint, template, import/export/parser/revision suites checked in scope pass.
Typecheck/lint/build, Node syntax, docs checker and whitespace pass. Existing
PlatformSettingsPage lint/bundle warnings remain. PlantUML is unavailable locally;
no diagram/sequence participant changed.

## Real isolated browser verification

A separate Docker installation was created with its own PostgreSQL/Vault/cache/
credential volumes. An Internal account was verified against authorized kind
using read-only API checks; exact Git sources prepared real Terraform namespace
and workload-template fixtures. Four clearly named catalog samples are retained,
not added to the older #84 trial or retained Console. Setup is not app deployment.

Browser opened the exact Tailscale Console URL, authenticated, loaded real API
catalog data and edited existing-cluster, endpoint, Terraform namespace and
workload-template Definitions. For each it changed a field in the form, confirmed
code, wrote YAML, returned to the matching form, saved, then edited/saved JSON.
Both saves bumped version exactly once; driver/account/input/source pins and
criteria/provision were unchanged during the name edit. Active revision stayed
unchanged and history gained exactly two proposed revisions. Application lists
were unchanged and no deployment request occurred. The editor remained correct
after Save and another form round-trip. Final-source browser repetition passed
at 02:32 UTC (09:32 Bangkok), including full-width textarea checks.

UI reference: existing signed-in Definition editor and namespace/provider forms,
AppShell/styles/tokens/Button. Visual inspection caught an initially narrow,
unstyled textarea; it now reuses the shared editor fieldset/monospace controls,
full panel width and existing spacing. Browser asserts available width, with
private code/form/catalog screenshots. Optional help stays collapsed; no extra
banner or shell/navigation/theme change. Access was tested from the host using
the actual tailnet URL, not an HTML shell/mock or a claim about every device ACL.

## Full frontend execution and attempts

Fresh full frontend from exact `18fcf579` archive passed on authorized kind:
**116 files / 1093 tests**, install/typecheck/lint/build all exit 0, one worker /
Node 22.23.4 Bookworm. Frontend tree is
`53785a63cc2b060ae682d546fa5f875c11170d73`.
One worker matches the accepted main comparison, which retains the documented
four-worker observability timing instability; no assertion is changed.

Initial runner stdin error failed before namespace creation; failed receipt/log
are retained. The next run at `0ce85374` exposed three duplicate-notice failures
in the existing Definition suite. Product fix `18fcf579` renders notices only in
the active method. Its focused 80 tests and the fresh full run pass without changing assertions.
The old run retained 1090 pass / 3 fail; all other checks passed. Failures were
`loads the stored configuration, keeps the ID and Resource Type read-only and saves one PUT with the expected version`,
`keeps the saved result when the list reload fails and retries only the list`, and
`switches the verified module of the same Resource Type during an edit, keeps each pair draft and saves one PUT with the chosen module`
in ResourceDefinitionsPage. Both created namespaces were deleted with original
owner/UID/resourceVersion conditions and their absence confirmed. Failed logs
remain private, attributed to their actual source. Later documentation-only
commits leave the tested frontend/backend trees unchanged.

## Unchanged backend evidence — explicitly reused

Current feature and main `725541ef` have the exact backend tree
`ef48c434a788310e9f130aaf74cd0519999fc499`, identical to accepted main `57ecc971`.
The new backend binary builds and operates the real trial API; no backend source,
dependency, schema or test was changed. Completed kind backend logs from the
[accepted main run](2026-11-17-keycloak-main.md) are reused by that source identity,
not called a fresh candidate/main execution.

Raw reused results retain **2485 passing / 102 failing / 8 skipped test/subtest
events**, **68 top-level failing tests / 15 failed packages**. Both current backend
source trees are identical, so this source evidence applies equally; none of the
failures is relabelled as pass. The PostgreSQL package timed out after 30 minutes
in the upgrade/runtime-isolation test; later assertions were not all executed.
Eight live-kind/Xvfb opt-in skips remain skips. The original jobstore fixture's
loopback-password failure remains in raw counts, alongside its separately
recorded corrected focused result. This is not a green full-backend gate or a
complete PostgreSQL suite. Its disposable namespace cleanup was recorded by the
original run; no new backend test namespace was created for this UI-only change.

The #84 Gateway driver is not on this main base. The editor handles the backend's
supported authoring DTO without introducing that driver; #84 integration retains
its driver-specific form. Samples/volumes/bridge stay up for user acceptance.
No cloud, shared Gateway/controller/DNS/TLS or retained application mutation.
Private credentials, URLs/IPs, receipts and screenshots remain outside Git/issues.

## Reused top-level backend failure inventory


### `orchestrator/examples/acceptance-app/internal/jobstore`

- `TestStoreWithReservedCharacterPassword`

### `orchestrator/internal/adapters/postgres`

- `TestBaselineEnforcesCurrentConstraints`
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

### `orchestrator/test/integration/v2cleanup`

- `TestDeliveryCleansAppliedTargetsWithoutLegacyEnvironmentBinding`

### `orchestrator/test/integration/v2run`

- `TestTheScenarioScriptRecordsExactlyTheAssertionsTheRunnerRequires`

Failed-package set (including packages without a top-level test failure):

- `orchestrator/examples/acceptance-app/internal/jobstore`
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
