# Operation Contracts

Các contract dưới đây dùng tên method cố định cho realization và Go implementation.

## OC-01 `ApplicationService.CreateApplication`

- Use case: UC-01 MS-01–MS-04.
- Preconditions: Organization tồn tại; connection `READY`; application key chưa tồn tại.
- Creates: `Application` với profile/connection binding và version 1.
- Postconditions: AWS Application `PENDING`; internal Application bind existing cluster; chưa có infrastructure.
- Persistence: insert `applications` trong một transaction.

## OC-02 `ApplicationService.CreateEnvironment`

- Use case: UC-01 MS-05–MS-08.
- Preconditions: Application tồn tại.
- Creates: `Environment`, immutable empty `DeploymentSet`, stable namespace identity.
- Postconditions: Environment current-set pointer trỏ empty set; version 1.
- Persistence: insert environment/set và current pointer atomically.

## OC-03 `ResourceTypeService.RegisterResourceType`

- Use case: UC-02.
- Preconditions: schemas hợp lệ; key chưa tồn tại trong Organization.
- Creates: `ResourceType` contract.
- Postconditions: available cho Score validation và Resource Definition registration.
- Persistence: insert `resource_types`; unique constraint là final duplicate guard.

## OC-04 `ResourceDefinitionService.RegisterResourceDefinition`

- Use case: UC-03.
- Preconditions: Resource Type, Driver/connection tồn tại; references/rules hợp lệ; output contract tương thích.
- Creates: `ResourceDefinition` và một hoặc nhiều `MatchingCriterion`.
- Postconditions: queryable cho deterministic matching; source fingerprint được ghi nếu có.
- Persistence: definition + criteria atomically.

## OC-05 `ConnectionService.RegisterAWSDriverAccount` / `RegisterKubernetesCluster`

- Use case: UC-04.
- Preconditions: credential chỉ tồn tại trong request memory; identity/connectivity/RBAC verify thành công.
- Creates: secret in Secret Store; `Connection` record với opaque secret reference.
- Postconditions: Connection `READY`; database không chứa secret value.
- External side effect: Secret Store write trước database insert.

## OC-06 `PreviewService.PreviewDeployment`

- Use case: UC-05.
- Preconditions: Planning snapshot đọc được; Score hợp lệ.
- Returns: `DeploymentPreview` gồm Delta, Candidate Set, graph, matches, batches, classification và environment version.
- Postconditions: không persist Deployment; không gọi executor/deployer; runtime state không đổi.
- Invariant: base set + Delta = Candidate Set.

## OC-07 `PlanningService.Plan`

- Use cases: UC-05, UC-06, UC-07.
- Preconditions: immutable base snapshot, Resource Types/Definitions/connections và Active Resources đã load.
- Returns: deterministic `DeploymentPlan` với plan hash.
- Before-state rule: module và từng shared entry do `before Score` khai báo phải deep-equal current set.
- Candidate rule: một shared ID mới không được ghi đè entry khác nội dung; shared entry bị workload bỏ chỉ rời Candidate khi không còn module khác tham chiếu.
- Postconditions: graph là DAG; mỗi resource node match đúng một Definition; contracts valid; provider-first batches; workload node không thuộc resource-execution batches.
- Side effects: none.

## OC-08 `DeploymentService.DeployWorkload`

- Use case: UC-06.
- Preconditions: PRE-01..PRE-05; Environment version khớp base snapshot.
- Creates: Deployment, Candidate Set, DeploymentPlan, deployment-resource progress, Active Resources/Workload Instance as execution proceeds.
- Postconditions: UC-08 complete; workload ready; current-set pointer atomically đổi; AWS Application runtime `READY`; Deployment `SUCCEEDED`.
- External side effects: Terraform/Kubernetes outside DB transaction.
- Commit rule: Candidate Set never becomes current before readiness.

## OC-09 `DeploymentService.UpdateWorkload` / `RemoveWorkload`

- Use case: UC-07.
- Preconditions: before Score deep-equals current module và từng shared entry mà workload khai báo.
- Updates: only target workload contribution and legitimately owned shared contribution; conflicting shared ID is rejected before execution.
- Postconditions: other modules preserved; referenced shared resources preserved; stale resource `UNREFERENCED`; no destroy.
- Commit rule: same optimistic final transaction as OC-08.

## OC-10 `ResourceProvisioningService.Provision`

- Use case: UC-08.
- Preconditions: persisted immutable plan; resource-only DAG/batches; matched Definitions; connections `READY`.
- For each node: resolve prior state and inputs -> executor provision/reconcile -> validate outputs -> atomically persist Active Resource + deployment-resource progress.
- Returns: `ProvisionResult` mapping descriptor to outputs and deployment target.
- Postconditions: all desired resource nodes `READY`; output bindings available for workload render.
- Idempotency key: logical identity `(organization, descriptor, scope_type, scope_id)` plus executor state.

## OC-11 `DeploymentQueryService.GetDeployment`

- Use case: UC-09.
- Preconditions: scoped Deployment exists and actor is authorized at boundary.
- Returns: persisted status, set snapshot, graph, matches, batches, resource/workload status and non-secret outputs.
- Postconditions: no mutation and no runtime adapter call.

## Shared error/consistency rules

- Optimistic conflict uses Environment `version`; no last-write-wins on current-set pointer.
- Domain errors are typed Go errors; delivery maps them to API status later.
- Failed external execution may mark Deployment/Resource `FAILED`, but retry/resume/rollback remains outside current happy path.
- Secret values never enter logs, plan hash input exposed to clients, database JSONB or UC-09 view.
