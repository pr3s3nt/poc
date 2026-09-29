---
id: UC-02-UI-SCREENS
artifact: use-case-ui-screens
status: current
last_reviewed: 2026-09-29
related: UC-02
---

# UC-02 UI screens

`Platform / Resource types` hiển thị catalog thuộc Organization hiện tại.
Phần đầu là danh sách ID và số input/output. Phần `Register resource type`
nhập ID, thêm/bớt các dòng input/output, chọn `string`, `number`, `bool`,
`any`; output có cờ `Secret`. `Register` lưu rồi cập nhật danh sách.
Không có thao tác sửa/xóa type đã đăng ký.

Trang chỉ được điều hướng từ sidebar với vai trò Platform Engineer/Admin;
API vẫn thực thi kiểm soát quyền độc lập với UI.
