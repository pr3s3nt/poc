---
id: UC-06-REALIZATION
artifact: use-case-realization
status: current
last_reviewed: 2026-10-24
---

# UC-06 — Use Case Realization

## Trách nhiệm

Điều phối end-to-end Score -> plan -> UC-08 resource provisioning -> output binding -> Kubernetes workload apply -> optional internal-kind Ingress reconcile -> atomic commit current Deployment Set. Planning semantics thuộc specification UC-03/05/06; reference analysis là
nguồn tham khảo, không thay requirements.

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
- `EnvironmentOperationManager`, `ModulePreflight`, `RenderPreflight`: kiểm
  snapshot/artifacts và nhận owner/fence trước UC-08; release sau process stopped.
- `UnitOfWork` cho create-plan và final commit transactions.

## Trace main flow

| Step | Collaboration |
|---|---|
| MS-01 | Create Deployment `PLANNING` without Delta Snapshot, load/version current set. Planning failure keeps a `FAILED` record, which may also lack Snapshot (BR-16). |
| MS-02–MS-03 | Planning pipeline validate/bảo toàn container resources, tạo Humanitec-shaped Delta/Candidate Set và chứng minh invariant. |
| MS-04–MS-08 | Resolve/pin Environment target/profile (Application identity only), resolve descriptor tokens, enrich graph, match/validate/schedule; persist immutable plan snapshot. |
| MS-09 | Chuyển Deployment `PROVISIONING`, gọi UC-08 với resource-only batches. |
| MS-10 | `OutputBindingResolver.ResolveWorkloadBindings`. |
| MS-11 | `WorkloadRenderer.Render` ánh xạ typed container requests/limits -> Kubernetes resources theo BR-11: giá trị khai báo giữ nguyên, request field thiếu lấy limit cùng field, thiếu cả hai thì nhận default `10m`/`32Mi`, limits chỉ gồm field khai báo; renderer không sửa module input. Sau đó `WorkloadDeployer.Apply` -> `WaitReady`; internal kind reconcile toàn bộ Ingress sau khi các workload bị ảnh hưởng Ready. Fleet mode ghi/chờ bundle `_routes`. |
| MS-12 | Final transaction version-check Environment, set current Deployment Set, save instances/resources, mark AWS Environment runtime `READY` và Deployment `SUCCEEDED`. |
| MS-13 | Trả `DeploymentResult`. |

## Profile realization

- `aws-eks`: `ImplicitResourceEnricher` thêm Environment/generation-scoped VPC/EKS only; UC-08 dùng Terraform cho VPC/EKS/ElastiCache/Aurora và Kubernetes modules
  namespace/Redis/PostgreSQL; target lấy từ EKS outputs.
- `internal-k8s`: enricher thêm existing cluster provider; UC-08 resolve Connection bằng ExistingClusterAdapter; Terraform quản lý
  namespace/Redis/PostgreSQL qua Kubernetes provider trên target đó.

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
T22 removes host-context targets: context-only/unscoped execution fails closed.
Internal Kubernetes requires scoped KUBECONFIG; EKS credential được tái tạo theo pinned opaque identity cho mỗi invocation,
không persist đường dẫn kubeconfig. Kết quả code thuộc CURRENT_STATE.

## Definition-selected workload rendering collaboration

See [ADR-010](../../architecture/decisions/ADR-010-score-k8s-workload-rendering.md)
and [rendering contract](../../architecture/contracts/workload-rendering.md).
Catalog kiểm required score-k8s bundle cho cả profiles; planner pin
Definition/bundle/module/provider identity. Deploy preflight kiểm known inputs
và digests trước provision; không có renderer thay thế khi thiếu/lỗi bundle.
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

## T25 — Public route input

Score parsing and public Deployment Set projection reject `service.publicPort`;
only `publicRoutes` reaches Candidate validation and rendering. Multi-path,
same-host ownership, declared-port validation and route-only retry remain intact.
No alias conversion or default route is introduced.


## V03 collaboration replacement

[ADR-016](../../architecture/decisions/ADR-016-definition-selected-workload-targets.md) replaces internal profile enrichment/system binding in the collaboration above. Catalog validates public existing-cluster/Internal account and workload typed namespace/cluster refs plus provision rules. Planning matches workload first and expands all nodes to fixed point; same normal matcher selects cluster Definition. Target resolver builds/pins per-consumer placement, rejects mismatch/ambiguous/cycle/multi-cluster V03. Shared snapshot/admission compare pins/versions; UC-08 resolves cluster then owned namespace/resources. UC-06 renders/applies/waits on selected workload target; no global target or builtin persistence. Applied target restored for Update/Remove/restart; separate cleanup preserves preexisting cluster. AWS legacy collaboration remains until V05–V06.


## V03 provision flags and execution stages

UC-06 BR-08 applies without changing flag meanings when the parent is a workload
Definition. An empty/false rule alone creates no ordering edge; is_dependent
adds child→parent, match_dependents adds every parent consumer→child. References
add their own edges. Validate cycles over the full graph including workloads
and self-edges before execution. Resource-only UC-08 scheduling must never erase
a dependency merely because its provider is a workload.

UC-06 owns the combined lifecycle order and dispatches selected workload render/
apply/readiness at the required graph position; UC-08 executes only resource
subsets whose providers have completed. A resource that depends on a workload
executes after its acknowledged lifecycle/output stage, before final success
commit. Typed safe workload metadata outputs, when declared/supported, are
validated before their consumers. No new public endpoint or all-purpose external
driver protocol is implied. Existing owner/fence, persisted progress, restart
and current-set-after-complete guarantees remain across all stages.


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

UC-06 REV-* uses [ADR-019](../../architecture/decisions/ADR-019-visual-deployment-review.md)
and the [review contract](../../architecture/contracts/deployment-review.md).
Admission preserves server reviewRequestId and safe desired review in the immutable
plan; resource execution persists stage/attempt/readiness facts. Workload owner
captures redacted rendered manifest before Apply and associates applied baseline
only after that workload is READY. All writes use existing owner/fence and short
transactions; no network/renderer subprocess spans a DB transaction. UC-09 reads
these records; no simulation timer, polling-triggered mutation or new scheduler.

## Git render pipeline — ADR-021

RenderPreflight verifies local cache/content/runtime/inputs before UC-08.
Existing provisioning and OutputBindingResolver run unchanged. Separate private
secret bridge from native policy rendering; the bridge prepares approved Secret
references/auxiliaries. GitRenderEngine consumes neutral JSON input, runs defaults/
normalization, pinned score-k8s output-only provisioners, then ordered patches
and semantic validation. It never builds a hidden Go policy Deployment/Service.
Existing workload progress, manifest digest, Apply/WaitReady, public routes and
current-set transactions consume the validated result unchanged. Installed pins
retain the legacy adapter path. Worker subprocess cancellation and cleanup are
part of render completion, not deferred background work.
