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
- `PlanningService` và planner components được liệt kê tại UC-05.
- `ResourceProvisioningService` — UC-08 control.
- `OutputBindingResolver`, `WorkloadRenderer`, `WorkloadDeployer`.
- `Deployment`, `DeploymentPlan`, `DeploymentSet`, `ActiveResource`, `WorkloadInstance`.
- Application/Environment/Deployment/DeploymentSet/ActiveResource/WorkloadInstance repositories.
- `UnitOfWork` cho create-plan và final commit transactions.

## Trace main flow

| Step | Collaboration |
|---|---|
| MS-01 | Create Deployment `PLANNING`, load/version current set. |
| MS-02–MS-03 | Planning pipeline tạo Delta/Candidate Set. |
| MS-04–MS-08 | Load profile, enrich graph, match/validate/schedule; persist immutable plan snapshot. |
| MS-09 | Chuyển Deployment `PROVISIONING`, gọi UC-08 với resource-only batches. |
| MS-10 | `OutputBindingResolver.ResolveWorkloadBindings`. |
| MS-11 | `WorkloadRenderer.Render` -> `WorkloadDeployer.Apply` -> `WaitReady`. |
| MS-12 | Final transaction version-check Environment, set current Deployment Set, save instances/resources, mark AWS Application runtime `READY` và Deployment `SUCCEEDED`. |
| MS-13 | Trả `DeploymentResult`. |

## Profile realization

- `aws-eks`: `ImplicitResourceEnricher` thêm application-scoped VPC/EKS; UC-08 dùng Terraform Executor cho VPC, EKS, Aurora; target lấy từ EKS outputs.
- `internal-k8s`: enricher thêm existing cluster provider; UC-08 dùng connection adapter và Kubernetes Executor cho namespace/PostgreSQL; target lấy từ connection outputs.

## Transaction boundary

Không giữ DB transaction qua Terraform/Kubernetes call. Transaction A tạo Deployment và persist plan snapshot. Mỗi UC-08 node persist progress bằng transaction ngắn. Transaction B sau readiness atomically đổi Environment current-set pointer, save WorkloadInstance và mark Deployment `SUCCEEDED`, với optimistic Environment version.

## Planned tests

- `TestDeployWorkload_AWSHappyPath`, `TestDeployWorkload_InternalHappyPath`.
- `TestDeployWorkload_CommitsCurrentSetOnlyAfterReadiness`.
- `TestDeployWorkload_ResolvesResourceOutputsBeforeRender`.
- `TestDeployAcceptance_FrontendBackendWorkerSharedDatabase`.
