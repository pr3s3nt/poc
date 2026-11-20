---
id: UC-18-UI-API
artifact: use-case-ui-api-mapping
status: current
last_reviewed: 2026-10-27
related: UC-18, ADR-025
---

# UC-18 UI API mapping

Paths use `/api/v1/applications/{applicationId}` and UC-00 session scope.
Missing session `401`, foreign/missing/`DELETED` Application `404`, invalid body
`400`, stale token/version or a running deletion `409`. A blocked preview is a
`200` with `blocked: true`, per-Environment blockers and no token; a confirmation
that meets new blockers is `409 APPLICATION_DELETION_BLOCKED` with blocker codes.

| UI operation | Method/path suffix | Request / result |
|---|---|---|
| Load preview | `POST /deletion-preview` | `{}` → `{token, environments:[{key, workloads, resources, blockers}], kept, warnings, confirmationName}` |
| Confirm / Retry | `POST /deletions` | `{token, confirmationName}` → `202 {operationId, status}`; same token repeats return the same operation |
| Follow progress | `GET /deletions/{operationId}` | Per Environment stage/status, per-instance results, remaining IDs, safe error codes |

While the Application is `DELETING`, every existing mutation endpoint under
the Application returns `409` with code `APPLICATION_DELETING`. Reads continue.
Responses never contain secret values, Terraform state or credentials.

Force uses the same preview and confirmation routes with optional `force: true`
in both request bodies. A Force token is valid only for a Force confirmation.
Omitting the field preserves the original API.

## Xóa app khỏi DB

| Thao tác | Method/path | Kết quả |
|---|---|---|
| Xác định quyền hiển thị | `GET /api/v1/applications` | `databasePurgeEnabled: true` cho Admin hoặc Platform Engineer khi owner đã bật tính năng |
| Xem trước | `GET /database-purge` | `{preview}` gồm app UUID, tên, trạng thái writer và các định danh hạ tầng đã ghi nhận |
| Xác nhận | `POST /database-purge` | `{applicationId, confirmationName, priorExecutionStopped}` → `204` sau khi transaction commit |

API xác định Organization từ phiên đăng nhập. App đang DELETING vẫn có thể
mở thao tác này. API trả `409` khi còn thao tác đang chạy hoặc chưa xác nhận
executor bị gián đoạn đã dừng. UUID và tên được kiểm tra lại khi xác nhận.
