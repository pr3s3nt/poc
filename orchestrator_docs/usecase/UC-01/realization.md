---
id: UC-01-REALIZATION
artifact: use-case-realization
status: current
last_reviewed: 2026-09-21
---

# UC-01 — Use Case Realization

## Trách nhiệm

Tạo Application với Execution Profile cố định, bind connection phù hợp và tạo Environment cùng Deployment Set rỗng/namespace identity ổn định.

## System operations

```go
ApplicationService.CreateApplication(ctx context.Context, cmd CreateApplicationCommand) (*Application, error)
ApplicationService.CreateEnvironment(ctx context.Context, cmd CreateEnvironmentCommand) (*Environment, error)
```

`CreateApplicationCommand` chứa Organization ID, Application ID/name, profile, connection ID và optional AWS region. `CreateEnvironmentCommand` chứa Application ID, Environment ID/name và Environment Type.

## Participants

- `ApplicationController` — boundary nhận hai command.
- `ApplicationService` — control kiểm tra profile/connection và điều phối transaction.
- `Application`, `Environment`, `DeploymentSet`, `NamespaceIdentity` — domain entities/value objects.
- `ConnectionRepository` — đọc connection `READY`.
- `ApplicationRepository`, `EnvironmentRepository`, `DeploymentSetRepository` — persistence ports.
- `UnitOfWork` — transaction cho từng aggregate creation.

## Trace main flow

| Step | Collaboration |
|---|---|
| MS-01–MS-04 | Controller -> `CreateApplication` -> load connection -> `Application.Create` -> save trong một transaction. |
| MS-05–MS-08 | Controller -> `CreateEnvironment` -> load Application -> `Environment.Create` + `DeploymentSet.Empty` -> save cùng current-set pointer trong một transaction. |
| VAR-01 | Validate AWS connection/region; runtime status `PENDING`, không provision VPC/EKS. |
| VAR-02 | Validate Kubernetes connection `READY`; bind cluster ID. |

## Transaction boundary

Hai operation là hai transaction độc lập. `CreateEnvironment` khóa/version-check Application, lưu Environment và Deployment Set rỗng atomically; không có external infrastructure call.

## Planned tests

- `TestCreateApplication_AWSEKS`, `TestCreateApplication_InternalK8s`.
- `TestCreateEnvironment_InitializesEmptyDeploymentSetAndNamespaceIdentity`.
- Repository integration test cho unique `(organization_id, application_id)` và `(application_pk, environment_id)`.
