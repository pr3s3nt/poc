---
id: TRACEABILITY-MATRIX
artifact: traceability-matrix
status: current
last_reviewed: 2026-10-09
---

# UC-00..UC-09 Traceability Matrix, with UC-12/UC-16 design traces

Mỗi main-flow step được ánh xạ tới operation, PlantUML sequence, class/method, persistence/state và planned test. `OC-nn` tham chiếu `architecture/contracts/operation-contracts.md`.

## UC-00

| Steps | Operation / sequence | Class methods | Persistence/state | Contract / tests |
|---|---|---|---|---|
| MS-01, MS-02 | `SignIn`; `UC-00/sequence.puml` | `authentication.Service.SignIn`, account lookup, `password.Verify` | read active `user_accounts`; no session on failure | OC-00; `authentication/service_test.go`, `test/e2e/onboarding_test.go` |
| MS-03 | same | session identity builder | User ID, Organization key, role | OC-00; Organization/role and cross-Organization HTTP tests |
| MS-04 | same | random-token generator, `SaveSession` | token-hash-only `sessions` record | OC-00; raw-token persistence and response-leak tests |
| MS-05 | authentication middleware | session lookup and context injection | authenticated request context | OC-00; protected-route, expiry, revocation, production-profile and Playwright tests |

## UC-01

| Steps | Operation / sequence | Class methods | Persistence/state | Contract / tests |
|---|---|---|---|---|
| MS-01, MS-02 | `CreateApplication`; `UC-01/sequence.puml` | `application.Service.Create`, strict HTTP decoder, DNS-label validator | `applications`; case-insensitive Organization Name and global normalized Subdomain constraints | OC-01; service, HTTP and PostgreSQL constraint tests |
| MS-03, MS-04 | same | generate Application identity/provider, no target resolution | Application ID; new Environment targets UNCONFIGURED | OC-01; creation without default and no provisioning |
| MS-05, MS-06 | same | Environment and empty Deployment Set creation | exactly `staging` and `production`; `environments`, `deployment_sets` | OC-01; transaction rollback, HTTP and PostgreSQL persistence tests |
| MS-07, MS-08 | same | endpoint derivation, Application persistence | derived desired endpoints; atomic Application/Environment/current-set write | OC-01; no-deploy-side-effect frontend/HTTP/Playwright tests |
| UC-01 ES-01..06, BR-07..14; UC-05 BR-11; UC-06 BR-20; UC-08 BR-07 | scoped target resolution, shared snapshot, matching and executor binding validation | Historical ADR-011 Environment Settings set-once UI/API (superseded by ADR-012 rows below), resolver, `planning.ConnectionMismatch` + provisioning guard | generation-0 identities/legacy backfill; permanent lock superseded by ADR-012 | explicit choice, unsafe/foreign choice, mismatch rejection, [HTTP](../../backend/test/e2e/application_connection_http_test.go), [planner](../../backend/internal/planning/connection_binding_test.go), [execution](../../backend/internal/application/deployment/selected_target_test.go), ADR-011 migration/concurrency/save-guard/new AWS identity tests; [human Settings/live kind evidence](../verification/2026-10-07-environment-connection-kind.md) |

## UC-02

| Steps | Operation / sequence | Class methods | Persistence/state | Contract / tests |
|---|---|---|---|---|
| MS-01, MS-02 | `RegisterResourceType`; `UC-02/sequence.puml` | `catalog.Service.RegisterResourceType`, `resource.Type.Validate`, role-gated HTTP POST | none before validation | OC-03; `catalog/service_test.go`, `test/e2e/http_test.go` |
| MS-03, MS-04 | same | `ListResourceTypes/CreateResourceType` | insert-only `(org,key)` on memory/JSON/PostgreSQL | OC-03; registration contract, repository/service/HTTP race tests |
| MS-05, MS-06 | same | return `ResourceType` / repository read contract | persisted schemas | OC-03; planner catalog integration test; `ResourceTypesPage.test.tsx` |

## Definition-selected workload rendering (ADR-010)

| Requirement / boundary | Implementation | Local evidence |
|---|---|---|
| UC-03 BR-15/16 registration | `resource.ValidateRenderDefinition`, catalog bundle registry and Console form | `catalog/rendering_test.go`, `ResourceDefinitionsPage.test.tsx`, `test/e2e/rendering_http_test.go`; upgrade A → B: `planning/rendering_test.go`, `pending/rendering_test.go` |
| UC-05 BR-09/10; UC-06 BR-17 plan pinning/preflight | `planning/rendering.go`, deployment preflight, additive plan JSON | `planning/rendering_test.go`, `deployment/scorek8s_test.go` |
| UC-06 BR-18/19; UC-08 BR-09 output-only rendering | `adapters/scorek8s`, shared platform policy and existing delivery | `scorek8s/renderer_test.go`, `deployment/scorek8s_test.go` |
| UC-07/12/16 identity, configuration, no-op and stale token preservation | pending renderer comparison with last persisted plan; unchanged Secret/delivery owners | `pending/rendering_test.go`, existing pending/configuration/remove tests, CLI Agent/VSO reference tests |

See [rendering contract](../architecture/contracts/workload-rendering.md).
Local CLI tests use fake executors/deployers; live kind/AWS remains unverified.

## UC-03

| Steps | Operation / sequence | Class methods | Persistence/state | Contract / tests |
|---|---|---|---|---|
| MS-01, MS-02, MS-03; BR-07 | `RegisterResourceDefinition`; `UC-03/sequence.puml` | `catalog.Service.RegisterResourceDefinition`, `resource.Definition.Validate`, role-gated HTTP POST | none before validation | OC-04; `catalog/definition_test.go`, `test/e2e/http_test.go`, `ResourceDefinitionsPage.test.tsx` |
| MS-04, MS-05 | same | type/connection repository reads; `ValidateReferencesAndRules` | `resource_types`, `connections` | OC-04; invalid reference test |
| MS-06, MS-07 | same | `ListResourceDefinitions`, embedded `terraform.Inspector.Inspect` or static executor output contract | source fingerprint candidate | OC-04; duplicate/remote-source/output validation tests |
| MS-08, MS-09; BR-07 | same | `CreateResourceDefinition/ListResourceDefinitions` | insert-only org-scoped catalog; PostgreSQL Definition + criteria atomic transaction | OC-04; registration contract/race/criterion-failure rollback; no-restart HTTP Preview; explicit `{}` wildcard and criteria-shape conformance |
| BR-01, BR-03 | [ADR-001](../architecture/decisions/ADR-001-profile-resource-scopes.md) | Optional Definition profile guard before unchanged five-field scoring | `TestPlan_NewApplicationMatchesPostgresByExecutionProfile`, invalid-profile validation |

## UC-04

Kubernetes upload implementation and local/isolated PostgreSQL tests pass on
2026-10-06. [Video verification and review](../verification/2026-10-06-uc04-kubeconfig-upload.md)
are recorded separately. AWS remains future scope.

| Steps | Operation / sequence | Class methods | Persistence/state | Contract / tests |
|---|---|---|---|---|
| MS-01..03; VAR-01 | inspect; `UC-04/sequence.puml` | HTTP Controller, InspectKubeconfig, parser | transient safe context metadata | OC-05; `TestInspectKubeconfig_ListsSafeMetadataOfValidContexts`, `TestKubeconfig_RequiresAPIVersionAndKind`, `TestUC04UploadHTTPContract`, `ConnectionsPage.test.tsx` |
| MS-04..05; BR-06..08 | upload register, verify | Service.RegisterKubeconfig, verifier.VerifyKubeconfig | read-only API/RBAC | OC-05; `TestNormalizeKubeconfig_RejectionsAreSafeAndCategorized`, `TestVerifyKubeconfig_ReadOnlyChecksWithSelectedContextOnly` |
| MS-06; BR-02,09,10 | credential Put | scoped CredentialStore | external immutable secret, no database bytes | OC-05; `TestConnectionCredentials_ScopedCreateOnlyLifecycle`, `TestConnectionCredentials_RejectsOutOfScopeReferencesWithoutRequests`, `TestCredentialResolver_FailsClosed`, `TestCredentialTarget_KubectlFailuresNeverEchoOutput` |
| MS-07; BR-01,15,16 | CreateConnection | repository + UnitOfWork | name/auth type, insert-only READY, default unchanged | OC-05; `TestRegisterKubeconfig_ConcurrentSameNameGetsDistinctKeys`, `TestMigration5BackfillsConnectionNameAndAuthentication`, `TestPostgresKubeconfigRegistrationRaceAndReopen`, `TestSnapshotConnectionsReadWithLegacyDefaults` |
| MS-08..09 | public response/reload | DTO, ConnectionsPage | no response secret; catalog consumption | OC-05; `TestUC04UploadHTTPContract`, frontend registration/state tests, dedicated Playwright runner |
| ERR-07..08 | write/cleanup failures | CredentialStore.Delete | only own attempt cleaned with bounded context | OC-05; `TestRegisterKubeconfig_FailuresSaveNothingReady`, `TestUC04UploadWithoutCredentialStoreFailsClosed` |
| BR-10; POST-03 | execution credential resolution | resolver, Kubernetes adapters | opaque target identity only | `TestCredentialTarget_DeployerUsesScopedPrivateKubeconfig`, `TestCredentialTarget_RoutesVSOAndExecutorUseScopedCredential`, `TestExistingCluster_CredentialBackedConnectionIsAuthoritative`, `TestCredentialBackedTarget_RemovalAfterRestart`, `TestFleetRejectsCredentialBackedTargets` |
| VAR-02; BR-11..12,14 | future AWS register | AWS verifier/resolver, Terraform | ADR-009 credential namespace | designed; not implemented by upload slice |

## UC-05

| Steps | Operation / sequence | Class methods | Persistence/state | Contract / tests |
|---|---|---|---|---|
| MS-01 | `PreviewDeployment`; `UC-05/sequence.puml` | `LoadPlanningSnapshot`, `Store.ReadSnapshot` shared with Deploy | one consistent read snapshot, current set/version | OC-06; `TestPreview_ReadsOneConsistentSnapshot`, `TestPostgresPreview_RepeatableReadAndNoWrites` |
| MS-02, MS-03; BR-07 | `PlanningService.Plan` | `ScoreConverter.ConvertAndValidate`, `WorkloadSpecValidator.ValidateContainerResources` | typed fragment with optional CPU/memory requests/limits | OC-07; Score fixture + container-resource contract tests (`TestParse_ContainerResources*`, `TestFragment_ContainerResourcesStayPerContainer`, `TestPlan_PreservesContainerResourceRequirements`, `TestPlan_ContainerResourcesAddNoGraphNodes`, `TestPlan_ContainerResourceChangeIsModuleRelativePatch`) |
| MS-04 | same | `BeforeStateValidator.Validate`, `DeltaBuilder.BuildHumanitecDelta` | transient Humanitec-shaped Delta/Candidate | OC-07; shape, relative-patch, array-diff and invariant tests (`internal/planning/delta_test.go`, `internal/planning/jsonpatch`, conformance `assertDelta`) |
| MS-05 | same | `ImplicitResourceEnricher.Enrich`, `ResourceGraphBuilder.BuildAndExpand`, `DefinitionMatcher.MatchAll` | in-memory graph/matches | OC-07; AWS/internal graph tests |
| MS-06, MS-07 | same | `DriverContractInspector.InspectContracts`, `ActiveResourceClassifier.Classify`, `BatchScheduler.Schedule` | in-memory plan | OC-07; contract/topology fixtures |
| MS-08 | `PreviewDeployment`, scoped `score-preview` API and Console | `PreviewDeployment`, explicit `Public` view | no write/state transition; no pending token | OC-06; `TestPreview_NoRuntimeMutation`, `TestPreview_MatchesDeployPlan`, `TestUC05ScorePreviewHTTP`, `ScorePreviewPage.test.tsx`; secret/error projection and stale-response tests |

## UC-06

| Steps | Operation / sequence | Class methods | Persistence/state | Contract / tests |
|---|---|---|---|---|
| MS-01; BR-16 | `DeployWorkload`; shared/cloud/internal sequences | `DeploymentService.DeployWorkload`, `EnvironmentRepository.LoadPlanningSnapshot`, `DeploymentRepository.Create` | `deployments=PLANNING`, Snapshot ID nullable; planning failure may be `FAILED` without Snapshot | OC-08; `TestDeploymentReferencesExactlyOneDeltaSnapshot`, planning-failure query test |
| MS-02, MS-03 | `PlanningService.Plan` | Score/workload validator/before/`BuildHumanitecDelta` | `deployment_delta_snapshots`, Candidate `deployment_sets` | OC-07/08; Delta shape/invariant and resource-preservation tests; Snapshot persistence/association (`TestDeployWorkload_PersistsOneDeltaSnapshotPerDeployment`, `TestDeploymentReferencesExactlyOneDeltaSnapshot`) |
| MS-04, MS-05, MS-06; BR-12 | same | profile load, `ResourceDescriptorParser.ParseDescriptorText`, `ImplicitResourceEnricher`, graph builder | canonical descriptors + graph JSON snapshot | OC-07; scoped-token and implicit-resource tests |
| MS-07, MS-08 | same | matcher, inspector, classifier, scheduler | `deployment_plans`; `PROVISIONING` | OC-07/08; match/contract/DAG tests |
| MS-09 | `ResourceProvisioningService.Provision` | UC-08 methods | `active_resources`, `deployment_resources` | OC-10; resource integration tests |
| MS-10 | deploy | `OutputBindingResolver.ResolveWorkloadBindings` | resolved values stay in execution context; safe snapshot only | OC-08; output propagation test |
| MS-11; BR-11 | deploy | `WorkloadRenderer.Render`, `WorkloadDeployer.Apply/WaitReady` | declared requests/limits in manifests; missing request = same-field limit, else default; `workload_instances=APPLYING/READY` | OC-08; renderer resource mapping + kind/fake adapter tests (`TestRender_*ContainerResources*`, `TestRender_LimitsOnlyBelowDefaultUseLimitAsRequest`, `TestRender_DoesNotMutateModuleResources`, `TestDeployWorkload_RendersDeclaredContainerResources`, kind `TestKindInternalVerification` live resources) |
| VAR-03; BR-13/BR-14 | Fleet GitRepo delivery | `gitops.Deployer.Apply/WaitReady/Reconcile`, Fleet `GitRepo` | Workload bundles plus non-secret Environment `_routes` Ingress bundle; exact workload/route revision observed | ADR-007; `gitops/deployer_test.go`, `public_routes_test.go`, [2026-09-28 route recheck](../verification/2026-09-28-public-routes-recheck.md) |
| BR-15 | Route-only retry | `pending.Service.Deploy`, `DeploymentService.ReconcilePublicRoutes` | `Environment.publicRoutesPending` persists until route applied, no workload apply on retry | `TestFailedRouteCanRetryWithoutRestartingWorkload` |
| MS-12 | deploy | `CompareVersionAndSetCurrent`, repo upserts, `MarkSucceeded` | atomic current pointer + `SUCCEEDED`; AWS runtime `READY` | OC-08; commit-after-ready transaction test |
| MS-13 | deploy | return `DeploymentResult` | read committed status | OC-08; API contract test |

## UC-07

| Steps | Operation / sequence | Class methods | Persistence/state | Contract / tests |
|---|---|---|---|---|
| MS-01, MS-02 | update/remove; `UC-07/sequence.puml` | load snapshot, `BeforeStateValidator.Validate` module + declared shared entries | base set/version | OC-09; stale module/shared mismatch tests |
| MS-03, MS-04 | `PlanningService.Plan` | `DeltaBuilder.BuildHumanitecDelta` conflict/reference/relative-patch rules; graph/classifier | immutable Delta Snapshot + Candidate plan | OC-07/09; module add/remove/update, shared patch, conflict, last-reference and preserve-other tests (`TestDelta_*`, `TestPlan_RemoveWorkloadDeltaListsModule`) |
| MS-05 | `ResourceProvisioningService.Provision` | UC-08 methods | desired resources `READY` | OC-10; reconciliation test |
| MS-06 | update/remove | `WorkloadDeployer.Apply/WaitReady` or `Delete` | workload `READY` or `REMOVED` | OC-09; update/remove adapter tests |
| VAR-03 | Fleet GitRepo remove | `gitops.Deployer.Remove` | scoped bundle path removed; Fleet prunes Deployment/Service before state commit | `fleet-gitrepo-kind-verify.sh`; kind delete/cleanup evidence |
| MS-07 | remove/update | `Service.markUnreferenced`, `FindByLogicalIdentity/UpsertActiveResource` | Environment-owned READY → UNREFERENCED; preserve outputs/state; no destroy or Application-wide cleanup | OC-09; `TestRemoveWorkload_PreservesSharedDatabaseAndMarksLastReference`, scope and re-reference tests |
| MS-08 | final commit | `CompareVersionAndSetCurrent`, marker upserts, `SaveDeployment` | atomic current pointer/marker/SUCCEEDED; rollback on failure | OC-09; `TestUnreferencedMarking_RollsBackWithFailedRuntimeOrCommit`, `TestPostgresRemove_CommitsSetStatusAndMarkerAtomically` |
| BR-08 | Candidate validation; pending final batch | `ValidateServiceReferences` | no runtime mutation for dangling Service; consumer/provider batch remove allowed | OC-07/09; `TestRemovingReferencedServiceIsBlockedUnlessConsumerGoesToo`, `TestDirectRemovalOfReferencedServiceFailsBeforeSideEffects` |
| Console delivery boundary | UC-16 scoped pending API | strict draft/version/token decoding; Application home/editor | confirmation/Undo, stale/busy/obsolete-response guards; safe results/history failures | `TestUC07PendingDraftHTTPContract`, `TestDeployReportDoesNotEchoRuntimeErrors`, `TestDeploymentHistoryHidesUnsafeFailureReasons`, frontend tests and local recording |

## UC-08

| Steps | Operation / sequence | Class methods | Persistence/state | Contract / tests |
|---|---|---|---|---|
| MS-01 | `Provision`; `UC-08/sequence.puml` | `ResourceProvisioningService.Provision` | read immutable `deployment_plans` | OC-10; batch-order test |
| MS-02 | same | `ActiveResourceResolver.Resolve`, `FindByLogicalIdentity` | active logical/executor state | OC-10; VPC/EKS reuse test |
| MS-03 | same | `OutputBindingResolver.ResolveInputs` | completed output context | OC-10; provider-output test |
| MS-04, MS-05 | same | `ExecutorRegistry.Resolve`, `ResourceExecutor.Provision` | external Terraform/Kubernetes state | OC-10; executor contract tests |
| MS-06 | same | `OutputContractValidator.Validate` | validated outputs | OC-10; cross-profile postgres contract test |
| MS-07 | same | `ActiveResourceRepository.UpsertReady`, `DeploymentResourceRepository.MarkReady` | short atomic node transaction | OC-10; repository integration test |
| MS-08, MS-09 | same | update run context and loop batches | next consumer inputs | OC-10; chain/diamond topology tests |
| MS-10 | same | return `ProvisionResult` | descriptor -> outputs/target | OC-10; result contract test |

Container resource requests/limits intentionally do not appear in UC-08 rows:
they remain workload module data and are consumed by UC-06 MS-11 after UC-08
returns the target and resource outputs.

## UC-09

| Steps | Operation / sequence | Class methods | Persistence/state | Contract / tests |
|---|---|---|---|---|
| MS-01, MS-02 | `ListDeployments` / `GetDeployment`; `UC-09/sequence.puml` | authenticated scoped query service, repo reads | Deployment/plan/set snapshot | OC-11; session/Organization/Application/Environment scope tests |
| MS-03 | same | deployment resource/workload snapshot repository reads | `deployment_resources`, `deployment_workloads` | OC-11; snapshot immutability and status assembly tests |
| MS-04, MS-05; BR-06 | same | `DeploymentViewAssembler.Assemble` | persisted graph/matches/batches if present; no fake Delta/plan on planning failure | OC-11; planning-failure query test |
| MS-06 | same | output redactor; omit resolved inputs | no mutation | OC-11; encoded-response secret/input absence test |
| MS-07 | same | return `DeploymentView` | read-only | OC-11; no runtime call test |
| BR-05 | `UC-09/ui/screens.md`, `ui/states.md`, `ui/api-mapping.md` | React history/detail/recent components | scoped Application/Environment deployment GET routes | frontend state/filter tests and local Playwright |

## UC-12 — MVP implementation trace

UC-12 specification and UI are approved. [ADR-006](../architecture/decisions/ADR-006-application-configuration-provider.md)
selects a per-Application provider with Vault as the first adapter; the
persistent kind Vault release is installed and initialized. Desired and applied
revision paths are covered by `backend/internal/application/configuration/service_test.go`,
`backend/internal/adapters/vault/provider_test.go`,
`backend/test/e2e/configuration_workload_test.go` and
`frontend/src/features/configuration/SettingsPage.test.tsx`,
`backend/internal/adapters/kubernetes/renderer_test.go`,
`backend/test/e2e/configuration_workload_test.go` and the run-scoped
`backend/test/integration/uc12-kind-verify.sh`. Production HA and secret
lifecycle are not covered.

| Steps | Design artifact | Required state/validation | Planned test |
|---|---|---|---|
| MS-01 | [Screens](../usecase/UC-12/ui/screens.md) | Two Environment tabs; variable and secret sections together in each tab | Type and Environment isolation |
| MS-02–MS-03 | [Specification](../usecase/UC-12/specification.md) BR-01–BR-03 | Scoped uniqueness; secret write without readback | Duplicate names, secret redaction and no-readback tests |
| MS-04 | [Realization](../usecase/UC-12/realization.md) | Affected workloads and pending Preview → Deploy; runtime unchanged | Impact list and no-runtime-mutation tests |
| VAR-01–VAR-03 | [States](../usecase/UC-12/ui/states.md) | Warning but allow rename/delete; no automatic reference repair; secret replacement hidden | Warning/confirm, broken-reference Preview rejection and secret-update tests |
| BR-08–BR-10, BR-14 | [ADR-006](../architecture/decisions/ADR-006-application-configuration-provider.md), [ADR-008](../architecture/decisions/ADR-008-vso-native-secret-delivery.md) | Per-Application provider; immutable desired/applied revisions; VSO Secret with `secretKeyRef` on kind | Provider isolation, pending-vs-applied revision, VSO Secret synchronization and restart tests |

## UC-16 — MVP implementation trace

UC-16 specification and UI are approved. Draft save/delete/undo, Score import
and reference validation are covered by
`backend/internal/application/workloadconfig/service_test.go`,
`backend/test/e2e/configuration_workload_test.go` and
`frontend/src/features/workloads/WorkloadEditorPage.test.tsx`,
`frontend/src/features/applications/ApplicationHomePage.test.tsx` and
`backend/internal/application/workloadconfig/service_test.go`. Preview,
stale-token rejection, partial retry and edit reconstruction are exercised in
local tests; create and configuration-only redeploy are verified on kind.

| Steps | Design artifact | Required state/validation | Planned test |
|---|---|---|---|
| MS-01–MS-03 | [Screens](../usecase/UC-16/ui/screens.md), [sequence](../usecase/UC-16/sequence.puml) | Selected Environment and workload form/import | Environment isolation and form/import parity |
| BR-12 | [Screens](../usecase/UC-16/ui/screens.md), [UC-06 BR-14](../usecase/UC-06/specification.md) | Multiple public paths per Environment, unique path and declared port; reconcile after readiness | form/planner/Ingress renderer and route-transfer tests; [kind BusyBox `/` + `/api`](../verification/2026-09-28-public-routes-recheck.md) |
| BR-13 | [Specification](../usecase/UC-16/specification.md) | Semantic no-op draft excluded from Preview; referenced UC-12 revision change remains update | `pending/preview_integration_test.go`, `pending/preview_test.go`; [kind Pod UID unchanged](../verification/2026-09-28-public-routes-recheck.md) |
| MS-03, BR-11 | [Specification](../usecase/UC-16/specification.md), [screens](../usecase/UC-16/ui/screens.md) | Resource Type inputs shown on form; required params validated and written into Score | `WorkloadEditorPage.test.tsx` PostgreSQL params test |
| MS-04–MS-05 | [Specification](../usecase/UC-16/specification.md) BR-02–BR-05 | UC-12, resource-output and same-Environment Service references; no literal binding or secret disclosure | Source eligibility, missing key/output/port, cross-Environment rejection and secret redaction |
| MS-06–MS-07 | [Realization](../usecase/UC-16/realization.md) | Validate then save pending desired change; current Deployment Set/runtime unchanged | Field errors, pending-save and no-runtime-mutation tests |
| VAR-01–VAR-02 | [States](../usecase/UC-16/ui/states.md) | Score import uses same rules; deletion requires confirmation and supports Undo | Literal-import rejection, import parity and pending-delete/undo tests |

## 2026-10-02 implemented validation policy — local verification

| Requirement | Realization / contract | Required coverage |
|---|---|---|
| UC-02 BR-05/BR-06, UC-03 BR-10 | Existing registration validators; OC-03/OC-04 | `TestRegisterResourceType_PublicIDPolicy`, `TestRegisterResourceDefinition_PublicIDPolicy`, `TestRegisterResourceDefinition_SeededShapesStillRegister`, `TestUC02UC03RegistrationPolicyHTTP`; frontend ID-error/form retention tests |
| UC-03 BR-11–BR-14 | Definition validation + DriverContractInspector; OC-04 | `TestRegisterResourceDefinition_StrictDriverInputs`, `TestRegisterResourceDefinition_PlaceholderAwareTyping`, `TestRegisterResourceDefinition_NamespaceNameFromTypeContract`; nested-placeholder rejection and required-string namespace exception regression coverage |
| UC-16 BR-14 | Shared Save/ValidateImport validation; OC-16 | `TestSaveAndImportValidateUnboundResourceParams`, `TestResourceParamValidationStoreFailureIsInternal`, `TestUC16ResourceParamsRejectedOnSaveAndImportHTTP`; editor Save/import rejection tests |
| UC-04 BR-07 | ADR-009; SecretStore variant of OC-05 | Accepted design only; AWS credential registration/execution not delivered by local hardening |

Execution: [2026-10-02 local validation evidence](../verification/2026-10-02-catalog-workload-validation-local.md).

## UC-16 existing Application-key selection — local implementation trace

| Requirement | Design / contract | Regression coverage |
|---|---|---|
| MS-04/MS-05, BR-15 | UC-16 checklist screens/sequence; OC-16 | Tick variable/secret with same-name reference, scoped keys, no value copy/secret display, no implicit selection |
| BR-16 | Optional name override; validation before serialization | Alias/reset, empty name and collision with Application/resource/Service names, allowed names across containers |
| BR-17 | Lossless Binding projection | Uncheck isolation, Edit/import aliases, multiple names for one key, unchanged advanced Score path |
| BR-18 | Unavailable/loading/error states; other sources editor | Missing key retained and blocks Save, empty/error distinction, Retry, resource/Service flows and submitting/stale guards |

Coverage is implemented in
[`WorkloadEditorKeys.test.tsx`](../../frontend/src/features/workloads/WorkloadEditorKeys.test.tsx):
same-name selections, explicit aliases/reset, variable/resource/Service
collisions, per-container isolation, Edit/import multiple aliases, missing
keys, catalog loading/error/Retry, stale reload and real-scope resets.
Deferred responses cover catalog, file read/parse, Save and reload;
prototype-member dictionary names remain own properties through serialization.
Execution evidence: [local picker verification](../verification/2026-10-02-workload-key-picker-local.md).


## 2026-10-02 human UI execution trace

[Reviewed recordings](../verification/2026-10-02-human-ui-kind-recordings.md)
exercise UC-12 registration, UC-16 BR-15 checklist selection and BR-17 Edit
restoration, UC-05/06 Preview/deployment, UC-02 Type registration/duplicates,
UC-03 ID/Driver Inputs validation, and UC-04 BR-05/BR-06 real read-only
connection verification to READY. Developer Preview consumes the newly
registered Definition without restart. This does not extend runtime support
for arbitrary Resource Types, AWS onboarding or production RBAC.

## ADR-012 implementation/verification

| Requirement | Canonical design | Required evidence |
|---|---|---|
| UC-04 SS-01..06 | ADR-012 Secret Store Connection | scoped Vault verify/register/redaction/probe cleanup tests |
| UC-04 SS-07..08 | ADR-012 local Compose registration bootstrap; UC-04 realization/sequence | `secretstores/bootstrap_test.go`, `bootstrap/platformvault_test.go`, `vault/bootstrap_test.go`, shared managed-admission persistence contract and [real Docker/video verification](../verification/2026-10-08-compose-vault-normal-store.md) |
| UC-01 ES-01..06 / BR-05..13 | ADR-012 versions/transitions | Settings CAS/busy, generation isolation, migration/recovery/cleanup |
| UC-12 BR-15..19 | ADR-012 copy/provider refs | copy failure atomicity, old refs preserved, variable conversion, VSO per-store |
| UC-05/06/08 admission | ADR-012 atomic owner claim | PostgreSQL concurrent processes, stale token before side effects, crash recovery |
| PostgreSQL migration | ADR-012 first-delivery scope | real kind record backup/restore/readiness/route cutover, human video |

The implementation and local gates above are complemented by the
[2026-10-08 real-kind human recording](../verification/2026-10-08-environment-stores-kind.md):
two stores, VSO rollout, stale-preview rejection, PostgreSQL restoration, generation
isolation, actual Ingress traffic, restart and explicit cleanup. The live evidence
uses two logical Connections on one physical cluster. Recovery/fencing failure
paths use local tests; no distinct-cluster DNS or AWS migration is claimed.

## Implicit internal cluster binding (ADR-013)

| Requirements | Collaboration / contract | Persistence / compatibility | Validation |
|---|---|---|---|
| UC-03 BR-17; UC-04 BR-17 | System-owned cluster binding, no per-cluster registration; ADR-013 | Reserved Definition key, preserve authored history | `catalog/policy_test.go`; `planning/connection_binding_test.go`; `provisioning/builtin_test.go` |
| UC-05 BR-12; UC-06 BR-21 | Enrichment creates internal cluster provider; bypass matching, visible in Preview; OC-07 | Deterministic binding/hash, no Preview writes | `planning/connection_binding_test.go`; `preview/service_test.go`; `ScorePreviewPage.test.tsx` |
| UC-08 BR-08; UC-07 internal compatibility | Existing-cluster executor through UC-08; OC-10; shared Kubernetes target | Idempotent FK admission, READY/org/kind checks, restore old targets, ADR-012 generation | `provisioning/builtin_test.go`; `deployment/builtin_cluster_test.go`; existing transition, cloud planning and output-contract suites |

Test paths are under `backend/internal/` unless the Console test is named.
Code navigation is in [code map](../implementation/code-map.md#implicit-internal-cluster-adr-013).
[Local evidence](../verification/2026-10-09-implicit-existing-cluster-local.md)
records Codex review/checks and fake-adapter browser restart.
[Real kind evidence](../verification/2026-10-09-k8s4f-live.md) covers retained
kubeconfig Connection registration and the isolated real-adapter Playwright
FE/BE/PostgreSQL flow, builtin binding and cleanup. No cloud or SQL-system-store
integration proof is claimed.
