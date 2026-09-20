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
- `BeforeStateValidator`, `ActiveResourceClassifier`.
- `WorkloadDeployer.Apply/Delete/WaitReady`.
- Deployment/DeploymentSet/ActiveResource/WorkloadInstance repositories.

## Trace main flow

| Step | Collaboration |
|---|---|
| MS-01–MS-03 | Load snapshot, validate before contribution, build scoped Delta/Candidate. |
| MS-04 | Rebuild graph và classify existing/new/unreferenced. |
| MS-05 | Include UC-08 cho desired resource batches. |
| MS-06 | Update: Render/Apply/WaitReady; Remove: `WorkloadDeployer.Delete`. |
| MS-07 | `ActiveResource.MarkUnreferenced` và persist; không gọi destroy. |
| MS-08 | Final transaction commit current set và Deployment `SUCCEEDED`. |

## Transaction boundary

Giống UC-06. Shared-resource ownership được tính từ toàn Candidate Deployment Set trước khi mark unreferenced; không xóa shared entry/resource nếu workload khác còn tham chiếu.

## Planned tests

- `TestUpdateWorkload_PreservesOtherModules`.
- `TestRemoveWorkload_MarksResourceUnreferencedWithoutDestroy`.
- `TestRemoveWorkload_PreservesSharedDatabaseUsedByWorker`.
