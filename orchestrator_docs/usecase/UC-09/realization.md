---
id: UC-09-REALIZATION
artifact: use-case-realization
status: current
last_reviewed: 2026-09-21
---

# UC-09 — Use Case Realization

## Trách nhiệm

Tạo read-only deployment view từ persisted plan/status/resource/workload snapshots và loại secret outputs.

## System operation

```go
DeploymentQueryService.GetDeployment(ctx context.Context, query GetDeploymentQuery) (*DeploymentView, error)
```

## Participants

- `DeploymentQueryController`, `DeploymentQueryService`.
- `DeploymentViewAssembler`, `OutputRedactor`.
- `DeploymentRepository`, `DeploymentSetRepository`, `DeploymentResourceRepository`, `ActiveResourceRepository`, `WorkloadInstanceRepository` read ports.

## Trace main flow

| Step | Collaboration |
|---|---|
| MS-01 | Controller tạo query với Application/Environment/Deployment IDs. |
| MS-02–MS-03 | Service load Deployment, set snapshot, resource/workload status. |
| MS-04–MS-05 | Assembler thêm status metadata và persisted graph/matches/batches. |
| MS-06 | `OutputRedactor.Redact` theo Resource Type output metadata. |
| MS-07 | Trả immutable `DeploymentView`; không gọi runtime/executor. |

## Transaction boundary

Read-only consistent query. Implementation có thể dùng một database transaction `REPEATABLE READ` hoặc một read model query để tránh trộn snapshots.

## Planned tests

- `TestGetDeployment_ReturnsPersistedPlanAndStatuses`.
- `TestGetDeployment_RedactsSecretOutputs`.
- `TestGetDeployment_DoesNotCallRuntimeAdapters`.
- Web Console list/detail: Environment filter, navigation, status/failure,
  read-only resource/workload rows and secret-safe output rendering.
