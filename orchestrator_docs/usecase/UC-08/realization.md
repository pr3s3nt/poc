---
id: UC-08-REALIZATION
artifact: use-case-realization
status: current
last_reviewed: 2026-10-26
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
| MS-06 | State backend acknowledge; kiểm Type outputs, provider completion và mọi supplemental readiness áp dụng theo BR-10; store Secret refs. |
| MS-07 | Persist Active Resource/executor state + deployment-resource status trong transaction ngắn. |
| MS-08–MS-09 | Thêm outputs vào run context, tiếp tục batch kế tiếp. |
| MS-10 | Trả `ProvisionResult` map descriptor -> outputs/target. |

## Profile realization

- `aws-eks`: Terraform AWS cho VPC/EKS/ElastiCache/Aurora; Kubernetes provider
  cho namespace/Redis/PostgreSQL theo class, bind EKS target đã Ready.
- `internal-k8s`: ExistingClusterAdapter trả cluster target; Terraform
  Kubernetes modules tạo namespace/Redis/PostgreSQL.

## Transaction boundary

Theo BR-10, Terraform provider completion áp dụng cho mọi loại managed resource
trong verified module. `ReadinessChecker` kiểm đủ address/type/ID và deadline,
sau đó chạy mọi probe Kubernetes/AWS hiện có cho các objects tương ứng. Module
không có object cần probe bổ sung vẫn hoàn tất; module hỗn hợp không được bỏ
qua probe thất bại. EKS target usability và namespace provenance giữ nguyên.

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

Shared snapshot keeps legacy Environment graph target until V03. Definition
ranking follows UC-03 V02-02 without profile filtering; resolve each AWS winner's
scoped READY Driver Account and Driver Inputs region, not Environment identity.
Planner, snapshot projection and pre-provision share safe selected identity pins;
provider invocations privately resolve that account only. Account/type/READY or
state target conflict fails without fallback. Definition/account version/content
join Preview hashes and admission stale checks. Mutable catalog edits do not
change historical plan/resource/cleanup pins, secret refs or module/state identity.
Context, version, scope and target metadata remain pinned.
AWS enrichment produces Environment/generation-scoped VPC/EKS only (T27).
Retired scopes fail closed; current generation-0 identity is preserved under [ADR-011](../../architecture/decisions/ADR-011-environment-execution-binding.md).
Target changes use ADR-012 versioned admission/transition; runtime READY update is Environment-only.
UI/query use selected Environment or deployment-pinned target, no Application target.

## Matched internal cluster collaboration

ADR-016 replaces the system provider. The workload Definition contributes
namespace/cluster references and provision rules before fixed-point expansion.
The same matcher selects existing-cluster and pins its own account. Target
resolution validates each consumer against that cluster and namespace; no
reserved Definition admission or global target inference. UC-08 resolves the
preexisting cluster before namespace/resources; UC-06 uses its own selected
workload target and applied identity across restart/update/remove.

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


## V03 collaboration replacement

[ADR-016](../../architecture/decisions/ADR-016-definition-selected-workload-targets.md) replaces internal profile enrichment/system binding in the collaboration above. Catalog validates public existing-cluster/Internal account and workload typed namespace/cluster refs plus provision rules. Planning matches workload first and expands all nodes to fixed point; same normal matcher selects cluster Definition. Target resolver builds/pins per-consumer placement, rejects mismatch/ambiguous/cycle/multi-cluster V03. Shared snapshot/admission compare pins/versions; UC-08 resolves cluster then owned namespace/resources. UC-06 renders/applies/waits on selected workload target; no global target or builtin persistence. Applied target restored for Update/Remove/restart; separate cleanup preserves preexisting cluster. AWS legacy collaboration remains until V05–V06.


## V04–V06 collaboration

[ADR-017](../../architecture/decisions/ADR-017-definition-dependency-execution.md)
replaces the remaining profile-driven collaboration above. Select workload
Definition, expand all authored references/provision/provider bindings to fixed
point, validate full DAG and pin each node's Definition/account/region/placement.
Planning keeps unknown typed outputs and safe provenance without provider calls.
Provisioning dispatches matched driver with private invocation credentials,
validates completed outputs/readiness and persists original state/target pins.
Namespace/workload resolve their explicit cluster; EKS target auth is reconstructed
per invocation after API/network/node readiness. OutputBindingResolver renders
only after dependencies pass. Failure blocks consumers and preserves progress.
Cleanup walks reverse actual graph using original state/module/identity and
ownership/UID guards, protecting shared/preexisting objects. See cloud sequence
and module/state contracts for target and secret boundaries.

## Visual review execution contract

UC-08 REV-* uses [ADR-019](../../architecture/decisions/ADR-019-visual-deployment-review.md)
and the [review contract](../../architecture/contracts/deployment-review.md).
Admission preserves server reviewRequestId and safe desired review in the immutable
plan; resource execution persists stage/attempt/readiness facts. Workload owner
captures redacted rendered manifest before Apply and associates applied baseline
only after that workload is READY. All writes use existing owner/fence and short
transactions; no network/renderer subprocess spans a DB transaction. UC-09 reads
these records; no simulation timer, polling-triggered mutation or new scheduler.
