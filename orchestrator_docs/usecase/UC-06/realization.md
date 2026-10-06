---
id: UC-06-REALIZATION
artifact: use-case-realization
status: current
last_reviewed: 2026-10-06
---

# UC-06 — Use Case Realization

## Trách nhiệm

Điều phối end-to-end Score -> plan -> UC-08 resource provisioning -> output binding -> Kubernetes workload apply -> optional internal-kind Ingress reconcile -> atomic commit current Deployment Set. Planning semantics lấy từ `implementation/uc06-planner-reference.md`; execution không dùng challenge harness.

## System operations

```go
DeploymentService.DeployWorkload(ctx context.Context, cmd DeployWorkloadCommand) (*DeploymentResult, error)
PlanningService.Plan(ctx context.Context, req PlanRequest) (*DeploymentPlan, error)
ResourceProvisioningService.Provision(ctx context.Context, req ProvisionRequest) (*ProvisionResult, error)
WorkloadRenderer.Render(ctx context.Context, req RenderWorkloadRequest) ([]Manifest, error)
WorkloadDeployer.Apply(ctx context.Context, target DeploymentTarget, manifests []Manifest) error
WorkloadDeployer.WaitReady(ctx context.Context, target DeploymentTarget, workloads []WorkloadRef) error
```

## Participants

- `DeploymentController`, `DeploymentService` — boundary/control.
- `PlanningService` và planner components được liệt kê tại UC-05;
  `DeltaBuilder` trả typed Delta document có Humanitec shape và
  `ResourceDescriptorParser.ParseDescriptorText` resolve scoped tokens BR-12.
- `ResourceProvisioningService` — UC-08 control.
- `OutputBindingResolver`, `WorkloadRenderer`, `WorkloadDeployer`.
- `PublicRouteManager` — trên internal kind, reconcile toàn bộ Ingress cấp Environment;
  adapter Kubernetes direct hoặc GitOps `_routes` theo delivery mode.
- Pending batch giữ `Environment.publicRoutesPending` trước khi áp dụng;
  nếu route reconcile lỗi sau workload commit, Preview cho retry route-only.
- `Deployment`, `DeploymentPlan`, `DeploymentDeltaSnapshot`, `DeploymentSet`, `ContainerResourceRequirements`, `ActiveResource`, `WorkloadInstance`.
- Application/Environment/Deployment/DeploymentDeltaSnapshot/DeploymentSet/ActiveResource/WorkloadInstance repositories.
- `UnitOfWork` cho create-plan và final commit transactions.

## Trace main flow

| Step | Collaboration |
|---|---|
| MS-01 | Create Deployment `PLANNING` without Delta Snapshot, load/version current set. Planning failure keeps a `FAILED` record, which may also lack Snapshot (BR-16). |
| MS-02–MS-03 | Planning pipeline validate/bảo toàn container resources, tạo Humanitec-shaped Delta/Candidate Set và chứng minh invariant. |
| MS-04–MS-08 | Load profile, resolve descriptor tokens, enrich graph, match/validate/schedule; persist immutable plan snapshot. |
| MS-09 | Chuyển Deployment `PROVISIONING`, gọi UC-08 với resource-only batches. |
| MS-10 | `OutputBindingResolver.ResolveWorkloadBindings`. |
| MS-11 | `WorkloadRenderer.Render` ánh xạ typed container requests/limits -> Kubernetes resources theo BR-11: giá trị khai báo giữ nguyên, request field thiếu lấy limit cùng field, thiếu cả hai thì nhận default `10m`/`32Mi`, limits chỉ gồm field khai báo; renderer không sửa module input. Sau đó `WorkloadDeployer.Apply` -> `WaitReady`; internal kind reconcile toàn bộ Ingress sau khi các workload bị ảnh hưởng Ready. Fleet mode ghi/chờ bundle `_routes`. |
| MS-12 | Final transaction version-check Environment, set current Deployment Set, save instances/resources, mark AWS Application runtime `READY` và Deployment `SUCCEEDED`. |
| MS-13 | Trả `DeploymentResult`. |

## Profile realization

- `aws-eks`: `ImplicitResourceEnricher` thêm application-scoped VPC/EKS; UC-08 dùng Terraform Executor cho VPC, EKS, Aurora; target lấy từ EKS outputs.
- `internal-k8s`: enricher thêm existing cluster provider; UC-08 dùng connection adapter và Kubernetes Executor cho namespace/PostgreSQL; target lấy từ connection outputs.

## Transaction boundary

Không giữ DB transaction qua Terraform/Kubernetes call. Initial insert tạo
Deployment `PLANNING` với nullable Snapshot. Transaction A persist immutable
Delta Snapshot/Candidate Set/plan snapshot và gắn Snapshot trước khi chuyển
`PROVISIONING`. Một `FAILED` trong planning có thể không có Snapshot. Mỗi UC-08
node persist progress bằng transaction ngắn. Transaction B sau readiness
atomically đổi Environment current-set pointer, save WorkloadInstance và mark
Deployment `SUCCEEDED`, với optimistic Environment version.

## Planned tests

- `TestDeployWorkload_AWSHappyPath`, `TestDeployWorkload_InternalHappyPath`.
- `TestDeployWorkload_CommitsCurrentSetOnlyAfterReadiness`.
- `TestDeployWorkload_ResolvesResourceOutputsBeforeRender`.
- `TestDeployWorkload_PersistsHumanitecShapedDelta`.
- `TestDeployWorkload_RendersDeclaredContainerResources`.
- `TestParseDescriptorText_ExpandsScopedTokens`.
- `TestDeployAcceptance_FrontendBackendWorkerSharedDatabase`.

## UC-04 credential-backed target integration

[Shared credential design](../../architecture/connection-credentials.md) owns
resolution. Carry only Organization/Connection identity through fresh and
reused resource targets and persisted WorkloadInstance TargetRef. Resolve
credential at each adapter call; never persist normalized kubeconfig or temp
paths in outputs/state/plan. Missing/not-READY Connection fails closed. Restore
the same scoped target for removal and route reconciliation after restart.
Legacy host-context targets stay supported. This is the approved design for
UC-04 upload delivery; implementation evidence is tracked in current state.

## Definition-selected workload rendering collaboration

See [ADR-010](../../architecture/decisions/ADR-010-score-k8s-workload-rendering.md)
and [rendering contract](../../architecture/contracts/workload-rendering.md).
Catalog validates the installed bundle; shared planning selects/pins the renderer.
UC-06 validates availability before provisioning, then dispatches the existing
WorkloadRenderer port after output binding. UC-08 still executes resource-only
batches. Kubernetes/Fleet apply/readiness and Environment route ownership stay
in their existing adapters. No database transaction spans a CLI/external call.
