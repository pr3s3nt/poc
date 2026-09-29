---
id: UC-02-REALIZATION
artifact: use-case-realization
status: current
last_reviewed: 2026-09-21
---

# UC-02 — Use Case Realization

## Trách nhiệm

Đăng ký Resource Type như một contract inputs/outputs độc lập với implementation và công bố contract cho Score/Definition validation.

## System operation

```go
ResourceTypeService.RegisterResourceType(ctx context.Context, cmd RegisterResourceTypeCommand) (*ResourceType, error)
```

## Participants

- `ResourceTypeController` — HTTP boundary kiểm tra session/role và parse document.
- `ResourceTypeService` — control validate và persist.
- `ResourceType` — aggregate chứa ID, input schema, output schema.
- `ResourceType.Validate` — kiểm tra schema subset được hỗ trợ.
- `ResourceTypeRepository`, `UnitOfWork` — persistence ports.

## Trace main flow

| Step | Collaboration |
|---|---|
| MS-01 | Controller tạo `RegisterResourceTypeCommand`. |
| MS-02 | Service gọi `ResourceType.Validate`. |
| MS-03 | Repository kiểm tra ID trong Organization. |
| MS-04 | `ResourceType.Register` và repository save trong transaction. |
| MS-05–MS-06 | Trả aggregate đã lưu; repository trở thành nguồn contract cho planner/UC-03. |

## Transaction boundary

Validate ngoài transaction; local/test adapter kiểm tra key trong transaction. Khi
chuyển sang PostgreSQL, unique constraint `(organization_id, key)` là chốt cuối
cùng chống concurrent duplicate.

## Planned tests

- `TestRegisterResourceType_ValidContract`.
- `TestRegisterResourceType_DuplicateID`.
- `TestRegisterResourceType_InvalidSchema`.
- Repository integration test cho schema JSONB và unique key.

Web Console `Platform / Resource types` gọi `GET`/`POST /api/v1/resource-types`;
POST chỉ cho Platform Engineer/Admin. Contract list và form nằm trong
`UC-02/ui/`.
