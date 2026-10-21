---
id: TRACEABILITY-MATRIX
artifact: traceability-matrix
status: current
last_reviewed: 2026-10-21
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
| MS-03, MS-04 | same | generate Application identity/current metadata, no target/provider persistence or resolution | Application ID; new Environment targets UNCONFIGURED | OC-01; creation without default and no provisioning |
| MS-05, MS-06 | same | Environment and empty Deployment Set creation | exactly `staging` and `production`; `environments`, `deployment_sets` | OC-01; transaction rollback, HTTP and PostgreSQL persistence tests |
| MS-07, MS-08 | same | endpoint derivation, Application persistence | derived desired endpoints; atomic Application/Environment/current-set write | OC-01; no-deploy-side-effect frontend/HTTP/Playwright tests |
| UC-01 ES-01..06, BR-07..14; UC-05 BR-11; UC-06 BR-20; UC-08 BR-07 | scoped target resolution, shared snapshot, matching and executor binding validation | Historical ADR-011 Environment Settings set-once UI/API (superseded by ADR-012 rows below), resolver, `planning.ConnectionMismatch` + provisioning guard | generation-0 identities/legacy backfill; permanent lock superseded by ADR-012 | explicit choice, unsafe/foreign choice, mismatch rejection, [HTTP](../../backend/test/e2e/application_connection_http_test.go), [planner](../../backend/internal/planning/connection_binding_test.go), [execution](../../backend/internal/application/deployment/selected_target_test.go), ADR-011 migration/concurrency/save-guard/new AWS identity tests; [human Settings/live kind evidence](../verification/2026-10-07-environment-connection-kind.md) |

## UC-02

| Steps | Operation / sequence | Class methods | Persistence/state | Contract / tests |
|---|---|---|---|---|
| MS-01, MS-02 | `RegisterResourceType`; `UC-02/sequence.puml` | `catalog.Service.RegisterResourceType`, `resource.Type.Validate`, role-gated HTTP POST | none before validation | OC-03; `catalog/service_test.go`, `test/e2e/http_test.go` |
| MS-03, MS-04 | same | `ListResourceTypes/CreateResourceType` | insert-only `(org,key)` on memory/JSON/PostgreSQL | OC-03; registration contract, repository/service/HTTP race tests |
| MS-05, MS-06 | same | return `ResourceType` / repository read contract | persisted schemas | OC-03; planner catalog integration test; `ResourceTypesPage.test.tsx` |

T04 UI projection theo [UC-02 screens](../usecase/UC-02/ui/screens.md) giữ
BR-01/02/05/06 và OC-03. `ResourceTypesPage.test.tsx` kiểm schema GET, copy
độc lập/flags/types, duplicate giữ draft, tiếng Việt và whitelist/custom CTA;
`RegistrationStates.test.tsx` kiểm freeze/committed reload/no replay.
`ResourceDefinitionsPage.test.tsx`, `routes.test.ts` và `App.test.tsx` kiểm
query prefill theo catalog/whitelist, không POST và không ghi đè lựa chọn.
Scenario T04 kiểm details → copy/sửa → duplicate → đăng ký ID mới → supported
CTA và custom no-CTA trên backend fake; không mở rộng executor/API.

## Rendering implementation checkpoint — ADR-010 trước M04

| Requirement / boundary | Implementation | Local evidence |
|---|---|---|
| UC-03 BR-15/16 registration | `resource.ValidateRenderDefinition`, catalog bundle registry; Console `RenderingTemplatesPage` with PE/Admin gate and shared criteria editor | `catalog/rendering_test.go`, `test/e2e/rendering_http_test.go`; upgrade A → B: `planning/rendering_test.go`, `pending/rendering_test.go` |
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
| MS-06, MS-07 | same | `catalog.validateTerraform`, verified registry inspection; planner chỉ đọc bundle metadata | immutable module bundle + source fingerprint | OC-04; registry allowlist/transitive/digest tests, catalog mapping tests, PostgreSQL conflicting-pin race |
| MS-08, MS-09; BR-07 | same | `CreateResourceDefinition/ListResourceDefinitions` | insert-only org-scoped catalog; PostgreSQL Definition + criteria atomic transaction | OC-04; registration contract/race/criterion-failure rollback; no-restart HTTP Preview; explicit `{}` wildcard and criteria-shape conformance |
| BR-01, BR-03 | [ADR-016](../architecture/decisions/ADR-016-definition-selected-workload-targets.md), [ADR-017](../architecture/decisions/ADR-017-definition-dependency-execution.md) | Five-field criteria choose Definitions; references choose dependencies/targets; no profile predicate | Versioned Organization catalog; scoped original instance pins | `planning/placement_test.go`, `definition_catalog_v06_test.go`, `managed_only_graph_test.go`; historical profile tests retired with their contract |

T02 projection theo [UC-03 UI screens](../usecase/UC-03/ui/screens.md) giữ
BR-01/03/08/10–14 và contract đăng ký. `ResourceDefinitionsPage.test.tsx`
kiểm auto driver/module/profile, profile rỗng, READY/kind dropdown, giữ form và
safe errors; `RegistrationStates.test.tsx` kiểm freeze/committed reload/no replay.
Scenario T02 trong `frontend/test/e2e/refactor-local.mjs` kiểm product UI trên
backend fake; renderer page và renderer browser scenario được tách ở T03.
Execution: [T02 local verification](../verification/2026-10-10-T02-resource-form.md).

T02B projection theo [UI screens](../usecase/UC-03/ui/screens.md) giữ
BR-01/03/06/07: `CriteriaEditor.test.tsx` và
`ResourceDefinitionsPage.test.tsx` kiểm bốn chế độ, wildcard/nhiều dòng
round-trip, copy độc lập, giữ edits khi tải/lỗi/late response và profile tách
khỏi env_type. Cảnh báo overlap/xem trước phạm vi chỉ là trợ giúp frontend;
matching authoritative thuộc T19, matcher/contract đăng ký không đổi.


T03 projection theo [UI screens](../usecase/UC-03/ui/screens.md) giữ BR-15/16
và ADR-010: `RenderingTemplatesPage.test.tsx` kiểm workload-only list, payload
ngầm, không override/provision/source, ID/bundle errors và committed reload;
`App.test.tsx` kiểm PE/Admin/Developer gates, `routes.test.ts` kiểm deep link.
Scenario T03 kiểm đăng ký qua UI, native/selected renderer Preview và selected
render failure qua Deploy với fake infrastructure/delivery + CLI stub local.
Preview chỉ chọn renderer, không chạy render/provision; bundle selector được hoàn tất ở T18 bên dưới.

## T19 single-context matching preview

| Requirement | Collaboration / contract | Validation |
|---|---|---|
| UC-03 BR-20–22; OC-04c | `catalog.Service.PreviewMatching`, one scoped `ReadSnapshot`, same-key replacement in memory; domain BestCriterion and ConnectionMismatch; [sequence](../usecase/UC-03/sequence.puml) | `catalog/matching_preview_test.go`, `delivery/http/matching_preview_test.go`: weights/profile/winning ties, scope/target/system/native, safe errors and no mutation |
| UC-03 UI exact preview | shared `MatchingPreviewPanel` in CriteriaEditor/resource/template forms; full unsaved candidate + explicit context, separate transport status, snapshot warning | `MatchingPreviewPanel.test.tsx`, headed scenario T19: input/metadata/session/late reply guards, wildcard/class/tie/non-overlap/incomplete |
| UC-05/06 unchanged full graph boundary | separate fresh snapshot/planner validation; exact preview issues no token or admission | full Go/conformance suite; match.go and reference fixtures unchanged |

## UC-04

T06 CN-01/02 UI behavior: [canonical UI states](../usecase/UC-04/ui/states.md),
`ConnectionsPage.test.tsx` (auto-inspect/debounce, single/multi context, name
ownership, stale file-read/inspect, retry/unmount, safe errors), T06 headed local
scenario and [evidence](../verification/2026-10-11-T06-connections.md).
T06 preserved the API at that checkpoint; T22 now removes HOST_CONTEXT and
the old registration body under UC-04 BR-15.

Kubernetes upload implementation and local/isolated PostgreSQL tests pass on
2026-10-06. [Video verification and review](../verification/2026-10-06-uc04-kubeconfig-upload.md)
are recorded separately. AWS onboarding chưa implement tại checkpoint này; required behavior hiện
nằm ở UC-04 VAR-02/OC-05 và I04-06, không còn là deferred product scope.

| Steps | Operation / sequence | Class methods | Persistence/state | Contract / tests |
|---|---|---|---|---|
| MS-01..03; VAR-01 | inspect; `UC-04/sequence.puml` | HTTP Controller, InspectKubeconfig, parser | transient safe context metadata | OC-05; `TestInspectKubeconfig_ListsSafeMetadataOfValidContexts`, `TestKubeconfig_RequiresAPIVersionAndKind`, `TestUC04UploadHTTPContract`, `ConnectionsPage.test.tsx` |
| MS-04..05; BR-06..08 | upload register, verify | Service.RegisterKubeconfig, verifier.VerifyKubeconfig | read-only API/RBAC | OC-05; `TestNormalizeKubeconfig_RejectionsAreSafeAndCategorized`, `TestVerifyKubeconfig_ReadOnlyChecksWithSelectedContextOnly` |
| MS-06; BR-02,09,10 | credential Put | scoped CredentialStore | external immutable secret, no database bytes | OC-05; `TestConnectionCredentials_ScopedCreateOnlyLifecycle`, `TestConnectionCredentials_RejectsOutOfScopeReferencesWithoutRequests`, `TestCredentialResolver_FailsClosed`, `TestCredentialTarget_KubectlFailuresNeverEchoOutput` |
| MS-07; BR-01,15,16 | CreateConnection | repository + UnitOfWork | name/auth type, insert-only READY, default unchanged | OC-05; `TestRegisterKubeconfig_ConcurrentSameNameGetsDistinctKeys`, `TestFreshSchemaSeedsVerifyingKubeconfigAndRejectsHostContext`, `TestPostgresKubeconfigRegistrationRaceAndReopen`, current JSON Connection metadata/reopen checks (`adapters/store/load_test.go`) |
| MS-08..09 | public response/reload | DTO, ConnectionsPage | no response secret; catalog consumption | OC-05; `TestUC04UploadHTTPContract`, frontend registration/state tests, dedicated Playwright runner |
| ERR-07..08 | write/cleanup failures | CredentialStore.Delete | only own attempt cleaned with bounded context | OC-05; `TestRegisterKubeconfig_FailuresSaveNothingReady`, `TestUC04UploadWithoutCredentialStoreFailsClosed` |
| BR-10; POST-03 | execution credential resolution | resolver, Kubernetes adapters | opaque target identity only | `TestCredentialTarget_DeployerUsesScopedPrivateKubeconfig`, `TestCredentialTarget_RoutesVSOAndExecutorUseScopedCredential`, `TestExistingCluster_CredentialBackedConnectionIsAuthoritative`, `TestCredentialBackedTarget_RemovalAfterRestart`, `TestFleetRejectsCredentialBackedTargets` |
| VAR-02; BR-11..12,14 | AWS register cần I04-06 | AWS verifier/resolver, Terraform | ADR-009 credential namespace | designed; not implemented by upload slice |

## T22 explicit Kubernetes credentials

| Requirement | Collaboration / contract | Validation |
|---|---|---|
| UC-04 BR-15; OC-05 | upload-only HTTP/service; explicit auth/kind compatibility; scoped resolver | `TestUC04UploadHTTPContract` rejects old/mixed body before verifier/secret/row effects; `TestConnectionValidateRequiresExplicitMetadata`; resolver negative scope/ref/auth tests |
| UC-04 BR-10/15; UC-06 BR-20; UC-08 BR-07/08 | ExistingClusterAdapter and OpenCLI/CLI.Run reject host defaults; keep explicit Terraform EKS kubeconfig path | `TestContextOnlyTargetFailsClosedWithoutRunningKubectl`, `TestExistingClusterRejectsNonKubeconfigConnections`, `TestExplicitKubeconfigPathTargetStillRuns`; existing temp cleanup/cancellation/redaction/restart tests |
| UC-04 BR-15/16; schema default marker | seed KUBECONFIG/VERIFYING, no private ref; new Application remains UNCONFIGURED | `TestSeedKubernetesRecordIsVerifyingWithoutCredential`, `TestProductSeedClusterIsVerifyingAndApplicationCreateStillWorks`; SQL FK/authentication tests are opt-in, not executed in T22 |
| ADR-007 T22 boundary | Fleet context-only/scoped Connection targets unsupported, before Git/cluster mutation | guard tests and full conformance; no live Fleet proof |

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
| MS-04, MS-05 | same | `ExecutorRegistry.Resolve`, `ResourceExecutor.Provision` | baseline adapter state tại 3367f7e; current durable-state obligation ở I04-01/03 | OC-10; executor contract tests |
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

T15 VI-02/04/05 và UC-09 BR-01..07: [UI screens/states](../usecase/UC-09/ui/screens.md),
DeploymentDetailsPage và DeploymentPages tests kiểm nhãn Việt, snapshot IDs,
redaction, no-plan, safe errors, GET-only và obsolete scope/ID/attempt replies;
[T15 local evidence](../verification/2026-10-11-T15-deployment-details.md) phân biệt
backend fake và projection mocks, không external mutation.

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
| VAR-01–VAR-03 | [States](../usecase/UC-12/ui/states.md) | Warning but allow rename/delete; rename-only/delete retain references, explicit opt-in follows T30 below; secret replacement hidden | Warning/confirm, broken-reference Preview rejection and secret-update tests |
| BR-08–BR-10, BR-14 | [ADR-006](../architecture/decisions/ADR-006-application-configuration-provider.md), [ADR-008](../architecture/decisions/ADR-008-vso-native-secret-delivery.md) | Per-Application provider; immutable desired/applied revisions; VSO Secret with `secretKeyRef` on kind | Provider isolation, pending-vs-applied revision, VSO Secret synchronization and restart tests |

T24 / UC-12 BR-03, BR-08, BR-15–19 follows current ADR-012 rather than the
historical per-Application provider above. Domain/repository validation rejects
store-backed Variables; configuration service spy tests prove no store calls for
Variable CRUD/list alongside Secrets, safe DTOs, Secret-only copy, preserved
Variables/old owning-store refs and failed-copy/version/BUSY safety. HTTP tests
cover configuration redaction and invalid old-format rejection. See
[T24 local evidence](../verification/2026-10-12-T24-metadata-variables.md).

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
| UC-04 SS-01..06 | ADR-012 Secret Store Connection; [T05 UI](../usecase/UC-04/ui/screens.md) | scoped Vault verify/register/redaction/probe cleanup tests; `SecretStoresPage.test.tsx`, T05 headed fake scenario (same/different address, validation, token clear, backend/auth projection), [evidence](../verification/2026-10-11-T05-secret-stores.md) |
| UC-04 SS-07..08 | ADR-012 ordinary Compose registration bootstrap; UC-04 realization/sequence; T23 removes legacy conversion/backfill | `secretstores/bootstrap_test.go`, `bootstrap/platformvault_test.go`, `vault/bootstrap_test.go`, shared managed-admission persistence contract; [historical real Docker verification](../verification/2026-10-08-compose-vault-normal-store.md) predates T23; current local checks in [T23 evidence](../verification/2026-10-12-T23-ordinary-secret-stores.md) |
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

Historical matrix for baseline `7a00e43`, superseded by ADR-016/V03.10.
Builtin admission/tests below were removed with that runtime route; current
workload placement and persistence are traced in the V03 section.

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

## T07 — Environment Settings frontend trace

| Requirement | UI design | Implementation/checks | Evidence |
|---|---|---|---|
| UC-01 ES-02/03, ENV-01 | UC-01 UI screens/states T07: single READY draft only, explicit Save; no default/fallback | EnvironmentConnection and SecretStoreSelection; unit tests for one/many/none, existing binding, stale/reload and scope | [T07 local verification](../verification/2026-10-11-T07-environment-settings.md) |
| UC-12 BR-03/05/16..19; VI-02/04/05 | UC-12 UI screens/states T07: masked Secret, rename warnings, copy failure/BUSY, retained editor | SettingsPage/environment tests; headed T07 fake-adapter and marked mock scenario | [T07 local verification](../verification/2026-10-11-T07-environment-settings.md) |

## T09 — Tạo ứng dụng frontend trace

| Requirement | UI design | Code/test | Evidence |
|---|---|---|---|
| APP-01, UC-01 MS-01/02, BR-01/02/14 | UC-01 UI screens/states T09: gợi ý slug, giữ manual edits, name/subdomain only | CreateApplicationPage và tests; T09 UI tạo/trùng/validation/payload/navigation | [T09 local verification](../verification/2026-10-11-T09-create-application.md) |
| VI-02/04/05 | Vietnamese labels/errors, retain draft, explicit retry | 309 frontend tests; headed fake T09, async 500 mock được ghi rõ | [T09 evidence](../verification/2026-10-11-T09-create-application.md) |

## T08 — Workload editor frontend trace

| Requirement | UI design | Implementation/tests | Evidence |
|---|---|---|---|
| WL-01/02, UC-16 MS-03/BR-11/14 | UC-16 UI screens/states T08: advanced resources/class/target-port, effective summaries | WorkloadEditorPage and editor tests: defaults omitted, typed number/bool, imported overrides retained | [T08 local verification](../verification/2026-10-11-T08-workload-editor.md) |
| UC-16 BR-12, VI-02/04/05 | Explicit Add-route, one valid port suggestion, invalid target retained | Editor tests and headed T08: unique/multiple/invalid ports, no PUT on invalid route, no retarget | [T08 local verification](../verification/2026-10-11-T08-workload-editor.md) |
| UC-16 BR-15–18 | Vietnamese key picker, no implicit keys, aliases/scope guards | WorkloadEditorKeys regressions; T08 mock Secret catalog/stale/BUSY explicitly marked; T03 human-helper regression | [T08 local verification](../verification/2026-10-11-T08-workload-editor.md) |

## T11 — Danh sách ứng dụng frontend trace

| Requirement | UI design | Code/test | Evidence |
|---|---|---|---|
| VI-02/04/05 | UC-01 UI screens/states T11: Vietnamese cards, accessible loading/empty/error/retry, unchanged data/routes | ApplicationsPage; App tests về list order, hai create CTA, explicit retry và initial deep link; headed T11 reload/card flow | [T11 local verification](../verification/2026-10-11-T11-applications-list.md) |

## T13 — Xem trước Score frontend trace

| Requirement | UI realization | Checks | Evidence |
|---|---|---|---|
| VI-02/04/05; UC-05 standalone boundary, BR-04 | UC-05 UI screens/states T13: Vietnamese form/results/errors, read-only Preview, input/scope guards | ScorePreviewPage 20 tests; headed T13 valid/invalid Score, graph/docs, error/retry/session/scope and no draft/deployment requests | [T13 local verification](../verification/2026-10-11-T13-score-preview.md) |
| UC-05 Score input contract | Parser messages only; YAML/JSON keys and parsing/alias rules unchanged; API action values retained | Parser object/document/alias/raw-text checks; add/update/remove payload assertions | [T13 evidence and mock limits](../verification/2026-10-11-T13-score-preview.md) |

## T12 — Trang ứng dụng frontend trace

| Requirement | UI design | Code/test | Evidence |
|---|---|---|---|
| VI-02/04/05, UC-16 BR-01/07/10, UC-07 Console boundary | UC-01 screens/states T12; UC-07/16 screens T12: Vietnamese pending actions, explicit Deploy, confirmation/Undo, safe errors | ApplicationHomePage tests; headed draft/preview/fake deploy/delete/undo/stale; BUSY and late-scope unit guards | [T12 local verification](../verification/2026-10-11-T12-application-home.md) |
| UC-01 BR-04/05, VI-02/04/05 | Independent target summary, recent deployments labels and states | ApplicationTarget, RecentDeployments; recent error/retry/order/5-item/link/obsolete response tests | [T12 evidence](../verification/2026-10-11-T12-application-home.md) |

## T16 — RD-02 Terraform account inheritance

| Required behavior | Design/implementation | Verification |
|---|---|---|
| UC-03 BR-08: reusable empty VPC/EKS key; reject every explicit key before insert | OC-04; UC-03 sequence; catalog registration and new seed | `TestRegisterResourceDefinition_TerraformVPCAndEKSRequireEmptyConnection`, `TestTerraformVPCEKSRegistrationRejectsConnectionHTTP`, `TestResourceDefinitionsVPCAndEKSHaveNoConnection` |
| UC-06 BR-20: pinned Environment target/hash, malformed winning conflict without fallback | ADR-011/013; unchanged matcher; UC-06 cloud sequence | `TestPlan_AWSVPCAndEKSInheritIndependentEnvironmentTargets`, `TestPlan_MalformedWinningVPCOrEKSKeyIsRejectedWithoutFallback`; conformance unchanged |
| UC-08 BR-07: effective READY org/kind account; persisted identity; Aurora explicit; no cross-account reuse | OC-10; UC-08 sequence; provisioning service | `TestProvision_AWSVPCAndEKSInheritEnvironmentConnection`, `TestProvision_AuroraKeepsExplicitAccountAndNeverInherits`, `TestProvision_ReuseNeverCrossesAccounts`, `TestProvision_EffectiveAWSConnectionUnavailableFailsBeforeExecutor` |
| UC-03 UI: VPC/EKS no selector, empty payload across type switches; Aurora selector retained | UI screens/states/API mapping; ResourceDefinitionsPage | page unit tests, headed fake T16 and T02 regression; [dated evidence](../verification/2026-10-11-T16-environment-aws-account.md) |

SQL opt-in and real AWS/cluster verification are not claimed by local evidence.

## T17 — public existing-cluster registration closure

Historical closure at `7a00e43`. V03/ADR-016 replaces this restriction with
public Definition registration and normal matching; old builtin test paths
refer to that historical commit, not current runtime.

| Requirement | Contract / implementation | Validation |
|---|---|---|
| UC-03 BR-08/BR-17; ADR-013; OC-04 | Catalog rejects every public existing-cluster driver before insert; reserved key guard retained | `catalog/registration_test.go`, `catalog/policy_test.go`, `backend/test/e2e/uc02_04_http_test.go` |
| UC-06 BR-21; UC-08 BR-08 | Trusted binding/executor and FK admission unchanged | `planning/connection_binding_test.go`, `provisioning/builtin_test.go`; full conformance unchanged |

[T17 evidence](../verification/2026-10-11-T17-existing-cluster-registration.md)
records local checks; no new browser or live/database integration proof.

## T18 installed bundle list and Console selector

| Requirement | Realization/code | Verification |
|---|---|---|
| UC-03 BR-18; OC-04b: PE/Admin session org, safe immutable registry metadata only | Catalog ListRenderBundles, shared RenderBundleAvailable; GET handler | TestRenderBundlesAuthAndRole, TestRenderBundlesEmptyIsArray, TestRenderBundlesSortedSafeMetadataOnly |
| BR-15/18: no persistence/external calls; POST revalidates availability | Registry query and registration share predicate | TestRenderBundlesReadOnlyAndRegistrationRevalidates; unavailable real local POST in T18 |
| BR-19: 0/1/n, loading/error/retry, preserve selection/draft, stale option blocked | RenderingTemplatesPage selector; UI screens/states/API mapping | RenderingTemplatesPage.test.tsx; headed T18 with explicit projection mocks |
| ADR-010 unchanged selection/payload, Preview pure | Existing registration/planner; T03 locator adaptation | Headed T18 selected template Preview, T03 regression; Go conformance |

Evidence: [T18 local verification](../verification/2026-10-11-T18-render-bundles.md).

## T20 — VI-03 public error codes

| Requirement | Design/code | Verification |
|---|---|---|
| Stable codes/fields/safe params, no raw provider/submitted secrets | Public HTTP error contract in OC; apierror.go and typed error producers | apierror_test.go canary/status/code/field/params/decoder tests; verification category tests; HTTP/e2e suite |
| Vietnamese known/unknown errors for all Console flows | Shared UI error design and UC-00/01/02/03/04/05/07/09/12/16 mappings/states; shared client/errors | client.test.ts malformed/prototype/unknown/401/PUT+POST no-replay; page tests and headed T20 |
| Preserve scoping/CAS/BUSY/stale/reload safety and selected renderer behavior | Existing service/planner contracts; code-first page controls and authoritative reload gates | Go conformance; scope-switch/reload-failure/retained-edit frontend tests; T01 regression; fake T20 two-tab stale and labelled BUSY/copy/unknown mocks |

Evidence and exact limits: [T20 verification](../verification/2026-10-12-T20-console-errors.md).

## T21 — Context alias removal (LEG-04)

| Requirement | Implementation | Verification |
|---|---|---|
| UC-03 Environment target references; ADR-011; OC-07 context rule | `planning.Context.Values/ResolveContext`; only env.profile/env.region resolve target values, no alias rewrite | `TestContext_AppProfileRegionAliasesAreUnknown`, `TestContext_EnvProfileRegionIgnoreLegacyApplicationFields`, `TestContext_ValuesResolveTheSelectedEnvironment`; [T21 evidence](../verification/2026-10-12-T21-context-aliases.md) |

## T23 — Ordinary workload Secret Stores (LEG-06)

| Requirement | Implementation/test | Evidence |
|---|---|---|
| UC-04 SS-06..08; UC-12 BR-15; ADR-012 | Scoped private credential-only Vault registry, no legacy flags/backfill/conversion; `TestRegistryCredentialResolutionFailsClosed`; bootstrap fresh/restart/refresh, probe cleanup and failed-attempt cleanup tests | [T23 local evidence](../verification/2026-10-12-T23-ordinary-secret-stores.md) |
| UC-04 SS-04,08; managed credential CAS | SQL/JSON managed admission checks ID/ref/marker/endpoint; shared persistence contract rejects unmarked user refresh; public DTO has no legacy/token/ref; frontend ordinary labels | Same evidence; SQL integration skipped by opt-in gate, headed fake T23 and regression coverage recorded separately |

## T25 — Public route alias removal

| Requirement | Design/code | Verification |
|---|---|---|
| UC-16 BR-12; UC-06 public routing; OC-07 | `Service.UnmarshalJSON`, Score parser, workloadconfig Save/ValidateImport/List: only publicRoutes, no conversion or state write on alias error | `TestServiceRejectsAnyPublicPort`, `TestPublicPortIsRejectedEverywhere`, `TestPublicPortIsRejectedWithoutWritingDraft`, `TestListFailsOnDraftWithPublicPort`, `TestHTTPRejectsRemovedPublicPortWithoutStateChange` |
| Strict Score input; VI-03 | Nested Service fields stay strict; stable INVALID_SCORE/service.publicPort and safe Vietnamese mapping | `TestServiceDecodingIsStrictAtEveryDepth`, `TestScoreRejectsUnknownServiceKeysAtEveryDepth`, HTTP/classifier and editor tests |
| T08 route controls; explicit Save → Preview | Multi-route import/edit/save/reopen, no serialize alias or fallback; protected literal policy unchanged | Full backend/frontend suites, headed T25 and T08 regression; [local evidence](../verification/2026-10-12-T25-public-routes.md) |

## T26 — Application metadata ownership (LEG-01)

UC-01 BR-04/MS-03..06, UC-06 PRE-02/MS-04/BR-20 and OC-01 map to the
identity-only Application domain/create service and SQL/JSON repositories.
`domain/application/application_test.go`, Application service tests and
`adapters/store/load_test.go` (T28 replaces retired-target load tests) assert serialization, no binding fallback
and safe retained-target rejection. Shared `persistencetest/binding.go` verifies
independent Environment selection and metadata save/reopen. `workloads.go` plus
JSON workload-history tests verify historical Connection/generation/TargetRef
survive Application saves and reopen. PostgreSQL counterparts were skipped at the T26 checkpoint; the current baseline
counterparts pass on the authorized T28 disposable server. Existing unconfigured Preview/Deploy
and conformance suites pass; fixtures/app.id and scope semantics are unchanged.
See [T26 evidence](../verification/2026-10-12-T26-application-metadata.md).

## T27 — Infrastructure binding scope cleanup (LEG-02)

| Requirement | Design/code | Verification |
|---|---|---|
| ADR-011/T27, UC-06 BR-02/20, UC-08 BR-07; ENVIRONMENT-only admission | Environment domain/Validate, target resolver, planner and JSON/SQL persistence scope guards | scope_test.go; TestPlan_RejectsEveryNonEnvironmentScopeWithoutEchoingIt; target negative cases; shared binding no-write checks; TestRetiredInfrastructureScopeIsRejected |
| ADR-012 generation contract, ADR-013 internal cluster boundary | Environment/generation @infra path and bounded AWS names; preserved internal cluster Connection descriptor/Application resource scope | TestResourceName_GenerationZeroIdentityUnchangedAndLaterGenerationsIsolated; existing target-generation and builtin cluster suites |
| UC-03 seed boundary; no exact old-template upgrades | Explicit ENVIRONMENT seed, missing-only Definition refresh | TestApplyBindsAcceptanceFixturesWithExplicitEnvironmentScopeAndIsIdempotent; TestApplyPreservesExactEarlierSeededAWSDefinitions; authored-definition preservation tests |
| Conformance boundary; matching specificity/ties unchanged | Harness ENVIRONMENT context, no additional historical path/name adapter needed | Full 33 accepted/rejected fixtures including 14/22/25/26; match.go and reference fixtures unchanged |
| Database schema ENVIRONMENT-only constraint | T28 fresh baseline CHECK and safe reset-required startup error; no conversion | SQL constraint/reopen/rollback coverage; T27 checkpoint skipped SQL opt-in; current baseline verification belongs to T28 |

Settings removes the retired binding label; frontend gates and headed fake T07
regression pass. See [T27 verification](../verification/2026-10-12-T27-environment-scope.md).

## T28 — Current persistence baseline (LEG-08)

| Requirement | Design / implementation | Required verification |
|---|---|---|
| Current schema, no startup upgrade/backfill | [Schema baseline](../architecture/database/schema.md#fresh-persistence-baseline-t28--leg-08), ERD, ADR-011/012 supersession; PostgreSQL baseline.sql/baseline.go | Fresh initialization, second boot, concurrent initialization, rollback, old layout/ledger removal rejection and schema drift without repair |
| LEG-01/02/05/06/07 persistence boundaries | Identity-only Application, ENVIRONMENT-only scope, explicit Connection metadata, ordinary Store, Variable/Secret CHECK | Real disposable PostgreSQL constraint, binding, registration, operations, multi-backend/fencing/race/reopen suites |
| Current JSON reopen; missing-required data fails safely | store/load.go; no default or history reconstruction | Current/empty format roundtrip, missing/null aggregates, retired fields and invalid identity/binding/config rejection; unchanged rejected file |
| UC-09 BR-01/04, historical snapshot safety; ADR-002 | UC-09 realization and preserved per-Deployment history, FK/deferred Snapshot/CAS boundaries | Current immutable history/reopen tests; missing history rejected rather than fabricated; transaction and read-snapshot suites |
| External-state gate | [Disposable runner](../operations/docker-local.md#t28-fresh-baseline-and-isolated-postgresql-verification), [T29 backup/reset preparation](../operations/k8s4f-dev.md#t29-backupreset-gate) | Separate approval for DB test mutation, owned cleanup evidence; no retained reset or browser/live proof in T28 |

T28 validation: authorized isolated PostgreSQL 16 full suite/build/conformance,
zero skipped tests, targeted persistence race, fresh/reopen/rollback/drift and
multi-backend guards pass. Synthetic restore and owned cleanup pass; no retained
mutation or browser flow. [Reviewed evidence](../verification/2026-10-12-T28-fresh-baseline.md).

## T30 — Atomic key rename and desired references (CFG-01)

| Requirement | Collaboration / implementation | Validation |
|---|---|---|
| UC-12 VAR-01, BR-05, BR-20–23; OC-12 T30 | `configuration.Service.List` consistent snapshot + `RenameKey` preparation; `persistence.DesiredRename` and memory/JSON/PostgreSQL atomic `CommitDesiredRename` | `configuration/rename_test.go`, `persistencetest/rename.go`, JSON snapshot rollback/reopen tests; PostgreSQL opt-in `rename_integration_test.go` prepares mid-write rollback/BUSY/multibackend/reopen coverage |
| UC-16 BR-19; UC-12 BR-21/22 | Shared `workloadconfig/rewrite.go`, `ReconstructVerified` + `VerifyRewritten`; upsert precedence, pending delete exclusion; preserve aliases/advanced/context/escaped refs | `reconstruct_verified_test.go`, `configuration/rename_test.go`: deployed no-draft, many-to-one, cross-env, unsafe/collision/no-provider and one-winner race |
| UI defaults/CAS/errors/scoping | Settings checkbox + snapshot guards, scoped pending notice/Preview CTA; strict HTTP PATCH and fixed REFERENCE_REWRITE_UNSAFE code | `configuration_rename_test.go`, `SettingsRename.test.tsx`, route/ApplicationHome tests; headed T30 real product API with fake adapters, explicit BUSY projection |
| UC-05/07 pending boundary | Existing OC-17 pins config/draft/current state; rename does not deploy | T30 headed Preview lists api/web; pre-rename token rejected; rename-only missing reference blocks Deploy; existing pending/transition/deployment regressions |

Execution results and PostgreSQL gate are recorded in the
[T30 verification](../verification/2026-10-12-T30-key-rename.md); prepared opt-in
tests are not execution proof.

## T31 — Typed Definition authoring and capability metadata

| Requirement | Collaboration / contract | Validation |
|---|---|---|
| UC-03 BR-23/24; OC-04d | ListDefinitionCapabilities, shared registration pairs, embedded inspector/static executor policy, scoped param source | catalog/capabilities_test.go; HTTP definition_capabilities_test.go; e2e definition_capabilities_http_test.go: auth, defaults/types, executor-owned exclusion, no-write/safe error, required-string namespace source |
| UC-03 BR-25; BR-11–14 preserved | TypedVariablesEditor, typedVariables, per-pair ResourceDefinitionsPage draft | TypedDefinitionForm.test.tsx, typedConversion.test.ts: number/list/placeholders/escaped and padded strings, any, synthetic bool/map, omission and lossless JSON |
| UC-03 BR-26; UC-06 BR-04/08, UC-08 BR-01 unchanged | ProvisionEditor/typedProvision, explicit matching snapshot and full Score Preview | T31 headed fake: stored flags/params/refs, consumer → provider reference and is_dependent edges, provider-first batches, no implicit Preview/Deploy |
| UC-02 support/CTA; UC-03 loading/error/prefill | capability client + catalog lists, no frontend whitelist fallback | ResourceTypesPage/TypedDefinitionForm tests; headed T31 metadata 500 projection/retry and confirmed CTA/prefill |

Execution evidence: [T31 local verification](../verification/2026-10-12-T31-typed-definitions.md).

## Required design-to-test obligations — M04

Các sections UC/T ở phía trên là implementation/evidence checkpoint trước M04,
đọc theo requirement version tại a897d6b/3367f7e; chúng không chứng minh luật
hiện tại đã đạt. Bảng dưới là current obligations.
[Reconciliation](../implementation/v2-rule-reconciliation.md) map mọi V2 ID
gộp sang BR/MS/VAR/OOS giữ nguyên ID và nêu nội dung đã đổi. Không sửa old
fixtures/video để giả bằng chứng mới. Các tests/scenarios dưới đây còn pending.

| Requirements hiện tại | Operation / collaboration | Data / required tests | M04 task |
|---|---|---|---|
| UC-02 BR-02..04; UC-03 BR-08..16/23..26, VAR-01/02 | OC-03/04/04d; UC-03 sequence/VOPC; ModuleSourcePolicy/BundleResolver | immutable bundle/source mappings, typed schemas, allowlist/digest/traversal/command/secret rejection; insert-only race/no query fetch | I04-02/04/05/07 |
| UC-04 VAR-02, BR-02/09..12 và V2-04; UC-08 BR-07 | OC-05/10; credential resolver, UC-04/08 sequences | scoped Vault refs/account/region/opaque EKS; failed registration cleanup, no host fallback, target reopen/cross-account rejection | I04-03/06 |
| UC-03 BR-02/15/21; UC-05 MS-06/BR-04/09..11/V2-04; UC-06 MS-09/11/BR-09/17..20 | OC-06/07/08/17; planner/preflight/score-k8s | required renderer both profiles, no match/tie/fallback, stable module/provider/state/target pins, stale/no-secret projection, non-destructive update | I04-02/03/07/10 |
| UC-08 MS-02/05/06, BR-02/03/05, V2-04/06/07; UC-06 V2-05 | OC-10/18; state/instance/attempt ports, UC-08 sequence/VOPC, ERD | real PostgreSQL ciphertext/reopen/rollback, serial replay/lineage rejection, two-owner lock/fence race, no timeout unlock, stable names/password; no duplicate unknown CREATE | I04-01/03/10 |
| UC-07 BR-03/VAR-02/V2-02/04; UC-08 BR-05 | OC-09/19; retained resource/cleanup engine and original targets | no destroy on Workload remove, shared conflict/reference preservation; test run rejects preexisting/updated/platform, verified reverse graph destroy | I04-03/08/11 |
| UC-07 V2-03, DEL-PRE-01/02, DEL-MS-01..04, DEL-BR-01/02 | OC-19 PrepareDeletion/admission; UC-07 sequence-resource-delete/VOPC/UI | PE/Admin vs Developer, scope/consumer/UNREFERENCED/busy/exact name, root-only default, explicit multi-selection, disabled consumer blockers/cascading unchecks, atomic whole-set refusal, unchecked items preserved, token versions/actor replay; no provider call on stale/refused | I04-01/08/11 |
| UC-07 DEL-MS-05..07, DEL-BR-03/DEL-POST-01; UC-09 RES-01..03, BR-05/07 | OC-19 execution + OC-11 detail/progress; UC-08 cleanup sequence | original state/module/target, resource dependency edges, safe stage/absence/remaining IDs; failed consumer retains provider, immutable history; A01 real UI delete proof | I04-03/08/11 |
| UC-16 MS-03/BR-07/11/14/V2-02/03; UC-05 BR-10 | OC-16/17; Workload editor and UC-03/05/16 UI artifacts | Type params/class/shared IDs, advanced/import roundtrip, metadata/stale guards, no module selector/provider credential in Workload; E01 no-create | I04-07/10/11 |
| UC-06 V2-06; runbook V2-A01/A02/A03/E01/E02 | Console form -> Terraform -> binding -> score-k8s -> delivery -> app UI | real full job/completion/reload + Redis hit/miss/refill and PostgreSQL result, headed video/phases/assertions; UI deletion A01; independent cleanup/absence proof | I04-09/10/11/12 |
| ADR-014/M04 gates và D12/D13/D14 boundary | Claude cloud → local clauded/tmux khi hết credit → Codex gpt-6-luna nếu local hết; Codex independent local verification, manual stopped-owner checks | No code before gate 1, no AWS create before I04-11 A01/E01/E02 pass + saved video/estimate + clean k8s-4f (gate 2 removed 2026-10-14), no false final approval; no auto-resume/import/DR claims | I04-00/11/12 |

Bước xóa UI là một phase của A01, không mở rộng thành live update/delete
exception matrix mới. Executor create/update/destroy và SQL/admission failure
contracts vẫn cần code tests dù không thành live scenarios riêng.

## I04-01 partial code evidence — 2026-10-13

OC-18/state persistence and fencing contract tests now have
[partial real PostgreSQL evidence](../verification/2026-10-13-i04-01-state-backend-partial.md)
from Claude and independent Codex runs. DB-permission separation remains open;
this does not close I04-01 or IMP-021 and is not live acceptance evidence.

## I04-01 option A evidence

Owner/Console/State DB privilege boundary in ADR-014/state/schema now has
[real-login PostgreSQL/CLI evidence](../verification/2026-10-13-i04-01-role-a.md).
DB transport TLS scope remains open; this does not close I04-01/IMP-021 or
provide any live UI/k8s/AWS acceptance proof.

## I04-01 final review complete

[Final evidence](../verification/2026-10-13-i04-01-complete.md): malformed-key 400/no state confusion regression, mandatory missing SQL fails, 835 real PostgreSQL tests zero skip. I04-01 complete; TLS deferred issue #2, I04-02 begins. No live acceptance claimed.

## I04-02 registry review

[Registry evidence](../verification/2026-10-13-i04-02-module-registry.md) covers
UC-03 MS-05..09/BR-08/11/12/23..26: approved source pins, static inspection,
typed mappings, secret classification, immutable insert/race and metadata-only
Preview/capability reads. Codex verified 884 tests with real PostgreSQL and zero
skips. Executor integration remains I04-03; class/form UI remains I04-07.

## I04-03 executor review

[Executor evidence](../verification/2026-10-13-i04-03-terraform-executor.md):
UC-08 BR-05/07 và OC-10/18/19 có local tests cho stable identity, scoped provider
pins, durable state, nondestructive UPDATE, unknown-outcome hold và ownership.
UC-07 DEL-MS-05..07/DEL-BR-03 có destroy engine tests cho original module/target,
namespace UID/provenance, remaining IDs và retry/restart sau state rỗng.
Codex xác minh 970 tests trên PostgreSQL thật, không skip; provider seams fake
được phân biệt trong evidence. Delete UI/API và live proof còn I04-08/11.

## I04-04 Kubernetes modules review

[Module evidence](../verification/2026-10-13-i04-04-kubernetes-modules.md):
UC-03 BR-08..16/23..26 và UC-08 BR-05/07 có registry/CLI tests cho immutable
module/image/provider pins, secret outputs, stable identity và state-owned PVC.
Module contract no-secret-to-nonsecret rule có catalog/planner/provisioning
regressions cho Type/module/runtime classification, nested/interpolated refs,
no consumer invocation/no metadata leakage và nonsecret A01 graph fixture.
Codex verified 1021 tests trên PostgreSQL thật, không skip. Init/validate/plan
và Docker image tests chưa thay live k8s/EKS/cleanup proof ở I04-11/12.

## I04-05 shared private network decision

Người dùng chốt UC-02 BR-02 bổ sung optional VPC `privateSubnetIds`; UC-08
BR-01 và OC-10 resolve subnet outputs từ cùng provider VPC node. ADR-014/module
contract giữ private network ownership trong state VPC, consumer cleanup không
xóa shared subnets. I04-05 phải kiểm nhiều Aurora/ElastiCache cùng VPC và reverse
cleanup; local implementation verified theo evidence I04-05; live verification chưa đạt.

## I04-05 AWS module review complete

[AWS module evidence](../verification/2026-10-14-i04-05-aws-modules.md):
UC-02 BR-02/UC-08 BR-01 và OC-10 có privateSubnetIds mappings, hai Aurora/hai
ElastiCache cùng VPC node, private network ownership và cleanup consumer guards.
ADR-014 EKS `/32` allowlist, immutable module/provider/addon pins và input
validation có registry/planner/actual CLI tests. Codex verified 1042 tests trên
PostgreSQL thật, không skip; provider plan seam mock. Preflight runner wiring,
UI registration và live AWS proofs còn I04-07/10/11/12.

## I04-06 AWS Connection review complete

[Verification](../verification/2026-10-14-i04-06-aws-connection.md) covers UC-04
VAR-02/BR-01/02/09/10/11/12/16, ERR-05..09 và OC-05: bounded role/org-gated
onboarding, STS/region/capability evidence, immutable scoped Vault ref, safe DTO,
SQL race/rollback/reopen and unchanged default/Kubernetes behavior. Verifier
fail-closed tests cover incomplete metadata, regional create denial, IAM global
context, missing context/paging/org denial, redirects and malformed allowed regions.
Codex: 1082 Go tests/zero skip, 479 frontend tests, actual SQL/Vault + headed
browser with AWS transport fake, credential scans and verified cleanup. Live
AWS identity/capabilities and execution preflight remain I04-10/12; no v2 live
acceptance claimed. Class/source forms remain I04-07.

## I04-07 metadata mapping clarification

UC-16 V2-02/03 và existing Reference catalog collaboration dùng GET resource-types
computed classes, không Type persistence/POST field hoặc privileged module
selector cho Developer. UC-03 OC-04d supplies verified source/mapping/provider
metadata for PE/Admin. [API mapping](../usecase/UC-16/ui/api-mapping.md#metadata-class-và-source--i04-07)
clarifies role/no-side-effect/error/default/custom guards; implementation/tests
pending I04-07, không claim delivery từ contract clarification.

## I04-07 local review complete

[Verification](../verification/2026-10-14-i04-07-definition-workload-ui.md)
hiện thực UC-03 BR-08/11/23 và OC-04d source/mapping/provider metadata;
UC-16 V2-02/03 class/default/custom/shared IDs; UC-05 BR-01/10 pure consistent
pending/standalone Preview, CREATE/UPDATE/RETAIN/UNVERIFIED/BIND intent;
UC-09/ADR-014 safe persisted attempts/column grants và required renderer trên
hai profiles. Codex 1121 Go/0 skip, 549 frontend tests, build/vet/docs, headed
SQL/Vault scenario và cleanup đạt. Attempts video dựng sẵn; physical resource
proof còn I04-10..12. Không sửa 33 reference fixtures hoặc dùng bypass để đạt.

## I04-08 local review complete

[Verification](../verification/2026-10-14-i04-08-resource-delete-ui.md) hiện thực
UC-07 V2-03/04 và DEL-PRE/MS/BR/POST, UC-09 RES-01..03/OC-19: scoped safe reads,
role denial trước plan, root-only default/opt-in/cascading unticks/exact name,
atomic whole-set admission/REQUESTED, stale/busy/consumer guards/idempotency,
original target/region, reverse graph/partial retention và operation-bound proof.
UC-05 BR-10 CREATE intent sau verified destroy; metadata chưa đồng bộ fail closed.
Codex: 1171 Go/0 skip, 609 frontend/allchecks, real engine/CLI/encrypted SQL/Vault
headed scenario và cleanup đạt. FAKE objects/absence, không live A01; chọn thêm
một dependency và giữ một candidate không chọn trên hạ tầng thật còn I04-11.

## I04-09 local review complete

[Verification](../verification/2026-10-14-i04-09-acceptance-redis.md) hiện thực
Redis job flow của RUNBOOK-TERRAFORM-V2-ACCEPTANCE và outputs auth/TLS của
TERRAFORM-MODULE-CONTRACT: commit/write-through, hit/miss/refill, TTL/prefix,
no duplicate/cache failure diagnostics và per-job proof không false-pass.
1254Go/0skip,609frontend/fullchecks/6image manifests/cleanup đạt. Không video
unit hoặc live A01/A02/A03; binding/runner/live proof còn I04-10..12.

## I04-10 local review complete

[Biên bản](../verification/2026-10-14-i04-10-integration.md):1351Go/race/0skip,
622frontend/fullchecks. Real-profile, durable Vault outputs, original-engine
cleanup với claims, process-kill proof, strict AWS identity và ECR/probe ownership
đã có code/test. Fake-backed provider/readiness tests không thay A01/A02/A03/E01/E02
live evidence; I04-11 current, I04-12 gated. IMP-021 còn mở, D12–D14 không đóng.

## I04-11 live partial, chưa đạt

[Biên bản](../verification/2026-10-14-i04-11-k8s4f.md): code fix `da524c6`
merge `72a807e`,1352Go/0skip,622frontend/fullchecks. A01 real adapters qua UI
đạt Preview/deploy/Ready/shared bindings/Vault/job/cache write/hit/TTL/miss/
refill/reload; delete thiếu native confirmation nên retry fail. Run đầu đã
CLOSED_CLEAN; retry namespace cleanup còn bị chặn bởi default SA provenance,
không false-pass. E01/E02/A02/A03 chưa chạy, AWS gate đóng, IMP-021 còn mở.

Quyết định2026-10-15 cập nhật AGENTS/ADR-014/runbook/M04: thất bại phải tìm
nguyên nhân và sửa/check rồi chạy lại sau cleanup sạch, bỏ giới hạn một retry
scenario. Không thay kết quả fail lịch sử, ownership guards hoặc policy retry
apply có unknown outcome. Namespace r2 còn chờ quyết định ngoại lệ.

Quyền bổ sung2026-10-15: dọn ngoại lệ đúng namespace/UID r2 sau kiểm tra lại,
thực hiện M04 đến hết I04-12 không chờ duyệt cuối, gửi báo cáo. Namespace/PV
r2 đã verified ABSENT; không biến failed UI scenario thành PASS. Luna đang
sửa code/test; giữ state/handles để khép cleanup trước lượt mới.

I04-11 bản sửa `6bffefa`/merge `ea101e0`: UID DeleteOptions/Foreground,
exact renderer ownership và giữ owner qua removal retry; baseline namespace/
default SA bất biến và typed NotFound/historical-ID verification, inventory
phân lớp. Regression tests trong kubernetes/credential_target_test.go,
deployment/unreferenced_test.go/remove_delivered_test.go,
tfexec/namespace_internal_test.go, readiness/readiness_test.go và v2run/k8s_test.go.
Codex full1394Go/race/0FAIL/0SKIP,622frontend/fullchecks đạt; chưa thay thế
A01/E01/E02/A02/A03 live proof hoặc đóng IMP-021.

AWS pre-create permission gate:3136dcc/6d9fb2f share complete product actions
with runner/ECR; tests reject missing EKS access-entry/access-policy permission
and incomplete/duplicate/unexpected/truncated simulation. Full1395Go/race/
0SKIP/622FE đạt, read-only A02/A03 actual preflight đạt; chưa AWS live proof.
A01 a01-20261015-r1 FAIL trước provider create, CLEAN/CLOSED_CLEAN; socket
length A/B diagnosis và shortroot rerun ghi trong VERIFY-I04-11-20261014.

A01shortroot r2:42liveassertions/8marks đạt/cleanupCLEAN/28UIDs giữ/2PVABSENT,
nhưng SQLreopenharness sai application/phase =>overallFAIL. AWSgate vẫn đóng;
browserPASS vàCLOSED_CLEAN không được đồng nhất acceptancePASS. Luna đang
sửa scopedSQLrestart/overallcompletionproof vàA03explicitgp2 (defaultclass
không bật trênEKS1.35/pinnedCSI). IMP-021 chưa đóng; proofs trongbiênbảnI04-11.


**Bản sửa đã kiểm chứng15Oct:** Luna `34a9dcc` đã được Codex review/merge;
full1396Go/race/0fail/0skip với PostgreSQL/Terraform/Redis thật,632frontend/
38files, build/vet/type/lint/docs/PlantUML vàshell/node syntax đạt. Evidence
`/home/thanhnt1/.m4/checks-i0412-runtime-r3/`; sourcehash unchanged, PostgreSQL/
network/credentialfile riêng đãvắng. R2checks1395pass/1staleCLItestfail/0skip
được giữ; sửa đúngfixture explicitclosure, không giảmguard. SQLreopen nayđúng
scope/scenario, AWS gateyêu cầu overallrunnerreceipt, DirtyRunsyêu cầu đóng
marker tường minh; A03gp2 explicit, không đổi modulepins. A01fresh
`a01-20261015-r3` chuẩn bị ở `.m4/a01-20261015-r3/`,28retainedUIDskhớp,
namespace chưa có khipreflight; imageownership trong `.m4/prep-a01-r3/`.
Chưa ghi livePASS; đủA01/E01/E02 mới mởAWS.


**Catalogfix đãđạt:** Luna `d1f1da4` merge sau independent632frontend/38files,
type/lint/build/node/docs/PlantUML/diffPASS, sourcehashunchanged tại
`/home/thanhnt1/.m4/checks-reopen-catalog-r1/`. Backendbyteunchanged34a9dcc,
giữ bằngchứng1396Go/race/0skip đãchạy; khôngchạy lại Go cho thayđổi riêngJS.
Reopen exactcatalog naygồm builtin vàrender template thực tế của từngscenario.
Chuẩn bịA01 `a01-20261015-r4`, không dùngreceiptFAILr3 đểmởAWS.


**A01 đạt toàn lượt15Oct:** `a01-20261015-r4`, code d1f1da4, exit0,
runner-completion PASS/CLOSED_CLEAN,42liveassertions/8marks. UI khai báo/
Preview/deploy/3Ready/jobDONE, actualPGrow/Rediswrite-hit/TTLmiss/refill/reload,
UIxóa workload vàselectedRedis+PGđạt, uncheckednamespace giữREADY đếnsupervisor.
SQLreopen đạt5types/5definitions/3DESTROYEDresources/6deployments.
Video `/home/thanhnt1/.m4/a01-20261015-r4/A01-captioned.mp4`1339.07s,
decode/READY-CACHE-CLEANUPframes/phụđềreview đạt;24textfiles quétsecret đạt.
Namespace/haiPV vắng,28retainedUIDs khớpđộc lập; database
`i0410_83705f083962`/ba rolesvắng,haiVaultdừng/privatevắng;10imagerefs/3aliases
đãdọn,baseline/sourceimages nguyênvẹn. Proofs `operator-retained-and-absence.json`,
`operator-auxiliary-cleanup.json`,imagecleanup `.m4/prep-a01-r4/`.
E01 `e01-20261015-r1` đang chạy từ cùngcheckout; E02/AWS chưa chạy,
I04-11 chưa đủba scenario, IMP-021 vẫn mở.


**E01r1 vàbản sửa05bba6e:** E01browser12PASS/4marks, noDeploy/noApply/
noResources/supervisorCLEAN nhưngoverallFAIL vìverifier kỳvọngbuiltin mà
Previewkhôngpersist. RealAPIexactkeys4 ghi `operator-reopen-definition-keys.json`.
ReceiptFAIL/exit1/CLOSED_CLEAN; independent28retainedUIDs/namespaceabsence,
DB `i0410_d0f4045dd407`/3roles/privatevắng,Vaultdừng. Video
`.m4/e01-20261015-r1/E01-captioned.mp4`482.2sdecode/frame/phụđềreview đạt,
22textfiles secret scanđạt. Source trace `provisioning.Service.provisionNode`
chỉgọiEnsureBuiltinCluster choreserved internal-clustermatch; E01Preview và
AWS/EKS khôngpersistbuiltin. Luna05bba6e sửaexactscenarioexpectations/test;
full632frontend/type/lint/build/node/docsPASS tại `.m4/checks-reopen-catalog-r2/`,
sourcehashunchanged/backendbyteunchanged34a9dcc (1396Go/race/0skip đãđạt).
A01r4 vẫnđạt; freshE01r2 tiếp theo, E02/AWS chưa chạy.


**E01 đạttoànlượt15Oct:** `e01-20261015-r2`, code05bba6e, exit0 vàreceipt
PASS/CLOSED_CLEAN;12liveassertions/4marks. UIchặnnamespace missing vàannotations
sai kiểu, khônglưuDefinitionlỗi; Previewpure,0deployments/0resources/noapply.
SQLreopen đạt5types/4definitions/0resources/0deployments. Video
`/home/thanhnt1/.m4/e01-20261015-r2/E01-captioned.mp4`484.73sdecodeđạt,
phaseframes/phụđềđãkiểm tra;22textfiles quétsecretđạt. Independentcleanup
28retainedUIDskhớp/namespacevắng, DB `i0410_c39149ce7f17`/ba roles/privatevắng,
Vaultdừng, khôngcóimageimport. Proof `operator-independent-cleanup.json`.
E02 `e02-20261015-r1` bắtđầu từ05bba6e; A01r4/E01r2 đãđạt, gateAWS cònE02.


E02 lượt `e02-20261015-r1` không đạt: namespace được tạo nhưng Redis không
có Pod/PVC, nên không chứng minh được lỗi kéo ảnh. Bộ kiểm chứng chờ hết hạn;
SQL reopen vẫn đọc được 2 resource và lịch sử thất bại. Supervisor CLEAN,
marker CLOSED_CLEAN, namespace và database `i0410_160695d878fe`/ba role vắng,
28 UID giữ lại nguyên vẹn; Vault dừng và thư mục riêng đã xóa. Video
`/home/thanhnt1/.m4/e02-20261015-r1/E02-captioned.mp4` dài1044 giây, giải mã
đạt; ảnh lỗi và bằng chứng dọn độc lập được giữ. Không ghi E02 đạt.

Nguyên nhân fixture: ô chuỗi nhận nguyên văn chuỗi JSON đã có dấu ngoặc kép,
đưa dấu đó vào image reference và bị module từ chối trước Pod. Danh mục AWS
cũng dùng chuỗi JSON cho giá trị typed. Luna `bb38fc8` sửa đường nhập liệu:
giải mã catalog trước khi nhập, chuỗi nhập nguyên giá trị, kiểu khác dùng JSON;
ảnh E02 nhập không kèm dấu kép. Giữ nguyên trường map sai kiểu có chủ đích của
E01. Codex review và chạy độc lập634frontend/38files, type/lint/build/node/
docs/PlantUML đạt, mã băm không đổi; backend không đổi so1396Go/race/0skip.
Bằng chứng `.m4/checks-form-overrides-r1/`. Chuẩn bịE02r2; AWS vẫn bị chặn.


Bản sửa `cdfb027` đã review/merge/push `5cf6fd1`: định tuyến Service/Secret
Terraform bằng PreviewRunID trong manifest, đồng thời kiểm tag run-id ngoài,
managed-by và hình dạng module/instance chính xác; các ca giả mạo bị từ chối.
E02 dùng panel kết quả thật rồi mở deployment gần đây để kiểm Redis READINESS /
FAILED / READINESS_UNAVAILABLE qua lịch sử UI và dữ liệu đã lưu. Kiểm độc lập
1402Go/race/0fail/0skip/build/vet đạt; sau sửa riêng bài nhãn tiêu đề ghép,
634frontend/type/lint/build/node/docs đạt, backend không đổi. Kết quả lượt
kiểm tra đầu FAIL được giữ; tổng hợp bằng chứng thành phần tại
`.m4/checks-i0412-cleanup-r4/final-validation.json`.

Cleanup tiếp E02r2 dùng engine/handles/state/module/target gốc, chỉ thay CLI
xác minh đã sửa; giữ expires-at gốc và ngân sách dọn còn4909giây. Supervisor
độc lập PID4103109 báo CLEAN sau40giây, cả3lớp DONE/1attempt. Engine gốc xác
minh2instances DESTROYED/no remaining; namespace/PV vắng và28UID giữ lại khớp.
DB/3role đãxóa, Vault đúng PID3822964/địa chỉ đãxác minh rồi dừng, private
credential đãdọn. Runmarker CLOSED_CLEAN; receiptFAIL cũ không sửa. Video
E02r2 giải mã đạt; bằng chứng `operator-*` tại `.m4/e02-20261015-r2/`, spec/
status/log gốc lưu0700 ở `.m4/e02-20261015-r2-recovery-private/`.
E02fresh `e02-20261015-r3` bắt đầu từcdfb027; chưaAWS.


I04-11 đã đạt15Oct: A01r4 (42assertions), E01r2 (12), E02r3 (15),
receiptPASS/exit0/CLOSED_CLEAN, SQLrestart/video/cleanup độc lập và28retainedUIDs
đạt. E02 chứng minh READINESS/FAILED thật với lịch sử/module provenance giữ;
fixcdfb027 đã kiểm1402Go/race/0skip/634frontend/fullchecks. Evidence vàcác lượt
lỗi giữ tại VERIFY-I04-11-20261014. I04-12 current sauAWSgate/preflightA02
read-only đạt, chưa AWS provision; IMP-021 còn mở/D12–D14 không đổi.


AWS runner startup regression010b1af: pinned kubectl dùng được trên AWS không
cóKube config, kubeconfig/context/namespace/sentinel vẫn không khả dụng.
1403Go/race/0skip/634frontend/fullchecks đạt; lần A02 trước không có effect,
scenario AWS thật vẫn còn. Evidence VERIFY-I04-12-20261015.


AWS startup/cleanup Vault delivery1d35a00: cùngmode theoadapter, VSO giữchoK8s,
agent dùngchoAWS. ActualcleanupOptions hồi quy vàCLIgetter coverage đạt;
1407Go/race/0skip/634frontend/fullchecks độc lập. No-effectA02r1 đãkhép,
freshA02r2 tiếp theo; liveAWS vẫn còn. VERIFY-I04-12-20261015.


I04-12 đang chạy A02r5 từ `5074bb2`. Getter ảnh probe, nhập boolean qua native
select, chẩn đoán từng điều kiện Preview và đọc đầu vào sau cổng khởi chạy đã
được review/merge. Bản sửa cuối có hồi quy file thật thay ECR/digest trong lúc
chờ cổng; 642 frontend và các checks tương ứng đạt, backend không đổi so R7
1.407 Go/race/0 skip. A02r1–r4 giữ kết quả FAIL và cleanup đã được xác minh.
Đây chưa phải bằng chứng A02/A03 đạt: IMP-021 còn mở, D12–D14 vẫn hoãn.
Bằng chứng: VERIFY-I04-12-20261015.


2026-10-16: `bcabb40` sửa VPC output tuple→list bằng release v4 bất biến và
ghi đủ bảy pin module trước create. R9 độc lập1408Go/race/0skip,642frontend/
fullchecks/sourcehash/fixturecleanup đạt; hồi quy Terraform CLI thật. A02r5
đã khép sau verifier gốc và đối chiếu EC2/SLR/SQL/Vault; chuẩn bị A02r6.
IMP-021 vẫn mở vì A02/A03 job/cache live chưa đạt; D12–D14 không đổi.
Bằng chứng VERIFY-I04-12-20261015.


A02 `a02-20261016-r2` đã đạt35assertion/8mốc, ba deploymentSUCCEEDED,
job/cache/Aurora/ElastiCache/TTL/refill/reload thật, SQLreopen, video,
secret scan, runnerPASS/exit0/CLOSED_CLEAN và supervisorCLEAN. DB/roles/
Vault/private vắng,28UID/11SLR giữ. Evidence trong
[biên bản I04-12](../verification/2026-10-15-i04-12-aws.md).
IMP-021 vẫn mở vì A03 chưa chạy; D12–D14 không đổi.


A03r1 FAIL do heartbeat cancellation đã CLOSED_CLEAN operator với audit,
original verdict giữ. Diagnostic heartbeat `1f3d3cb` đã review/merge, R15 đạt
1482Go/race0skip/649frontend/fullchecks/hash/fixture. Không đổi quyền/hold/
fencing; A03r2 chờ chạy thật, IMP-021 vẫn mở đến A03 đạt và cleanup sạch.


## M04 — closure ngày2026-10-16

IMP-021 đã đóng trong phạm vi Terraformv2. Năm biên nhận A01r4/E01r2/E02r3/
A02r2/A03r2 đềuPASS/exit0/CLOSED_CLEAN, có video/SQL/native cleanup/retained
UID/SLR và fixture absence. R15 đạt1482Go/race/0skip,649frontend/fullchecks.
[Biên bản bàn giao](../verification/2026-10-16-m04-completion.md) ánh xạ từng
task đến execution record/video. Các ghi chú còn mở phía trên là checkpoint
lịch sử trước closure; D12–D14/M02/TLSissue#2 vẫn hoãn, không bị đóng theo.


## V01 account catalog — design checkpoint

| Requirement | Canonical design | Implementation / evidence |
|---|---|---|
| UC-04 AC-01..05 | ADR-015; account API; schema/ERD; auth/credential boundaries | Pending writer; no V01 runtime claim |
| UC-04 AC-06..09 | Versioned rename/reverify/same-target replacement; OC-05a; UC-04 sequence/VOPC/state machine | Pending CAS/race/cleanup/redaction checks |
| UC-04 AC-10..12 | Account UI states; staged consumer boundary; no V02–V03 old-data migration | [V01 handoff](../verification/2026-10-17-v01-handoff.md); browser/restart pending |


## V01 infrastructure account catalog — ADR-015

| Requirement | Implementation | Verification |
|---|---|---|
| UC-04 AC-01..05: Organization catalog, type/auth distinction, safe descriptor and permissions | `connection/accounts.go`, `http/driver_accounts.go`, shared Connection version and scoped credentials | `driver_accounts_http_test.go`: org/PE/Admin/Developer gates, strict payloads and public DTO; existing resolver scope guards |
| UC-04 AC-06..09: CAS, read-only checks, same-target replacement, retained prior objects and conservative commit recovery | `accounts.go`, PostgreSQL `registration.go`/`store.go`, memory/JSON CAS | `accounts_test.go`: identity/trust mismatch, CAS loss, legacy-region denial, committed-but-unacknowledged create/replace, known rollback cleanup; `commit_error_test.go`; shared persistence contract |
| UC-04 AC-10: create/detail/edit/verify UI, stale and unknown outcome recovery | `features/platform/accounts/` | `DriverAccountsPage.test.tsx`; `v01-accounts-local.mjs` human-paced headed flow, Vietnamese captions, hard secret-reply assertions |
| UC-04 AC-11..12: SQL/Vault persistence and restart; preserve legacy consumers through V01 | PostgreSQL baseline/version compatibility, scoped Vault, legacy Connection resolver | shared SQL/JSON reopen tests, HTTP legacy-preservation tests, `v01-accounts-local.sh`; [dated checks and evidence](../verification/2026-10-17-v01-handoff.md) distinguish real SQL/Vault from simulated external providers |

## V02 Definition authoring and matching — delivered 2026-10-17

[UC-03 V02-01..09](../usecase/UC-03/specification.md) and ADR-015 V02 supersede
historical profile/Environment AWS binding/insert-only statements in prior rows.

| Requirement | Design | Validation required before completion |
|---|---|---|
| V02-01/06/07 | OC-04 V02, schema Name/version/Driver Account, sequence/VOPC CAS | Create/edit/reload/restart; immutable ID/type; PostgreSQL race/rollback; snapshots unchanged; stale preview |
| V02-02/05 | Shared matcher, OC-04c snapshot without profile filter | Five weights/AND/OR/wildcard/empty/ties/no-match; unconfigured env; winner conflict no fallback; stale/late replies |
| V02-03/04/09 | Capability/module contract; scoped selected account + region per node | Role/org/kind/READY; schema/types/mappings/references/provision; no secret/Environment fallback; pre-provision pins |
| V02-08 | Fresh baseline/JSON/seed, schema/ERD | No migration/reset on startup; incompatible old state rejected; fixtures deterministic |
| V02.09–10 | UC-03 UI artifacts, headed recording runbook | Independent full checks zero-skip; Vietnamese captions/cursor/input/reviewed native issue attachment; owned cleanup |

Independent checks: Go1528tests, race530tests/8packages plus closure99tests,
frontend687tests, build/vet/docs/PlantUML PASS; zero individual skips.
Catalog/domain/planning/tfexec/resource-cleanup and HTTP/SQL tests cover the
obligations above; `v02-definitions-local.sh`/`v02-definitions-local.mjs` cover
21 UI phases including edit/reload/restart and inherited cluster/current renderer.
T16 also passed independently. [Evidence and limits](../verification/2026-10-17-v02-handoff.md).

### V02 provision evidence follow-up — verified 2026-10-18

| Requirement | Added coverage | Evidence status |
|---|---|---|
| UC-03 BR-26/V02-03/07 | ResourceDefinitionsPage edit regression: nested params, flags, form↔JSON, unchanged account/inputs/criteria, local duplicate/invalid params rejection | Independent frontend690tests/full gates PASS |
| UC-06 BR-08 | CoProvisionFlagMatrix; stored HTTP registration → real Score Preview; flag toggle/CAS, nested params, cycle rejection | Full Go1536tests and race190tests/2packages PASS, zero skips |
| V02.09–10 | `v02-provision-local.sh`/`v02-provision-scenario.mjs`: UI author/save/reload/edit/restart and real Preview graph/batches; all parent consumers and provider-first edges | Headed SQL/Vault UI assertions PASS; video waived by user; no deployment claim |

The original V02 video only covered resource_ref inheritance; it did not author
provision rules. This follow-up keeps contract unchanged. Default false flags
and empty params normalize on the server; editor draft round-trip is lossless.

Follow-up original V02 rerun canceled at user request (exit143), after V02P
scenario/wrapper finished0. No new video upload required; existing V02 video is
not reclassified as companion-authoring evidence. Cleanup independently verified;
[handoff](../verification/2026-10-17-v02-handoff.md) records scope and waiver.


## V03 Definition-selected Internal workload placement

| Requirement | Design | Implementation / verification |
|---|---|---|
| UC-03 V03-01/02, UC-06 V03-01 | ADR-016; workload-rendering contract; UC-03/06 realization | `catalog/v03_registration_test.go`, `planning/placement_test.go`, `score/class_test.go`; live UI Type/Definition/Score/graph assertions |
| UC-05 BR-12, UC-06 BR-21/V03-02, UC-08 BR-08 | ADR-016; internal sequence; per-resource schema pins | `planning/placement_test.go`, `deployment/placement_test.go`, `placement_post_test.go`, `placement_retry_test.go`; strict inputs, full DAG cycles, flags, target agreement, unsupported multi-cluster, PostgreSQL reopen and pre/post workload order |
| UC-01 V03 Settings, UC-06 V03-04 | ADR-016 removal gate; UI/API contracts | R6 real replacement PASS before removal; `application/service_test.go` rejects Internal writes without mutation; `configuration/SettingsPage.test.tsx`, target/transition/seed guards and fail-fast historical runners |
| UC-06 V03-03; UC-07 V03 restoration | V03 coverage handoff; lifecycle/ownership contracts | `v03-internal-frontend.sh`, headed scenario and `v03run/` ownership tests; R6 replacement and final r7 update/restart/remove/cleanup PASS35/35; V04 business coverage explicitly deferred |

Independent final Go1580/race526/frontend700tests and build/vet/type/lint pass,
0individual fail/skip. Live coverage, video and cleanup evidence are in the
[V03 handoff](../verification/2026-10-18-v03-handoff.md); frontend-only proof
does not establish backend/jobs/cache behavior.


## V04–V06 — design, implementation and gate evidence

| Requirement | Canonical design | Verification mapping |
|---|---|---|
| UC-03/05/06/08 V04-01 | ADR-017 graph/reference/provision semantics; module/state contracts | V04 R18 complete/pushed5413fa1:54/54main assertions, full shared PG/Redis/jobs/cache/persistence/reverse cleanup; full/focused/race source-bound checks in handoff. Intentional live negatives removed19-10, existing tests retained |
| UC-03/05/06/08 V05-01 | ADR-017 typed EKS identity; UC-06 cloud sequence; per-resource schema pins | V05 R14 source39 replacementPASS46/46: unknown→resolved EKS/auth/API/network/node readiness, full app/update/restart/reuse/native absence;5SQLDESTROYED/4APIrows/7deployments reopen. S3 retirement integrated afterPASS; finalGo1715/race218/build/vet and focusedhelpersPASS; no intentional live negatives19-10 |
| UC-03/05/06/08 V06-01 | ADR-017 actual managed network dependencies/account isolation | Account/region and managed-only graph contract tests PASS. A02R3 and A03R1 each47/47/nativeSQLprocessprivateclean, sequenced after cleanup. S5 integrated after both gates; final source51 A03 a03f-1020041903-32d7 has47/47/exit0/reopen/secret/nativeSQLprocessprivateclean PASS |
| UC-06 V06-02 | ADR-017 replacement/removal gates | S3 applied only after complete R14replacement and independent cleanupPASS; final impacted checks before V05commit. S5 applied after A02R3/A03R1 full/native-clean; dead transition adapter/ports removed with focused164/race76 and preserved current UID logic. Final schema/DTO/UI A03 regression47/47/exit0/native-clean PASS; current UID creation/deletion also proved in same run |

Exact app-function→gate→UI/technical assertion→evidence→cleanup mapping is in
[pre-code coverage map](../verification/2026-10-18-v040506-handoff.md).
Gate outcomes and reuse limits follow the dated handoff. V06 replacement and
actual final post-removal A03 are complete; historical prepared evidence is not
substituted for those live runs.

## Issue #6 — EC2 IAM role authentication

| Requirement | Design | Implementation / validation |
|---|---|---|
| UC-04 EC2-01..03 | ADR-018; UC-04 realization/sequence/VOPC; schema | connection EC2 registration/verify/resolver and awsaccess IMDS/STS tests |
| UC-04 EC2-04 | Terraform module contract; credential design | provider binder refresh/readiness/EKS checks |
| UC-04 EC2-05 | UC-04 UI screens/states/API mapping | DriverAccountsPage tests; frontend checks |


## UC-03 direct Git module authoring — issue #7

| Requirements | Implementation | Focused verification |
|---|---|---|
| GIT-01/02/05/06; OC-04e | `registry.InspectGit`, `GitCLI.Resolve`, catalog inspection, POST terraform-modules/inspect | `registry/git_inspect_test.go`, `delivery/http/terraform_modules_test.go`: immutable pins, TLS Git, transitive modules, restart/cache, role/scope/no Definition mutation |
| GIT-03/04/05 | `GitModuleSource`, `ResourceDefinitionsPage`, capability projection | `GitModuleSource.test.tsx`, affected Definition/metadata tests; `gitmodulebrowser` + `git-module-local.mjs` real HTTPS Git create/edit/reload main flow |

## Resource Definition v3 — issue #8

[UC-03 UI3-01..04](../usecase/UC-03/specification.md) uses v3 layout only;
backend operation contracts, transactions and sequence stay unchanged.

| Requirement | Implementation | Focused validation |
|---|---|---|
| UI3-01 tabs/draft/CAS | ResourceDefinitionsPage mounted tabpanels, shared save/reload pipeline | DefinitionV3.test.tsx; ResourceDefinitionsPage.test.tsx CAS/unknown-outcome; real HTTP browser scenario |
| UI3-02 advanced/mappings/provider/provision/review | Existing MappingEditor/ProvisionEditor/typed converters; disclosure and full draft review | TypedDefinitionForm.test.tsx, moduleMapping.test.ts, typedConversion.test.ts, DefinitionV3.test.tsx; browser unequal Type/module names, flags/params round-trip |
| UI3-03 rule criteria/wildcard/empty/incomplete | CriteriaRules and draftToCriteria; parent-owned selected fields | ResourceDefinitionsPage.test.tsx rule flows; DefinitionV3.test.tsx five-field copy and wildcard; existing CriteriaEditor.test.tsx for renderer projection |
| UI3-04 copy/matching/help/hidden invalidation | CriteriaEditor native disclosures; mounted MatchingPreviewPanel signature/session/late reply guards | DefinitionV3.test.tsx hidden in-flight/session invalidation; MatchingPreviewPanel.test.tsx context/catalog metadata; RenderingTemplatesPage.test.tsx shared consumers; real matching/copy browser |
| Existing GIT-01..06 | GitModuleSource and source/pin selection retained | GitModuleSource.test.tsx, real HTTPS Git inspect/create/reload browser |

### V3 visual correction

UI3-01/02 layout: ResourceDefinitionsPage catalog/detail views, numbered cards,
actual metadata overview and fixed action bar; MappingEditor aligned rows and
optional provenance/help. DefinitionV3 tests preserve invalid JSON through
catalog navigation; focused App T04 tests retain prefill/no-write behavior.
Real HTTP browser assertions cover catalog hidden in detail, overview to the
right, fixed bar geometry, resume draft and CAS409 after navigation, plus mobile.
[Visual correction evidence](../verification/2026-10-21-resource-definition-v3-visual.md)
qualifies the earlier functional-only acceptance.

Issue #8 visual follow-up: UI3-02 maps to the single atomic advanced toolbar in
ResourceDefinitionsPage and disabled metadata previews in TypedVariablesEditor.
DefinitionV3/TypedDefinitionForm and stored-provision regression assert failed
conversion preserves both JSON drafts, unknown fields and omission semantics.
UI3-04 case summaries retain the legacy renderer projection; CriteriaEditor and
RenderingTemplates tests cover the shared consumers. The same visual evidence
records comparison with the explicit user-supplied live v3 URL.
