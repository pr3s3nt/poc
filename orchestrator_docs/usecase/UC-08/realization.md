---
id: UC-08-REALIZATION
artifact: use-case-realization
status: current
last_reviewed: 2026-10-13
---

# UC-08 — Use Case Realization

## Trách nhiệm

Thực thi resource-only batches theo provider-first order, resolve runtime inputs, gọi executor phù hợp, validate outputs và persist Active Resource state cho consumer tiếp theo.

Planner hợp nhất các references cùng VPC descriptor thành một provider node.
Terraform VPC tạo private network và trả `privateSubnetIds`; provisioning resolve
output vào `private_subnet_ids` của các Aurora/ElastiCache consumers. Mạng thuộc
state của VPC, không thuộc state database/cache. Cleanup engine giữ mạng khi xóa
một consumer và kiểm mọi consumer trước khi destroy VPC theo reverse graph.

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
- `TerraformExecutor`, `ExistingClusterAdapter` (chỉ resolve cluster có sẵn).
- `ModulePreflight`, `TerraformStateBackend/Repository/Cipher`,
  `ResourceInstanceRepository`, `ResourceAttemptRepository`: kiểm bundle,
  ghi intent/ownership, lưu encrypted state/locks và stage dưới owner/fence.
- `OutputContractValidator`.
- `ActiveResourceRepository`, `DeploymentResourceRepository`, `UnitOfWork`.

## Trace main flow

| Step | Collaboration |
|---|---|
| MS-01 | Nhận immutable graph/resource batches từ persisted DeploymentPlan. |
| MS-02 | Resolve instance/state theo Organization/Environment/generation/descriptor/ scope; persist physical intent/action/ownership trước call. |
| MS-03 | Resolve context, resource inputs và outputs từ completed providers. |
| MS-04–MS-05 | Kiểm module/input digests; `ResourceExecutor.Execute` CREATE hoặc non-destructive UPDATE dưới Environment owner và state lock. |
| MS-06 | State backend acknowledge; kiểm Type outputs/readiness và store Secret refs. |
| MS-07 | Persist Active Resource/executor state + deployment-resource status trong transaction ngắn. |
| MS-08–MS-09 | Thêm outputs vào run context, tiếp tục batch kế tiếp. |
| MS-10 | Trả `ProvisionResult` map descriptor -> outputs/target. |

## Profile realization

- `aws-eks`: Terraform AWS cho VPC/EKS/ElastiCache/Aurora; Kubernetes provider
  cho namespace/Redis/PostgreSQL theo class, bind EKS target đã Ready.
- `internal-k8s`: ExistingClusterAdapter trả cluster target; Terraform
  Kubernetes modules tạo namespace/Redis/PostgreSQL.

## Transaction boundary

Không có distributed transaction. Mỗi external provision call nằm ngoài DB transaction; kết quả của từng node được persist ngay bằng upsert idempotent theo logical identity. Happy path chạy batches tuần tự; concurrency trong batch có thể bổ sung mà không đổi contract.

## Planned tests

- `TestProvision_ProviderOutputsFeedConsumerInputs`.
- Environment/generation-scoped AWS isolation, exact generation-0 identities and retired-scope rejection tests.
- `TestProvision_InternalUsesExistingCluster`.
- `TestProvision_PostgresOutputContractEquivalentAcrossProfiles`.

## UC-04 credential-backed target integration

[Shared credential design](../../architecture/connection-credentials.md) owns
resolution. Carry only Organization/Connection identity through fresh and
reused resource targets and persisted WorkloadInstance TargetRef. Resolve
credential at each adapter call; never persist normalized kubeconfig or temp
paths in outputs/state/plan. Missing/not-READY Connection fails closed. Restore
the same scoped target for removal and route reconciliation after restart.
T22 removes host-context targets: context-only/unscoped execution fails closed.
Internal Kubernetes requires scoped KUBECONFIG; EKS target có scoped identity tái tạo credential cho từng invocation,
không persist kubeconfig/path. Kết quả code thuộc CURRENT_STATE.

## Definition-selected workload rendering collaboration

See [ADR-010](../../architecture/decisions/ADR-010-score-k8s-workload-rendering.md)
and [rendering contract](../../architecture/contracts/workload-rendering.md).
Catalog validates the installed bundle; shared planning selects/pins the renderer.
UC-06 validates availability before provisioning, then dispatches the existing
WorkloadRenderer port after output binding. UC-08 still executes resource-only
batches. Kubernetes/Fleet apply/readiness and Environment route ownership stay
in their existing adapters. No database transaction spans a CLI/external call.

## Selected Environment connection

Shared snapshot loader resolves persisted Environment target in session
Organization; UNCONFIGURED fails before planning/execution. Definition guards and
UC-08 defense-in-depth compare against this Environment connection. Terraform VPC/EKS use the pinned Environment AWS account even though the reusable Definition key is empty; resolve and validate org/kind/READY before execution and persist the effective account in ActiveResource. A malformed conflicting winning Definition fails without fallback. Aurora/ElastiCache cũng dùng Environment account/region; không override. Context,
version, scope mode and nonsecret target metadata are pinned into hashes/snapshots.
AWS enrichment produces Environment/generation-scoped VPC/EKS only (T27).
Retired scopes fail closed; current generation-0 identity is preserved under [ADR-011](../../architecture/decisions/ADR-011-environment-execution-binding.md).
Target changes use ADR-012 versioned admission/transition; runtime READY update is Environment-only.
UI/query use selected Environment or deployment-pinned target, no Application target.

## Implicit internal cluster collaboration

[ADR-013](../../architecture/decisions/ADR-013-implicit-existing-cluster.md)
changes only internal cluster binding: the enricher supplies the provider node,
trusted system binding bypasses criteria matching, and UC-08 resolves the selected
Environment Connection through existing-cluster. Preview is pure; execution
admits the reserved system Definition before persistence writes. Other resource
Definitions, executor scheduling and output propagation remain unchanged.


## State, retry và cleanup

Terraform init/validate/plan/apply dùng immutable module/provider pins; UPDATE
plan có delete/replace bị chặn. Backend lock/write/unlock kiểm current owner/
fence/lineage/serial; ciphertext/state revision commit nguyên tử. State error
không được báo READY. Generated password ref có trước apply và được dùng lại.
Retry transient một lần sau kiểm state/outcome, trong deadline; unknown outcome
giữ hold/private recovery handle, không replay CREATE hoặc tự import/resume.

ResourceCleanupService xử lý tập được user hoặc test-run authorization giao,
khóa state, destroy consumers trước providers và kiểm absence. UI eligibility/
confirmation do UC-07 sở hữu; engine không mở rộng scope hoặc xóa platform.
[Cleanup sequence](sequence-cleanup.puml), [state contract](../../architecture/contracts/terraform-state.md)
và OC-18/19 sở hữu protocol; D12–D14 là backlog riêng.
