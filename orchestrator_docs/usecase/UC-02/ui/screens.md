---
id: UC-02-UI-SCREENS
artifact: use-case-ui-screens
status: current
last_reviewed: 2026-10-12
related: UC-02
---

# UC-02 UI screens

Trang **Loại tài nguyên** thuộc Organization hiện tại, chỉ dành Kỹ sư nền
tảng/Quản trị viên; API kiểm quyền độc lập. Mọi nhãn, trợ giúp, aria-label và
trạng thái dùng tiếng Việt; ID, tên field, types và payload giữ nguyên.

## Catalog và chi tiết (T04)

Danh sách hiện ID, số input/output và trạng thái hỗ trợ. Có thể xem toàn bộ
schema từ GET: tên, type, required của input; tên, type, required, secret của
output. Không suy cờ required/secret từ tên field.

Nhãn “Có cách tạo hỗ trợ” và CTA lấy cặp resourceType/driverType/module từ
GET `/api/v1/resource-definition-capabilities` (UC-03 BR-23/OC-04d), kết hợp
Resource Type catalog của session; không giữ whitelist frontend. Custom type
không có cặp mang nhãn “Chỉ có contract”; workload hướng dẫn Mẫu dựng ứng dụng.
Loading/error metadata không được diễn giải là contract-only hoặc hỗ trợ, có
Thử lại và không CTA trước khi xác nhận. Metadata không bảo đảm deploy thành công.
CTA mở form UC-03 chọn sẵn type, không tự đăng ký/triển khai.

## Đăng ký và sao chép schema

Form gồm ID, các dòng tham số đầu vào và đầu ra. Types `string`, `number`,
`bool`, `any` giữ giá trị kỹ thuật; flags có nhãn “Bắt buộc”, “Bí mật”. Có thể
thêm/bớt dòng. ID phải theo BR-05/BR-06, không chuẩn hóa âm thầm.

“Sao chép schema” đưa schema vào draft độc lập, giữ nguyên types và flags;
ID mới để trống để người dùng nhập và xác nhận/sửa schema trước đăng ký.
Sửa draft không đổi nguồn. Không có sửa/xóa/version schema đã đăng ký.
ID trùng không ghi đè; submit lỗi giữ form. Sao chép không gửi POST.
