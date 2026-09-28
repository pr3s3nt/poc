---
id: TRACEABILITY-MATRIX
artifact: traceability-matrix
status: current
last_reviewed: 2026-09-23
---

# UC-00..UC-09 Traceability Matrix, with UC-12/UC-16 design traces

Mỗi main-flow step được ánh xạ tới operation, PlantUML sequence, class/method, persistence/state và planned test. `OC-nn` tham chiếu `architecture/contracts/operation-contracts.md`.

## UC-00

| Steps | Operation / sequence | Class methods | Persistence/state | Contract / tests |
|---|---|---|---|---|
| MS-01, MS-02 | `SignIn`; `UC-00/sequence.puml` | `AuthenticationService.SignIn`, `UserAccountRepository.FindActiveByUsername`, `PasswordHasher.Verify` | read active `user_accounts`; no session on failure | OC-00; invalid-password/disabled-account tests |
| MS-03 | same | identity-context builder | User ID, Organization ID, role | OC-00; organization/role context test |
| MS-04 | same | random-token generator, `SessionRepository.Save` | token-hash-only `sessions` record | OC-00; no-raw-token persistence test |
| MS-05 | authentication middleware | session lookup and context injection | authenticated request context | OC-00; protected-route and sign-out tests |

## UC-01

| Steps | Operation / sequence | Class methods | Persistence/state | Contract / tests |
|---|---|---|---|---|
| MS-01, MS-02 | `CreateApplication`; `UC-01/sequence.puml` | `ApplicationService.CreateApplication`, subdomain validator | `applications`; unique Name/Subdomain | OC-01; invalid/duplicate subdomain tests |
| MS-03, MS-04 | same | default-target resolver, `ConnectionRepository.FindReady`, `Application.Create` | Application system ID, profile/connection binding; AWS `PENDING`, internal `READY` | OC-01; target-resolution tests |
| MS-05, MS-06 | same | `Environment.Create`, `DeploymentSet.Empty`, `NamespaceIdentity.ForEnvironment` | exactly `staging` and `production`; `environments`, `deployment_sets` | OC-01; atomic default-environment test |
| MS-07, MS-08 | same | endpoint derivation, `ApplicationRepository.Save` | derived desired endpoints; atomic Application/Environment/current-set write | OC-01; endpoint derivation and no-infrastructure test |

## UC-02

| Steps | Operation / sequence | Class methods | Persistence/state | Contract / tests |
|---|---|---|---|---|
| MS-01, MS-02 | `RegisterResourceType`; `UC-02/sequence.puml` | `ResourceTypeService.RegisterResourceType`, `SchemaValidator.ValidateResourceTypeSchemas` | none before validation | OC-03; valid/invalid schema tests |
| MS-03, MS-04 | same | `ResourceTypeRepository.Exists/Save` | `resource_types`; unique `(org,key)` | OC-03; duplicate/repository tests |
| MS-05, MS-06 | same | return `ResourceType` / repository read contract | persisted schemas | OC-03; planner catalog integration test |

## UC-03

| Steps | Operation / sequence | Class methods | Persistence/state | Contract / tests |
|---|---|---|---|---|
| MS-01, MS-02, MS-03; BR-07 | `RegisterResourceDefinition`; `UC-03/sequence.puml` | `ResourceDefinitionService.RegisterResourceDefinition`, `DefinitionValidator.ValidateStructureAndCriteria` | none before validation | OC-04; Terraform/Kubernetes definition tests; missing/empty criteria rejected before persistence |
| MS-04, MS-05 | same | type/connection repository reads; `ValidateReferencesAndRules` | `resource_types`, `connections` | OC-04; invalid reference test |
| MS-06, MS-07 | same | `ResourceDefinitionRepository.Exists`, `DriverContractInspector.Inspect` | source fingerprint candidate | OC-04; output mismatch test |
| MS-08, MS-09; BR-07 | same | `ResourceDefinitionRepository.Save/FindCandidates` | `resource_definitions`, `matching_criteria` | OC-04; matching query integration test; explicit `{}` wildcard test; conformance adapter skips external missing/empty criteria (`TestReadDefinitionsCriteriaShapes`, `TestProductPlannerRejectsCriteriaLessDefinition`) |
| BR-01, BR-03 | [ADR-001](../architecture/decisions/ADR-001-profile-resource-scopes.md) | Optional Definition profile guard before unchanged five-field scoring | `TestPlan_NewApplicationMatchesPostgresByExecutionProfile`, invalid-profile validation |

## UC-04

| Steps | Operation / sequence | Class methods | Persistence/state | Contract / tests |
|---|---|---|---|---|
| MS-01, MS-02 | connection registration; `UC-04/sequence.puml` | Controller dispatches `RegisterAWSDriverAccount` or `RegisterKubernetesCluster` | transient credential only | OC-05; profile-specific tests |
| MS-03 | same | `AWSConnectionVerifier.Verify` / `KubernetesConnectionVerifier.Verify` | Connection `VERIFYING` | OC-05; RBAC/identity tests |
| MS-04 | same | `SecretStore.Put` | external secret + opaque ref | OC-05; no credential persistence test |
| MS-05, MS-06 | same | `Connection.MarkReady`, `ConnectionRepository.Save` | `connections.status=READY` | OC-05; repository test |

## UC-05

| Steps | Operation / sequence | Class methods | Persistence/state | Contract / tests |
|---|---|---|---|---|
| MS-01 | `PreviewDeployment`; `UC-05/sequence.puml` | `PlanningSnapshotRepository.Load` | read current set/version | OC-06; no-mutation test |
| MS-02, MS-03; BR-07 | `PlanningService.Plan` | `ScoreConverter.ConvertAndValidate`, `WorkloadSpecValidator.ValidateContainerResources` | typed fragment with optional CPU/memory requests/limits | OC-07; Score fixture + container-resource contract tests (`TestParse_ContainerResources*`, `TestFragment_ContainerResourcesStayPerContainer`, `TestPlan_PreservesContainerResourceRequirements`, `TestPlan_ContainerResourcesAddNoGraphNodes`, `TestPlan_ContainerResourceChangeIsModuleRelativePatch`) |
| MS-04 | same | `BeforeStateValidator.Validate`, `DeltaBuilder.BuildHumanitecDelta` | transient Humanitec-shaped Delta/Candidate | OC-07; shape, relative-patch, array-diff and invariant tests (`internal/planning/delta_test.go`, `internal/planning/jsonpatch`, conformance `assertDelta`) |
| MS-05 | same | `ImplicitResourceEnricher.Enrich`, `ResourceGraphBuilder.BuildAndExpand`, `DefinitionMatcher.MatchAll` | in-memory graph/matches | OC-07; AWS/internal graph tests |
| MS-06, MS-07 | same | `DriverContractInspector.InspectContracts`, `ActiveResourceClassifier.Classify`, `BatchScheduler.Schedule` | in-memory plan | OC-07; contract/topology fixtures |
| MS-08 | `PreviewDeployment` | return `DeploymentPreview` | no write/state transition | OC-06; adapter-not-called test |

## UC-06

| Steps | Operation / sequence | Class methods | Persistence/state | Contract / tests |
|---|---|---|---|---|
| MS-01 | `DeployWorkload`; shared/cloud/internal sequences | `DeploymentService.DeployWorkload`, `EnvironmentRepository.LoadPlanningSnapshot`, `DeploymentRepository.Create` | `deployments=PLANNING`, base set/version | OC-08; both-profile tests |
| MS-02, MS-03 | `PlanningService.Plan` | Score/workload validator/before/`BuildHumanitecDelta` | `deployment_delta_snapshots`, Candidate `deployment_sets` | OC-07/08; Delta shape/invariant and resource-preservation tests; Snapshot persistence/association (`TestDeployWorkload_PersistsOneDeltaSnapshotPerDeployment`, `TestDeploymentReferencesExactlyOneDeltaSnapshot`) |
| MS-04, MS-05, MS-06; BR-12 | same | profile load, `ResourceDescriptorParser.ParseDescriptorText`, `ImplicitResourceEnricher`, graph builder | canonical descriptors + graph JSON snapshot | OC-07; scoped-token and implicit-resource tests |
| MS-07, MS-08 | same | matcher, inspector, classifier, scheduler | `deployment_plans`; `PROVISIONING` | OC-07/08; match/contract/DAG tests |
| MS-09 | `ResourceProvisioningService.Provision` | UC-08 methods | `active_resources`, `deployment_resources` | OC-10; resource integration tests |
| MS-10 | deploy | `OutputBindingResolver.ResolveWorkloadBindings` | resolved values stay in execution context; safe snapshot only | OC-08; output propagation test |
| MS-11; BR-11 | deploy | `WorkloadRenderer.Render`, `WorkloadDeployer.Apply/WaitReady` | declared requests/limits in manifests; missing request = same-field limit, else default; `workload_instances=APPLYING/READY` | OC-08; renderer resource mapping + kind/fake adapter tests (`TestRender_*ContainerResources*`, `TestRender_LimitsOnlyBelowDefaultUseLimitAsRequest`, `TestRender_DoesNotMutateModuleResources`, `TestDeployWorkload_RendersDeclaredContainerResources`, kind `TestKindInternalVerification` live resources) |
| VAR-03; BR-13 | Fleet GitRepo delivery | `gitops.Deployer.Apply/WaitReady`, Fleet `GitRepo` | Git commit of non-secret manifests, namespace-local Harbor pull Secret, exact workload revision Ready | ADR-007; `gitops/deployer_test.go`, `fleet-gitrepo-kind-verify.sh` and 2026-09-28 kind evidence |
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
| MS-07 | remove/update | `ActiveResourceRepository.MarkUnreferenced` | `active_resources=UNREFERENCED`; no destroy | OC-09; no-destroy test |
| MS-08 | final commit | current pointer + `MarkSucceeded` | atomic set/deployment state | OC-09; transaction test |

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
| MS-01, MS-02 | `GetDeployment`; `UC-09/sequence.puml` | `DeploymentQueryService.GetDeployment`, repo reads | Deployment/plan/set snapshot | OC-11; scoped query test |
| MS-03 | same | resource/workload repository reads | deployment-resource/instance status | OC-11; status assembly test |
| MS-04, MS-05 | same | `DeploymentViewAssembler.Assemble` | persisted graph/matches/batches | OC-11; view test |
| MS-06 | same | `OutputRedactor.RedactSecretOutputs` | no mutation | OC-11; redaction test |
| MS-07 | same | return `DeploymentView` | read-only | OC-11; no runtime call test |

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
| MS-03, BR-11 | [Specification](../usecase/UC-16/specification.md), [screens](../usecase/UC-16/ui/screens.md) | Resource Type inputs shown on form; required params validated and written into Score | `WorkloadEditorPage.test.tsx` PostgreSQL params test |
| MS-04–MS-05 | [Specification](../usecase/UC-16/specification.md) BR-02–BR-05 | UC-12, resource-output and same-Environment Service references; no literal binding or secret disclosure | Source eligibility, missing key/output/port, cross-Environment rejection and secret redaction |
| MS-06–MS-07 | [Realization](../usecase/UC-16/realization.md) | Validate then save pending desired change; current Deployment Set/runtime unchanged | Field errors, pending-save and no-runtime-mutation tests |
| VAR-01–VAR-02 | [States](../usecase/UC-16/ui/states.md) | Score import uses same rules; deletion requires confirmation and supports Undo | Literal-import rejection, import parity and pending-delete/undo tests |
