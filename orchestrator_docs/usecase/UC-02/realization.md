---
id: UC-02-REALIZATION
artifact: use-case-realization
status: current
last_reviewed: 2026-09-30
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

Validate ngoài transaction; registration dùng repository insert-only
`CreateResourceType`, không dùng seed/upsert `SaveResourceType`. Unique constraint
`(organization_id, key)` là chốt cuối cùng chống concurrent duplicate trên
PostgreSQL. Hai lệnh đăng ký cùng key chỉ được có một lệnh thành công; lệnh còn
lại trả duplicate và không đổi record đầu tiên. List precheck chỉ giúp thông báo
lỗi, không thay thế constraint/insert semantics. Save/upsert được giữ riêng cho
seed/test, không trở thành public update capability.

## Planned tests

- `TestRegisterResourceType_ValidContract`.
- `TestRegisterResourceType_DuplicateID`.
- `TestRegisterResourceType_InvalidSchema`.
- Repository integration test cho schema JSONB và unique key.

Web Console `Platform / Resource types` gọi `GET`/`POST /api/v1/resource-types`;
POST chỉ cho Platform Engineer/Admin. Contract list và form nằm trong
`UC-02/ui/`.
