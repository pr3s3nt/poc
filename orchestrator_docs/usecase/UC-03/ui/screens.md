---
id: UC-03-UI-SCREENS
artifact: use-case-ui-screens
status: current
last_reviewed: 2026-09-29
related: UC-03
---

# UC-03 UI screens

Trang `Platform / Resource definitions` liệt kê Definition, Resource Type,
profile, driver và số criteria trong Organization. Form đăng ký có ID,
Resource Type từ catalog, execution profile, driver, connection key,
Terraform module (chỉ module nhúng), ít nhất một criterion với năm field chuẩn.
Driver variables và provision rules được nhập ở phần JSON nâng cao vì có thể
chứa resource/context placeholders; form hiển thị rõ định dạng và lỗi parse.

Không có sửa/xóa Definition; nút đăng ký chỉ dành Platform Engineer/Admin.
