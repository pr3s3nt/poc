---
id: UC-04-REALIZATION
artifact: use-case-realization
status: current
last_reviewed: 2026-09-30
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

Trong local/kind MVP, `RegisterKubernetesCluster` dùng context đã có trên host:
Kubernetes verifier kiểm tra API/RBAC read-only, `SecretStore.Put` không được
gọi. Connection record giữ cluster ID, context, endpoint xác minh và opaque
`host-kube-context://...` reference; không chứa credential. AWS registration
vẫn theo thiết kế đầy đủ và chưa triển khai ở lát cắt này.

HTTP `GET/POST /api/v1/connections` lọc theo Organization; POST chỉ cho
Platform Engineer/Admin. Trước transaction service validate ID và gọi verifier;
trong transaction kiểm tra Organization/unique key và lưu `READY`. UI contract
nằm trong `UC-04/ui/`.

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

External verification xảy ra trước database transaction. Local/kind host-context
variant không ghi secret. Full credential-store variant sẽ ghi secret trước
transaction và cần cleanup nếu DB save lỗi. Transaction chỉ lưu connection
`READY` sau khi Organization/unique key được kiểm tra. Registration dùng
insert-only `CreateConnection`; duplicate kể cả concurrent request không được
ghi đè config/reference/verification của record đã tồn tại. Seed/upsert
`SaveConnection` không phải public update API.

## Planned tests

- `TestRegisterAWSDriverAccount_StoresOnlySecretReference`.
- `TestRegisterKubernetesCluster_VerifiesRBAC`.
- `TestConnectionRecord_DoesNotContainCredentialMaterial`.
