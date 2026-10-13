---
id: UC-02-SPEC
artifact: use-case-specification
status: current
last_reviewed: 2026-10-13
---

# UC-02 — Register Resource Type

## Mục tiêu

Đăng ký Resource Type để định nghĩa loại resource mà workload có thể yêu cầu, cùng contract inputs và outputs của resource đó.

## Primary actor

- Platform Engineer

## Tiền điều kiện

- **PRE-01:** Organization đã tồn tại.
- **PRE-02:** Platform Engineer có quyền quản lý Resource Type.
- **PRE-03:** Resource Type document tuân theo contract được orchestrator hỗ trợ.

## Trigger

**TRG-01:** Platform Engineer gửi yêu cầu đăng ký một Resource Type mới.

## Main success scenario

1. **MS-01:** Platform Engineer cung cấp Resource Type ID, input schema và output schema.
2. **MS-02:** Orchestrator validate cấu trúc document và tính hợp lệ của các schema.
3. **MS-03:** Orchestrator kiểm tra Resource Type ID chưa tồn tại trong Organization.
4. **MS-04:** Orchestrator lưu Resource Type.
5. **MS-05:** Orchestrator trả về Resource Type đã đăng ký.
6. **MS-06:** Resource Type sẵn sàng được dùng trong Score và Resource Definition.

## Hậu điều kiện

- **POST-01:** Resource Type được lưu với ID duy nhất trong Organization.
- **POST-02:** Input schema có thể dùng để validate `resources.*.params` trong Score.
- **POST-03:** Output schema có thể dùng để validate resource placeholders và executor outputs.
- **POST-04:** Platform Engineer có thể tạo Resource Definition cho Resource Type này trong UC-03.

## Giao diện MVP

- Trang `Platform / Resource types` chỉ hiện cho Platform Engineer và Admin.
- Trang liệt kê Resource Types trong Organization đang đăng nhập và có form tạo mới: ID, danh sách input (name, type, required), danh sách output (name, type, required, secret).
- Submit thất bại phải giữ nguyên nội dung form và hiển thị lỗi. Không có sửa/xóa trong UC-02.

## Quy tắc nghiệp vụ

- **BR-01:** Resource Type ID là duy nhất trong Organization.
- **BR-02:** Resource Type định nghĩa contract độc lập module và provider.
  Redis và PostgreSQL là các loại được hỗ trợ; mỗi loại có cùng output schema
  cho in-cluster và dịch vụ AWS theo [module contract](../../architecture/contracts/terraform-modules.md).
  Type `vpc` giữ outputs hiện hành và thêm `privateSubnetIds` tùy chọn cho
  Aurora/ElastiCache tham chiếu; đây là danh sách subnet IDs không bí mật.
- **BR-03:** Mọi Definition của một Type phải cung cấp đúng output schema;
  output bí mật không được hạ xuống thành output công khai.
- **BR-04:** Score params chỉ chứa inputs trong schema đã đăng ký. Metadata
  công bố kiểu, required/default và constraints; không tự biến mọi Terraform
  variable thành param của Workload. Registration vẫn chỉ tạo mới; không
  chuyển đổi schema/dữ liệu thử nghiệm v1.

## Luồng nội bộ

```text
UC-02 Register Resource Type
├── Parse Resource Type document
├── Validate metadata and schemas
├── Check unique Resource Type ID
├── Persist Resource Type
└── Publish Resource Type for Score and Definition validation
```

## Ngoài phạm vi happy path

- **OOS-01:** Cập nhật hoặc xóa Resource Type.
- **OOS-02:** Versioning và migration schema.
- **OOS-03:** Kiểm tra ảnh hưởng tới Resource Definitions và Active Resources hiện có.
- **OOS-04:** Custom validation ngoài JSON Schema.
- **OOS-05:** RBAC chi tiết và audit history.

## Registration identifier policy (2026-10-02)

- **BR-05:** New public Resource Type IDs must be non-empty and contain only
  ASCII lowercase letters, digits and hyphens (`^[a-z0-9-]+$`). Reject invalid
  IDs; do not trim, lowercase or rename them silently. This policy does not
  change Application IDs, workload IDs, schema field names or connection IDs.
- **BR-06:** `environment` and `service` are reserved virtual Score Resource
  Types and cannot be registered as user catalog entries. Existing seeded
  catalog entries remain protected by insert-only duplicate semantics.
- Existing catalog reads, seed/import fixtures and planning remain compatible:
  this policy is enforced at new public registration, not by rewriting legacy
  records or broadening the planner's identifier restrictions.
