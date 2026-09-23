---
id: UC-01-REALIZATION
artifact: use-case-realization
status: current
last_reviewed: 2026-09-23
---

# UC-01 — Use Case Realization

## Trách nhiệm

Tạo Application self-service từ Name/Subdomain, resolve default execution target
của Organization và atomically tạo `staging`/`production` cùng Deployment Set
rỗng, namespace identity và desired endpoint. Không provision infrastructure.

## System operations

```go
ApplicationService.CreateApplication(ctx context.Context, cmd CreateApplicationCommand) (*Application, error)
```

`CreateApplicationCommand` chỉ chứa Organization ID, Name và Subdomain.
`DefaultExecutionTargetResolver` trả về profile, connection và optional AWS
region từ platform configuration của Organization. ID và hai Environment được
hệ thống tạo, không nằm trong command.

## Participants

- `ApplicationController` — boundary nhận create command của Developer.
- `ApplicationService` — control validate Name/Subdomain, resolve target và điều phối transaction.
- `DefaultExecutionTargetResolver` — đọc platform configuration cho Organization.
- `Application`, `Environment`, `DeploymentSet`, `NamespaceIdentity` — domain entities/value objects.
- `ConnectionRepository` — đọc connection `READY` của target đã resolve.
- `ApplicationRepository`, `EnvironmentRepository`, `DeploymentSetRepository` — persistence ports.
- `UnitOfWork` — transaction cho từng aggregate creation.

## Trace main flow

| Step | Collaboration |
|---|---|
| MS-01–MS-02 | Controller -> `CreateApplication` -> validate Name/Subdomain and uniqueness. |
| MS-03–MS-04 | Service -> resolve Organization default target -> load ready connection -> `Application.Create` with generated ID. |
| MS-05–MS-06 | Service -> `Environment.Create(staging/production)` + `DeploymentSet.Empty` + `NamespaceIdentity.ForEnvironment`. |
| MS-07–MS-08 | Service derives desired endpoints, then saves Application, both Environments and current-set pointers atomically. |
| VAR-01 | Resolve AWS connection/region; runtime status `PENDING`, không provision VPC/EKS. |
| VAR-02 | Resolve Kubernetes connection `READY`; bind cluster ID. |

## Transaction boundary

Một transaction tạo Application, hai Environment và hai Deployment Set rỗng
atomically; không có external infrastructure call. Unique Name/Subdomain là
duplicate guard cuối cùng của persistence.

## Planned tests

- `TestCreateApplication_UsesOrganizationDefaultTarget`.
- `TestCreateApplication_CreatesStagingAndProductionWithEmptyDeploymentSets`.
- `TestCreateApplication_RejectsInvalidOrDuplicateSubdomain`.
- Repository integration test cho unique `(organization_id, name)`, global
  Subdomain và exactly-two default Environment rows.
