---
id: UC-09-REALIZATION
artifact: use-case-realization
status: current
last_reviewed: 2026-09-30
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
- `DeploymentRepository`, `DeploymentSetRepository`,
  `DeploymentResourceRepository`, `DeploymentWorkloadRepository` and Resource
  Type read ports.

## Trace main flow

| Step | Collaboration |
|---|---|
| MS-01 | Authenticated controller tạo query với Organization, Application, Environment và Deployment IDs. |
| MS-02–MS-03 | Service load Deployment, set snapshot, resource/workload status. |
| MS-04–MS-05 | Assembler thêm status metadata và persisted graph/matches/batches nếu có; `FAILED` khi planning chưa tạo Snapshot chỉ trả status/failure reason, không dựng Delta/plan giả. |
| MS-06 | `OutputRedactor.Redact` theo Resource Type output metadata; resolved resource inputs không thuộc view. |
| MS-07 | Trả immutable `DeploymentView`; không gọi runtime/executor. |

## Transaction boundary

Read-only consistent query. Persistence read-snapshot boundary uses PostgreSQL
`REPEATABLE READ READ ONLY` or a private committed-state copy for memory/JSON,
so assembly cannot mix commits or persist a mutation. Resource output visibility
fails closed when contract metadata is missing or does not classify an output
as non-secret; nested secret references are redacted as well.

## Planned tests

- `TestGetDeployment_ReturnsPersistedPlanAndStatuses`.
- `TestGetDeployment_RedactsSecretOutputs`.
- `TestGetDeployment_DoesNotCallRuntimeAdapters`.
- Organization/Application/Environment scoping, status-filter and
  per-Deployment workload-snapshot tests.
- Web Console list/detail: Environment filter, navigation, status/failure,
  read-only resource/workload rows and secret-safe output rendering.
