# UC-01..UC-09 Traceability Matrix

Mỗi main-flow step được ánh xạ tới operation, PlantUML sequence, class/method, persistence/state và planned test. `OC-nn` tham chiếu `architecture/contracts/operation-contracts.md`.

## UC-01

| Steps | Operation / sequence | Class methods | Persistence/state | Contract / tests |
|---|---|---|---|---|
| MS-01, MS-02, MS-03 | `CreateApplication`; `UC-01/sequence.puml` | `ApplicationService.CreateApplication`, `ConnectionRepository.FindReady` | `applications`, `connections`; AWS `PENDING`, internal `READY` | OC-01; `TestCreateApplication_AWSEKS`, `...InternalK8s` |
| MS-04 | same | `Application.Create`, `ApplicationRepository.Save` | insert Application/version | OC-01; repository unique test |
| MS-05, MS-06, MS-07 | `CreateEnvironment`; sequence | `ApplicationService.CreateEnvironment`, `Environment.Create`, `DeploymentSet.Empty` | `environments`, `deployment_sets` | OC-02; `TestCreateEnvironment_Initializes...` |
| MS-08 | same | `DeploymentSetRepository.SaveAndSetCurrent` | atomic current-set pointer | OC-02; transaction integration test |

## UC-02

| Steps | Operation / sequence | Class methods | Persistence/state | Contract / tests |
|---|---|---|---|---|
| MS-01, MS-02 | `RegisterResourceType`; `UC-02/sequence.puml` | `ResourceTypeService.RegisterResourceType`, `SchemaValidator.ValidateResourceTypeSchemas` | none before validation | OC-03; valid/invalid schema tests |
| MS-03, MS-04 | same | `ResourceTypeRepository.Exists/Save` | `resource_types`; unique `(org,key)` | OC-03; duplicate/repository tests |
| MS-05, MS-06 | same | return `ResourceType` / repository read contract | persisted schemas | OC-03; planner catalog integration test |

## UC-03

| Steps | Operation / sequence | Class methods | Persistence/state | Contract / tests |
|---|---|---|---|---|
| MS-01, MS-02, MS-03 | `RegisterResourceDefinition`; `UC-03/sequence.puml` | `ResourceDefinitionService.RegisterResourceDefinition`, `DefinitionValidator.ValidateStructure` | none before validation | OC-04; Terraform/Kubernetes definition tests |
| MS-04, MS-05 | same | type/connection repository reads; `ValidateReferencesAndRules` | `resource_types`, `connections` | OC-04; invalid reference test |
| MS-06, MS-07 | same | `ResourceDefinitionRepository.Exists`, `DriverContractInspector.Inspect` | source fingerprint candidate | OC-04; output mismatch test |
| MS-08, MS-09 | same | `ResourceDefinitionRepository.Save/FindCandidates` | `resource_definitions`, `matching_criteria` | OC-04; matching query integration test |

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
| MS-02, MS-03 | `PlanningService.Plan` | `ScoreConverter.ConvertAndValidate` | in-memory fragment | OC-07; Score fixture tests |
| MS-04 | same | `BeforeStateValidator.Validate`, `DeltaBuilder.Build` | in-memory Delta/Candidate | OC-07; delta invariant test |
| MS-05 | same | `ImplicitResourceEnricher.Enrich`, `ResourceGraphBuilder.BuildAndExpand`, `DefinitionMatcher.MatchAll` | in-memory graph/matches | OC-07; AWS/internal graph tests |
| MS-06, MS-07 | same | `DriverContractInspector.InspectContracts`, `ActiveResourceClassifier.Classify`, `BatchScheduler.Schedule` | in-memory plan | OC-07; contract/topology fixtures |
| MS-08 | `PreviewDeployment` | return `DeploymentPreview` | no write/state transition | OC-06; adapter-not-called test |

## UC-06

| Steps | Operation / sequence | Class methods | Persistence/state | Contract / tests |
|---|---|---|---|---|
| MS-01 | `DeployWorkload`; shared/cloud/internal sequences | `DeploymentService.DeployWorkload`, `EnvironmentRepository.LoadPlanningSnapshot`, `DeploymentRepository.Create` | `deployments=PLANNING`, base set/version | OC-08; both-profile tests |
| MS-02, MS-03 | `PlanningService.Plan` | Score/before/delta components | Candidate `deployment_sets` | OC-07/08; Delta invariant |
| MS-04, MS-05, MS-06 | same | profile load, `ImplicitResourceEnricher`, graph builder | graph JSON snapshot | OC-07; implicit resource tests |
| MS-07, MS-08 | same | matcher, inspector, classifier, scheduler | `deployment_plans`; `PROVISIONING` | OC-07/08; match/contract/DAG tests |
| MS-09 | `ResourceProvisioningService.Provision` | UC-08 methods | `active_resources`, `deployment_resources` | OC-10; resource integration tests |
| MS-10 | deploy | `OutputBindingResolver.ResolveWorkloadBindings` | resolved values stay in execution context; safe snapshot only | OC-08; output propagation test |
| MS-11 | deploy | `WorkloadRenderer.Render`, `WorkloadDeployer.Apply/WaitReady` | `workload_instances=APPLYING/READY` | OC-08; kind/fake adapter tests |
| MS-12 | deploy | `CompareVersionAndSetCurrent`, repo upserts, `MarkSucceeded` | atomic current pointer + `SUCCEEDED`; AWS runtime `READY` | OC-08; commit-after-ready transaction test |
| MS-13 | deploy | return `DeploymentResult` | read committed status | OC-08; API contract test |

## UC-07

| Steps | Operation / sequence | Class methods | Persistence/state | Contract / tests |
|---|---|---|---|---|
| MS-01, MS-02 | update/remove; `UC-07/sequence.puml` | load snapshot, `BeforeStateValidator.Validate` | base set/version | OC-09; before mismatch test |
| MS-03, MS-04 | `PlanningService.Plan` | Delta/graph/classifier | Candidate plan | OC-07/09; preserve-other/shared tests |
| MS-05 | `ResourceProvisioningService.Provision` | UC-08 methods | desired resources `READY` | OC-10; reconciliation test |
| MS-06 | update/remove | `WorkloadDeployer.Apply/WaitReady` or `Delete` | workload `READY` or `REMOVED` | OC-09; update/remove adapter tests |
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

## UC-09

| Steps | Operation / sequence | Class methods | Persistence/state | Contract / tests |
|---|---|---|---|---|
| MS-01, MS-02 | `GetDeployment`; `UC-09/sequence.puml` | `DeploymentQueryService.GetDeployment`, repo reads | Deployment/plan/set snapshot | OC-11; scoped query test |
| MS-03 | same | resource/workload repository reads | deployment-resource/instance status | OC-11; status assembly test |
| MS-04, MS-05 | same | `DeploymentViewAssembler.Assemble` | persisted graph/matches/batches | OC-11; view test |
| MS-06 | same | `OutputRedactor.RedactSecretOutputs` | no mutation | OC-11; redaction test |
| MS-07 | same | return `DeploymentView` | read-only | OC-11; no runtime call test |
