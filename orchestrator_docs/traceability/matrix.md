---
id: TRACEABILITY-MATRIX
artifact: traceability-matrix
status: current
last_reviewed: 2026-10-13
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

## Definition-selected workload rendering (ADR-010)

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
| MS-06, MS-07 | same | `ListResourceDefinitions`, embedded `terraform.Inspector.Inspect` or static executor output contract | source fingerprint candidate | OC-04; duplicate/remote-source/output validation tests |
| MS-08, MS-09; BR-07 | same | `CreateResourceDefinition/ListResourceDefinitions` | insert-only org-scoped catalog; PostgreSQL Definition + criteria atomic transaction | OC-04; registration contract/race/criterion-failure rollback; no-restart HTTP Preview; explicit `{}` wildcard and criteria-shape conformance |
| BR-01, BR-03 | [ADR-001](../architecture/decisions/ADR-001-profile-resource-scopes.md) | Optional Definition profile guard before unchanged five-field scoring | `TestPlan_NewApplicationMatchesPostgresByExecutionProfile`, invalid-profile validation |

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
are recorded separately. AWS remains future scope.

| Steps | Operation / sequence | Class methods | Persistence/state | Contract / tests |
|---|---|---|---|---|
| MS-01..03; VAR-01 | inspect; `UC-04/sequence.puml` | HTTP Controller, InspectKubeconfig, parser | transient safe context metadata | OC-05; `TestInspectKubeconfig_ListsSafeMetadataOfValidContexts`, `TestKubeconfig_RequiresAPIVersionAndKind`, `TestUC04UploadHTTPContract`, `ConnectionsPage.test.tsx` |
| MS-04..05; BR-06..08 | upload register, verify | Service.RegisterKubeconfig, verifier.VerifyKubeconfig | read-only API/RBAC | OC-05; `TestNormalizeKubeconfig_RejectionsAreSafeAndCategorized`, `TestVerifyKubeconfig_ReadOnlyChecksWithSelectedContextOnly` |
| MS-06; BR-02,09,10 | credential Put | scoped CredentialStore | external immutable secret, no database bytes | OC-05; `TestConnectionCredentials_ScopedCreateOnlyLifecycle`, `TestConnectionCredentials_RejectsOutOfScopeReferencesWithoutRequests`, `TestCredentialResolver_FailsClosed`, `TestCredentialTarget_KubectlFailuresNeverEchoOutput` |
| MS-07; BR-01,15,16 | CreateConnection | repository + UnitOfWork | name/auth type, insert-only READY, default unchanged | OC-05; `TestRegisterKubeconfig_ConcurrentSameNameGetsDistinctKeys`, `TestFreshSchemaSeedsVerifyingKubeconfigAndRejectsHostContext`, `TestPostgresKubeconfigRegistrationRaceAndReopen`, current JSON Connection metadata/reopen checks (`adapters/store/load_test.go`) |
| MS-08..09 | public response/reload | DTO, ConnectionsPage | no response secret; catalog consumption | OC-05; `TestUC04UploadHTTPContract`, frontend registration/state tests, dedicated Playwright runner |
| ERR-07..08 | write/cleanup failures | CredentialStore.Delete | only own attempt cleaned with bounded context | OC-05; `TestRegisterKubeconfig_FailuresSaveNothingReady`, `TestUC04UploadWithoutCredentialStoreFailsClosed` |
| BR-10; POST-03 | execution credential resolution | resolver, Kubernetes adapters | opaque target identity only | `TestCredentialTarget_DeployerUsesScopedPrivateKubeconfig`, `TestCredentialTarget_RoutesVSOAndExecutorUseScopedCredential`, `TestExistingCluster_CredentialBackedConnectionIsAuthoritative`, `TestCredentialBackedTarget_RemovalAfterRestart`, `TestFleetRejectsCredentialBackedTargets` |
| VAR-02; BR-11..12,14 | future AWS register | AWS verifier/resolver, Terraform | ADR-009 credential namespace | designed; not implemented by upload slice |

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

## Terraform v2 — design-to-test obligations (implementation pending)

All V2 tests/assertions below are planned obligations, not already executed.
Stable V2 IDs are scoped to their owning specification. Actual code/test/evidence
links must replace planned labels at implementation handoff; old tests/videos
remain baseline evidence only. Shared contracts:
[module](../architecture/contracts/terraform-modules.md),
[state](../architecture/contracts/terraform-state.md),
[acceptance](../operations/terraform-v2-acceptance.md).

| Requirements | Operation / realization / sequence | Data/state | Required tests / acceptance |
|---|---|---|---|
| UC-02 V2-01/02; UC-03 V2-01/02/06 | OC-03/04/04d; UC-03 realization/sequence/VOPC; ModuleSourcePolicy/BundleResolver | immutable verified bundles + insert-only Definitions/criteria | source allowlist, commit/digest drift, traversal/command/credential rejection, output mismatch, typed schema/defaults, no registry fetch in pure query |
| UC-03 V2-03/04; UC-04 V2-01..04; UC-08 V2-02 | OC-05/07/10; UC-04 and UC-08 sequences | scoped Vault credential refs, same Environment account/region, opaque EKS target | AWS onboarding safe failure/attempt cleanup; cross-account/region/state rejection, host credential isolation, target reopen; all three live targets |
| UC-03 V2-05; UC-05 V2-01..04; UC-06 V2-03/04 | OC-06/07/08/17; UC-05/06 diagrams | pinned module/provider/binary/renderer digests + token/versions | missing/tied Definition, no native/class fallback, unavailable bundle, stale module pin, pure Preview, protected manifests/secret refs; V2-A01..03 |
| UC-06 V2-02/05; UC-08 V2-01/03..07 | OC-10/18; UC-08 sequence/VOPC; shared classes/ERD | encrypted states, instance identity, locks, attempts, Environment owner/fence | real disposable PostgreSQL ciphertext/reopen/rollback, two-owner race, stale fence write/unlock, serial/lineage rejection, no expiry unlock; stable name/password/state across retry |
| UC-07 V2-01..04; UC-08 V2-05/06 | OC-09/19; UC-08 cleanup sequence; Terraform instance state machine | ownership class, original module/target/state, cleanup progress/tombstones | create/update/destroy contract, destructive UPDATE rejection, last/shared consumer retention, preexisting/updated preservation, reverse graph, provider absence and interrupted cleanup; V2-E02 |
| UC-16 V2-01..04; UC-05 V2-01/02 | OC-16/17; UC-16 sequence and UC-03/05/16 UI artifacts | draft params/class/shared IDs; safe module/target metadata | typed params/class selection/advanced roundtrip, metadata error/stale guard, no Workload module selector; real headed UI Save/Preview/Deploy; V2-E01 no infrastructure |
| UC-06 V2-06; UC-09 V2-01..03 | OC-08/11; UC-09 sequence/screens | persisted safe stage/attempt/cleanup result + app job/cache evidence | no raw CLI/state/secret projection; all three headed Console -> app job completion/reload; actual PostgreSQL result + Redis write/hit/miss/refill; failed evidence retained |
| ADR-014 deferred boundary | D12/D13/D14; state contract minimal hold/manual verification | interrupted operation/needs-attention, private handles | no auto-resume/import/backup claim; independent supervisor cleanup after browser/runner failure; absence proof mandatory |

Acceptance IDs V2-A01/A02/A03/E01/E02 are owned by the runbook. Acceptance
update/delete matrix and disaster recovery remain outside live scope, while
executor/persistence unit/integration contract tests are required.
