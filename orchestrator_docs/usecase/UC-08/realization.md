---
id: UC-08-REALIZATION
artifact: use-case-realization
status: current
last_reviewed: 2026-10-06
---

# UC-08 — Use Case Realization

## Trách nhiệm

Thực thi resource-only batches theo provider-first order, resolve runtime inputs, gọi executor phù hợp, validate outputs và persist Active Resource state cho consumer tiếp theo.

Container `resources.requests/limits` không phải infrastructure resource input của UC-08. Chúng được giữ trong workload module và được `WorkloadRenderer` của UC-06 ánh xạ sang Kubernetes manifests sau khi UC-08 trả outputs/target.

## System operation

```go
ResourceProvisioningService.Provision(ctx context.Context, req ProvisionRequest) (*ProvisionResult, error)
```

## Participants

- `ResourceProvisioningService` — control loop theo batches.
- `ActiveResourceResolver` — tìm logical resource theo descriptor/scope.
- `OutputBindingResolver` — resolve context/resource-output binding.
- `ExecutorRegistry` — map Driver Type sang `ResourceExecutor`.
- `TerraformExecutor`, `KubernetesExecutor`, `ExistingClusterAdapter`.
- `OutputContractValidator`.
- `ActiveResourceRepository`, `DeploymentResourceRepository`, `UnitOfWork`.

## Trace main flow

| Step | Collaboration |
|---|---|
| MS-01 | Nhận immutable graph/resource batches từ persisted DeploymentPlan. |
| MS-02 | Resolve Active Resource bằng `(organization, descriptor, scope)`. |
| MS-03 | Resolve context, resource inputs và outputs từ completed providers. |
| MS-04–MS-05 | Resolve executor rồi `ResourceExecutor.Provision`. |
| MS-06 | Validate outputs theo Resource Type. |
| MS-07 | Persist Active Resource/executor state + deployment-resource status trong transaction ngắn. |
| MS-08–MS-09 | Thêm outputs vào run context, tiếp tục batch kế tiếp. |
| MS-10 | Trả `ProvisionResult` map descriptor -> outputs/target. |

## Profile realization

- `aws-eks`: Terraform Executor cho VPC -> EKS và VPC -> Aurora.
- `internal-k8s`: ExistingClusterAdapter trả cluster target; Kubernetes Executor apply namespace và PostgreSQL StatefulSet/Service.

## Transaction boundary

Không có distributed transaction. Mỗi external provision call nằm ngoài DB transaction; kết quả của từng node được persist ngay bằng upsert idempotent theo logical identity. Happy path chạy batches tuần tự; concurrency trong batch có thể bổ sung mà không đổi contract.

## Planned tests

- `TestProvision_ProviderOutputsFeedConsumerInputs`.
- `TestProvision_ReusesApplicationScopedVPCAndEKS`.
- `TestProvision_InternalUsesExistingCluster`.
- `TestProvision_PostgresOutputContractEquivalentAcrossProfiles`.

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
