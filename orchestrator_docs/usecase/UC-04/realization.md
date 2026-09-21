---
id: UC-04-REALIZATION
artifact: use-case-realization
status: current
last_reviewed: 2026-09-21
---

# UC-04 — Use Case Realization

## Trách nhiệm

Đăng ký và xác minh AWS Driver Account hoặc internal Kubernetes connection mà không lưu credential thô trong domain/database record.

## System operations

```go
ConnectionService.RegisterAWSDriverAccount(ctx context.Context, cmd RegisterAWSDriverAccountCommand) (*Connection, error)
ConnectionService.RegisterKubernetesCluster(ctx context.Context, cmd RegisterKubernetesClusterCommand) (*Connection, error)
```

## Participants

- `ConnectionController` — boundary chọn operation theo profile.
- `ConnectionService` — control lưu secret, verify và persist connection.
- `Connection` — aggregate với kind, config, secret reference, status.
- `SecretStore` — port lưu credential và trả opaque reference.
- `AWSConnectionVerifier`, `KubernetesConnectionVerifier` — integration ports.
- `ConnectionRepository`, `UnitOfWork` — persistence ports.

## Trace main flow

| Step | Collaboration |
|---|---|
| MS-01–MS-02 | Controller dispatch AWS/Kubernetes command. |
| MS-03 | Service gọi verifier tương ứng với credential chỉ ở memory. |
| MS-04 | `SecretStore.Put` trả secret reference. |
| MS-05–MS-06 | `Connection.MarkReady`, save record không chứa secret value. |
| VAR-01 | AWS verifier kiểm tra principal, region/backend và quyền tối thiểu. |
| VAR-02 | Kubernetes verifier kiểm tra API/RBAC cần cho namespace/workload/resource. |

## Transaction boundary

External verification và secret write xảy ra trước database transaction. Transaction chỉ lưu connection `READY`; secret cleanup khi DB save lỗi thuộc failure handling ngoài happy path.

## Planned tests

- `TestRegisterAWSDriverAccount_StoresOnlySecretReference`.
- `TestRegisterKubernetesCluster_VerifiesRBAC`.
- `TestConnectionRecord_DoesNotContainCredentialMaterial`.
