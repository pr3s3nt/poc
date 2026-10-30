---
id: UC-07-REALIZATION
artifact: use-case-realization
status: current
last_reviewed: 2026-10-28
---

# UC-07 — Use Case Realization

## Trách nhiệm

Cập nhật hoặc xóa đúng một workload, giữ nguyên contribution khác, gọi UC-08 cho desired resources và chỉ đánh dấu resource bỏ tham chiếu là `UNREFERENCED`.

## System operations

```go
DeploymentService.UpdateWorkload(ctx context.Context, cmd UpdateWorkloadCommand) (*DeploymentResult, error)
DeploymentService.RemoveWorkload(ctx context.Context, cmd RemoveWorkloadCommand) (*DeploymentResult, error)
```

Hai operation dùng lại `PlanningService.Plan`, `ResourceProvisioningService.Provision` và workload ports của UC-06.

## Participants

- `DeploymentController`, `DeploymentService`.
- Shared planning/provisioning/rendering components.
- `BeforeStateValidator` — deep-compare target module và từng shared entry do `before Score` khai báo.
- `DeltaBuilder` — tạo typed Delta document theo Humanitec shape, thay contribution của đúng workload, từ chối shared ID conflict và chỉ bỏ shared entry khi không còn module khác tham chiếu.
- `ActiveResourceClassifier`.
- `WorkloadDeployer.Apply/Delete/WaitReady`.
- Deployment/DeploymentDeltaSnapshot/DeploymentSet/ActiveResource/WorkloadInstance repositories.

## Trace main flow

| Step | Collaboration |
|---|---|
| MS-01–MS-02 | Load snapshot; `BeforeStateValidator.Validate` so module và shared contribution. |
| MS-03 | `DeltaBuilder.BuildHumanitecDelta` phân loại module add/remove/update, tạo module-relative/shared-relative patches, kiểm tra shared conflict và shared ownership/reference. |
| MS-04 | Rebuild graph và classify existing/new/unreferenced. |
| MS-05 | Include UC-08 cho desired resource batches. |
| MS-06 | Update: Render/Apply/WaitReady; Remove: `WorkloadDeployer.Delete`. |
| MS-07 | `ActiveResource.MarkUnreferenced` và persist; không gọi destroy. |
| MS-08 | Final transaction commit current set và Deployment `SUCCEEDED`. |

## Transaction boundary

Giống UC-06: immutable Delta Snapshot/Candidate Set/plan được persist trước external execution. Shared-resource ownership được tính từ toàn Candidate Deployment Set trước khi mark unreferenced; không xóa shared entry/resource nếu workload khác còn tham chiếu. Shared conflict và before-state mismatch dừng ở pure planning, trước mọi external side effect.

`UNREFERENCED` marking là một phần final transaction cùng optimistic current
pointer và `SUCCEEDED`, sau khi runtime action thành công. Runtime failure,
version conflict hoặc transaction rollback không được commit marker này.
Chỉ sửa resource thuộc đúng Organization và scope của plan, còn giữ nguyên ID,
outputs, executor state và input fingerprint; update last Deployment/version
qua repository. Không chuyển resource còn nằm trong desired graph hoặc thuộc
Environment/Application khác. Đây là metadata update, không executor destroy.
Marking dùng logical identity (Organization + descriptor + scope), không chỉ
descriptor string. Single-Environment deployment chỉ mark `READY` resources
thuộc Environment/shared/workload scope của nó; `APPLICATION` scope có thể
được Environment khác dùng nên không mark từ classification riêng lẻ này.
`FAILED`/`PROVISIONING` không được ép sang `UNREFERENCED`; follow state machine,
failure cleanup có operation riêng theo OC-19, không thuộc marking này. Record mới nhất được đọc trong transaction
trước upsert để không ghi đè executor state/outputs bằng bản snapshot cũ.

Console/API dispatch qua UC-16 pending flow: reconstruct before từ current Set,
pin version/config/draft trong Preview token rồi gọi orchestration chung.
Standalone Score Preview không phải token cho flow này. Shared planning snapshot
loader của I06-08 được dùng cho runtime planning. Xem [mapping](ui/api-mapping.md).

## Planned tests

- `TestUpdateWorkload_PreservesOtherModules`.
- `TestPlan_RejectsBeforeScoreWithStaleSharedResource`.
- `TestPlan_RejectsSharedConflict`.
- `TestPlan_DropsSharedResourceWhenTheLastWorkloadStopsDeclaringIt`.
- `TestPlan_KeepsSharedResourceWhileAnotherWorkloadReferencesIt`.
- `TestPlan_UpdateUsesModuleRelativePatch`.
- `TestPlan_RemoveUsesModulesRemoveAndSharedPatch`.
- `TestRemoveWorkload_PreservesSharedDatabaseAndMarksLastReference`.
- `TestUnreferencedMarking_StaysInOrganizationAndEnvironmentScope`.
- `TestUnreferencedMarking_RollsBackWithFailedRuntimeOrCommit`.
- `TestPostgresRemove_CommitsSetStatusAndMarkerAtomically`.
- `TestRemovingReferencedServiceIsBlockedUnlessConsumerGoesToo`.
- `TestDirectRemovalOfReferencedServiceFailsBeforeSideEffects`.
- `TestUC07PendingDraftHTTPContract`, Application home/editor UI tests.

## UC-04 credential-backed target integration

[Shared credential design](../../architecture/connection-credentials.md) owns
resolution. Carry only Organization/Connection identity through fresh and
reused resource targets and persisted WorkloadInstance TargetRef. Resolve
credential at each adapter call; never persist normalized kubeconfig or temp
paths in outputs/state/plan. Missing/not-READY Connection fails closed. Restore
the same scoped target for removal and route reconciliation after restart.
T22 removes host-context targets: context-only/unscoped execution fails closed.
Internal Kubernetes requires scoped KUBECONFIG; explicit Terraform-generated
EKS kubeconfig paths remain supported. Implementation evidence is tracked in current state.


## Collaboration Xóa tài nguyên (DEL-MS-01..07)

ResourceDetailController/ResourceQueryService đọc scoped resource và eligibility
cho UC-09. ResourceCleanupController kiểm session Organization + PE/Admin;
Developer bị từ chối trước cả lập delete plan hoặc nhận operation.
CleanupPlanner đọc danh sách root/dependency và consumer blockers từ current/
pending graphs + persisted dependency/ownership snapshots; phát token pin
root descriptor, candidate IDs/versions, current/draft versions và target generation.
UI chỉ tick root mặc định, cho người dùng chọn dependency theo DEL-BR-01 và
hiện đúng tập đã tick trong xác nhận. Backend kiểm selected set trong admission;
không mở rộng tập xóa. UI hiện scope, cảnh báo mất dữ liệu và tên cần gõ. Token/typed name không cấp
quyền nếu trạng thái đã đổi sau khi hiển thị hộp thoại.

ResourceCleanupService kiểm snapshot/role/UNREFERENCED/consumers dưới
EnvironmentOperationManager trong một giao dịch ngắn rồi persist REQUESTED.
CHECKING đọc original module/target/state, kiểm evidence; DESTROYING gọi
TerraformExecutor theo reverse graph dưới state locks; VERIFYING dùng provider
reads để ghi absence/remaining IDs. ResourceInstanceRepository và Operation
Repository lưu tiến độ/actor/confirmed scope. State cuối và audit giữ lại.
Tài nguyên đã xác minh hết chuyển DESTROYED; historic Deployment snapshots
không đổi. Consumer failure giữ provider dependency; không tự tăng phạm vi.

Cleanup run dùng capability riêng và CREATED_BY_RUN filter; user delete dùng
PE/Admin và ownership bền vững. Cả hai dùng cùng execution/verification engine;
script safety net không giả chữ ký xác nhận UI. Xem
[sequence delete](sequence-resource-delete.puml), [VOPC](vopc.puml),
[cleanup engine](../UC-08/sequence-cleanup.puml) và OC-19.

Tests: Developer 403, foreign scope 404, stale/busy/not-unreferenced/consumer
rejection trước provider calls, exact typed name, pinned selected set/retained
dependencies, duplicate token idempotency, lock/fencing và safe partial result.
V2-A01 chứng minh xóa thật qua headed UI; các ngoại lệ role/race có tests riêng.

## Visual review execution contract

UC-07 REV-* uses [ADR-019](../../architecture/decisions/ADR-019-visual-deployment-review.md)
and the [review contract](../../architecture/contracts/deployment-review.md).
Admission preserves server reviewRequestId and safe desired review in the immutable
plan; resource execution persists stage/attempt/readiness facts. Workload owner
captures redacted rendered manifest before Apply and associates applied baseline
only after that workload is READY. All writes use existing owner/fence and short
transactions; no network/renderer subprocess spans a DB transaction. UC-09 reads
these records; no simulation timer, polling-triggered mutation or new scheduler.

## Git rendering-only changes — ADR-021

Pending comparison includes immutable render pin and effective inputs; update
uses the existing deployment pipeline and infrastructure reuse. Removal resolves
original applied target/identity, not current renderer or mutable Git content.
Persisted review/pins survive restart; explicit re-selection of an old verified
bundle re-renders with current bindings. No new cleanup participant is introduced.

## ADR-030 collaboration extension

Removal restores applied receipts, withdraws the provider retaining objects, then invokes shared UID-precondition removal. Update retains union inventory until acknowledged prune; failures retain original targets/checkpoints.

Canonical shared collaboration: [delivery sequence](../../architecture/components/delivery-reconciliation-sequence.puml),
[participants](../../architecture/components/delivery-reconciliation-vopc.puml),
[infrastructure sequence](../../architecture/components/infrastructure-drift-sequence.puml).
These supplement the existing direct-path diagrams for this new design slice;
[CDR contract](../../architecture/contracts/delivery-reconciliation.md) owns port/API semantics.
