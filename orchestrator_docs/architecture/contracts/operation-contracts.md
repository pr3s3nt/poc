---
id: OPERATION-CONTRACTS
artifact: operation-contracts
status: current
last_reviewed: 2026-09-23
---

# Operation Contracts

Các contract dưới đây dùng tên method cố định cho realization và Go implementation.

## OC-00 `AuthenticationService.SignIn` / `SignOut`

- Use case: UC-00 MS-01–MS-05.
- Preconditions: fixed internal account tồn tại, `ACTIVE`; test accounts chỉ được load ở profile `local`/`test`.
- Creates: opaque Session gắn với User, Organization và role; delivery đặt raw token vào same-origin `HttpOnly` cookie.
- Postconditions: raw token không được persist/log; only token hash được lưu; revoked/expired session không authenticate được.
- Persistence: insert/revoke `sessions` trong một transaction; không mutate UserAccount khi sign-in thành công.

## OC-01 `ApplicationService.CreateApplication`

- Use case: UC-01 MS-01–MS-08.
- Preconditions: Organization tồn tại; default execution target có connection `READY`; Name/Subdomain hợp lệ và chưa trùng.
- Creates: system-ID `Application` với profile/connection binding, cùng `staging` và `production` Environments, immutable empty Deployment Sets và stable namespace identities.
- Postconditions: AWS Application `PENDING`; internal Application bind existing cluster; desired endpoints được suy ra nhưng chưa có infrastructure, workload hoặc route/ingress.
- Persistence: insert Application, hai Environment, hai Deployment Set và current pointers atomically trong một transaction.

## OC-03 `ResourceTypeService.RegisterResourceType`

- Use case: UC-02.
- Preconditions: schemas hợp lệ; key chưa tồn tại trong Organization.
- Creates: `ResourceType` contract.
- Postconditions: available cho Score validation và Resource Definition registration.
- Persistence: insert `resource_types`; unique constraint là final duplicate guard.

## OC-04 `ResourceDefinitionService.RegisterResourceDefinition`

- Use case: UC-03.
- Preconditions: Resource Type, Driver/connection tồn tại; có ít nhất một
  Matching Criterion; references/rules hợp lệ; output contract tương thích.
- Creates: `ResourceDefinition` và một hoặc nhiều `MatchingCriterion`.
- Postconditions: queryable cho deterministic matching; `{}` tường minh là
  wildcard điểm `0`; source fingerprint được ghi nếu có.
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
- Returns: `DeploymentPreview` gồm transient Humanitec-shaped Delta, Candidate Set, graph, matches, batches, classification và environment version.
- Postconditions: không persist Deployment; không gọi executor/deployer; runtime state không đổi.
- Invariant: base set + Delta = Candidate Set.

## OC-07 `PlanningService.Plan`

- Use cases: UC-05, UC-06, UC-07.
- Preconditions: immutable base snapshot, Resource Types/Definitions/connections và Active Resources đã load.
- Returns: deterministic `DeploymentPlan` với plan hash.
- Delta rule: output có `modules.add/remove/update` và `shared`; module/shared patches có relative root đúng contract, array diff deterministic và `base + delta = candidate`.
- Workload rule: optional `containers.*.resources.requests/limits` được validate và bảo toàn trong Candidate module; không biến thành resource node.
- Before-state rule: module và từng shared entry do `before Score` khai báo phải deep-equal current set.
- Candidate rule: một shared ID mới không được ghi đè entry khác nội dung; shared entry bị workload bỏ chỉ rời Candidate khi không còn module khác tham chiếu.
- Descriptor rule: `@app`, `@env`, `@connection` được resolve từ planning
  context trước khi parse descriptor; `@` kế thừa class/ID hiện tại giữ nguyên
  semantics.
- Postconditions: graph là DAG; mỗi resource node match đúng một Definition; contracts valid; provider-first batches; workload node không thuộc resource-execution batches.
- Side effects: none.

## OC-08 `DeploymentService.DeployWorkload`

- Use case: UC-06.
- Preconditions: PRE-01..PRE-05; Environment version khớp base snapshot.
- Creates: Deployment, immutable DeploymentDeltaSnapshot, Candidate Set, DeploymentPlan, deployment-resource progress, Active Resources/Workload Instance as execution proceeds.
- Postconditions: UC-08 complete; workload ready với declared container requests/limits; current-set pointer atomically đổi; AWS Application runtime `READY`; Deployment `SUCCEEDED`.
- External side effects: Terraform/Kubernetes outside DB transaction.
- Commit rule: Candidate Set never becomes current before readiness.

## OC-09 `DeploymentService.UpdateWorkload` / `RemoveWorkload`

- Use case: UC-07.
- Preconditions: before Score deep-equals current module và từng shared entry mà workload khai báo.
- Updates: only target workload contribution and legitimately owned shared contribution through Humanitec-shaped module/shared Delta; conflicting shared ID is rejected before execution.
- Postconditions: other modules preserved; referenced shared resources preserved; stale resource `UNREFERENCED`; no destroy.
- Commit rule: same optimistic final transaction as OC-08.

## OC-10 `ResourceProvisioningService.Provision`

- Use case: UC-08.
- Preconditions: persisted immutable plan; resource-only DAG/batches; matched Definitions; connections `READY`.
- For each node: resolve prior state and inputs -> executor provision/reconcile -> validate outputs -> atomically persist Active Resource + deployment-resource progress.
- Returns: `ProvisionResult` mapping descriptor to outputs and deployment target.
- Postconditions: all desired resource nodes `READY`; output bindings available for workload render.
- Idempotency key: logical identity `(organization, descriptor, scope_type, scope_id)` plus executor state.
- Boundary: container CPU/memory requests/limits are not UC-08 resources; UC-06 Workload Renderer applies them to manifests.

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
