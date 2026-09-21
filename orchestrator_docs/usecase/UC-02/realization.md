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

- `ResourceTypeController` — boundary parse command/document.
- `ResourceTypeService` — control validate và persist.
- `ResourceType` — aggregate chứa ID, input schema, output schema.
- `SchemaValidator` — kiểm tra JSON Schema subset được hỗ trợ.
- `ResourceTypeRepository`, `UnitOfWork` — persistence ports.

## Trace main flow

| Step | Collaboration |
|---|---|
| MS-01 | Controller tạo `RegisterResourceTypeCommand`. |
| MS-02 | Service gọi `SchemaValidator.ValidateResourceTypeSchemas`. |
| MS-03 | Repository kiểm tra ID trong Organization. |
| MS-04 | `ResourceType.Register` và repository save trong transaction. |
| MS-05–MS-06 | Trả aggregate đã lưu; repository trở thành nguồn contract cho planner/UC-03. |

## Transaction boundary

Validate ngoài transaction; unique check được bảo vệ lại bằng database unique constraint trong transaction save.

## Planned tests

- `TestRegisterResourceType_ValidContract`.
- `TestRegisterResourceType_DuplicateID`.
- `TestRegisterResourceType_InvalidSchema`.
- Repository integration test cho schema JSONB và unique key.
