---
id: UC-06-REALIZATION
artifact: use-case-realization
status: current
last_reviewed: 2026-09-22
---

# UC-06 — Use Case Realization

## Trách nhiệm

Điều phối end-to-end Score -> plan -> UC-08 resource provisioning -> output binding -> Kubernetes workload apply -> atomic commit current Deployment Set. Planning semantics lấy từ `implementation/uc06-planner-reference.md`; execution không dùng challenge harness.

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
- `Deployment`, `DeploymentPlan`, `DeploymentDeltaSnapshot`, `DeploymentSet`, `ContainerResourceRequirements`, `ActiveResource`, `WorkloadInstance`.
- Application/Environment/Deployment/DeploymentDeltaSnapshot/DeploymentSet/ActiveResource/WorkloadInstance repositories.
- `UnitOfWork` cho create-plan và final commit transactions.

## Trace main flow

| Step | Collaboration |
|---|---|
| MS-01 | Create Deployment `PLANNING`, load/version current set. |
| MS-02–MS-03 | Planning pipeline validate/bảo toàn container resources, tạo Humanitec-shaped Delta/Candidate Set và chứng minh invariant. |
| MS-04–MS-08 | Load profile, resolve descriptor tokens, enrich graph, match/validate/schedule; persist immutable plan snapshot. |
| MS-09 | Chuyển Deployment `PROVISIONING`, gọi UC-08 với resource-only batches. |
| MS-10 | `OutputBindingResolver.ResolveWorkloadBindings`. |
| MS-11 | `WorkloadRenderer.Render` ánh xạ typed container requests/limits -> Kubernetes resources theo BR-11: giá trị khai báo giữ nguyên, request field thiếu lấy limit cùng field, thiếu cả hai thì nhận default `10m`/`32Mi`, limits chỉ gồm field khai báo; renderer không sửa module input. Sau đó `WorkloadDeployer.Apply` -> `WaitReady`. |
| MS-12 | Final transaction version-check Environment, set current Deployment Set, save instances/resources, mark AWS Application runtime `READY` và Deployment `SUCCEEDED`. |
| MS-13 | Trả `DeploymentResult`. |

## Profile realization

- `aws-eks`: `ImplicitResourceEnricher` thêm application-scoped VPC/EKS; UC-08 dùng Terraform Executor cho VPC, EKS, Aurora; target lấy từ EKS outputs.
- `internal-k8s`: enricher thêm existing cluster provider; UC-08 dùng connection adapter và Kubernetes Executor cho namespace/PostgreSQL; target lấy từ connection outputs.

## Transaction boundary

Không giữ DB transaction qua Terraform/Kubernetes call. Transaction A tạo Deployment và persist immutable Delta Snapshot/Candidate Set/plan snapshot. Mỗi UC-08 node persist progress bằng transaction ngắn. Transaction B sau readiness atomically đổi Environment current-set pointer, save WorkloadInstance và mark Deployment `SUCCEEDED`, với optimistic Environment version.

## Planned tests

- `TestDeployWorkload_AWSHappyPath`, `TestDeployWorkload_InternalHappyPath`.
- `TestDeployWorkload_CommitsCurrentSetOnlyAfterReadiness`.
- `TestDeployWorkload_ResolvesResourceOutputsBeforeRender`.
- `TestDeployWorkload_PersistsHumanitecShapedDelta`.
- `TestDeployWorkload_RendersDeclaredContainerResources`.
- `TestParseDescriptorText_ExpandsScopedTokens`.
- `TestDeployAcceptance_FrontendBackendWorkerSharedDatabase`.
