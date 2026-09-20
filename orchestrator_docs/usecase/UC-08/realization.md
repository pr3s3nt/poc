# UC-08 — Use Case Realization

## Trách nhiệm

Thực thi resource-only batches theo provider-first order, resolve runtime inputs, gọi executor phù hợp, validate outputs và persist Active Resource state cho consumer tiếp theo.

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
