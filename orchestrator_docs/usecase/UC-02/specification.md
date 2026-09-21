---
id: UC-02-SPEC
artifact: use-case-specification
status: current
last_reviewed: 2026-09-21
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

## Quy tắc nghiệp vụ

- **BR-01:** Resource Type ID là duy nhất trong Organization.
- **BR-02:** Resource Type mô tả contract độc lập với cloud/internal implementation.
- **BR-03:** Mọi Resource Definition của cùng Resource Type phải cung cấp outputs tương thích output schema.
- **BR-04:** Score params chỉ được chứa inputs được schema cho phép trong subset MVP.

## Luồng nội bộ

```text
UC-02 Register Resource Type
├── Parse Resource Type document
├── Validate metadata and schemas
├── Check unique Resource Type ID
├── Persist Resource Type
└── Publish Resource Type for Score and Definition validation
```

## Trạng thái implementation hiện tại

- Seed catalog đã có Resource Types cho workload, VPC, cluster, namespace và PostgreSQL; planner dùng input contract để kiểm tra Score `params` và output contract để kiểm tra placeholder/executor output.
- API, UI, PostgreSQL persistence và nghiệp vụ `RegisterResourceType` chưa có; catalog hiện được seed khi process khởi động.

## Ngoài phạm vi happy path

- **OOS-01:** Cập nhật hoặc xóa Resource Type.
- **OOS-02:** Versioning và migration schema.
- **OOS-03:** Kiểm tra ảnh hưởng tới Resource Definitions và Active Resources hiện có.
- **OOS-04:** Custom validation ngoài JSON Schema.
- **OOS-05:** RBAC chi tiết và audit history.
