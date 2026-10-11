---
id: UC-02-UI-SCREENS
artifact: use-case-ui-screens
status: current
last_reviewed: 2026-10-11
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

Nhãn “Có cách tạo hỗ trợ” chỉ dành `vpc` (Terraform/vpc), `k8s-cluster`
(Terraform/eks), `k8s-namespace` (Kubernetes) và `postgres` (Kubernetes hoặc
Terraform/aurora), theo whitelist hiện hành của form Cấu hình tài nguyên.
Các ID khác, kể cả `postgres-ha`, mang nhãn “Chỉ có contract”. `workload` được
hướng dẫn sang Mẫu dựng ứng dụng, không có CTA tạo Cấu hình tài nguyên.
Nhãn hỗ trợ không bảo đảm triển khai thành công: vẫn cần cấu hình/đích hợp lệ.
Metadata authoritative thay whitelist ở T31.

CTA “Tạo Cấu hình tài nguyên” chỉ hiện cho bốn type hỗ trợ và chuyển sang
form UC-03 với type được chọn sẵn; không đăng ký hoặc triển khai tự động.

## Đăng ký và sao chép schema

Form gồm ID, các dòng tham số đầu vào và đầu ra. Types `string`, `number`,
`bool`, `any` giữ giá trị kỹ thuật; flags có nhãn “Bắt buộc”, “Bí mật”. Có thể
thêm/bớt dòng. ID phải theo BR-05/BR-06, không chuẩn hóa âm thầm.

“Sao chép schema” đưa schema vào draft độc lập, giữ nguyên types và flags;
ID mới để trống để người dùng nhập và xác nhận/sửa schema trước đăng ký.
Sửa draft không đổi nguồn. Không có sửa/xóa/version schema đã đăng ký.
ID trùng không ghi đè; submit lỗi giữ form. Sao chép không gửi POST.
