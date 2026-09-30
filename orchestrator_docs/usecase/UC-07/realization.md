---
id: UC-07-REALIZATION
artifact: use-case-realization
status: current
last_reviewed: 2026-09-30
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
failure cleanup vẫn ngoài scope. Record mới nhất được đọc trong transaction
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
