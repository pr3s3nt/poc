---
id: UC-03-UI-SCREENS
artifact: use-case-ui-screens
status: current
last_reviewed: 2026-10-10
related: UC-03
---

# UC-03 UI screens

Trang Cấu hình tài nguyên liệt kê cấu hình thuộc Organization hiện tại, gồm
ID, loại tài nguyên, phạm vi triển khai, driver và số điều kiện áp dụng. Không
hiển thị cấu hình `workload`; trang Mẫu dựng ứng dụng thuộc T03. Cấu hình
`existing-cluster` nếu có chỉ đọc và mang nhãn “Hệ thống”. Không có sửa/xóa;
đăng ký chỉ dành Kỹ sư nền tảng/Quản trị viên.

## Form đăng ký (T02)

Phần cơ bản gồm ID, Loại tài nguyên, cách tạo và Phạm vi triển khai. Form chỉ
cho Terraform/Kubernetes, không tạo `existing-cluster` hoặc `score-k8s`.
`vpc` tự chọn Terraform/module `vpc`; `k8s-cluster` tự chọn Terraform/module
`eks`; `k8s-namespace` tự chọn Kubernetes; `postgres` cho chọn hai cách tạo,
Terraform dùng module `aurora`. Không quảng bá custom type chưa có executor.

Terraform tự điền `aws-eks` và khóa Phạm vi triển khai; Kubernetes cho chọn
“Cluster nội bộ” (`internal-k8s`) hoặc “Dùng chung” (`""`). Dùng chung được xét
ở mọi profile nhưng vẫn phải thỏa điều kiện áp dụng; cảnh báo có thể trùng mức
ưu tiên với cấu hình khác, không tuyên bố đã phát hiện mọi overlap.

Terraform hiện vẫn yêu cầu Kết nối AWS tường minh (T16 mới đổi VPC/EKS).
Dropdown dùng metadata của GET `/connections`, chỉ nhận READY đúng kind trong
Organization của phiên và gửi key kỹ thuật. Kubernetes mặc định không override,
dùng Kết nối của Môi trường; lựa chọn Kết nối riêng nằm trong phần nâng cao.
Không tự chọn một Connection hoặc tạo Connection trong form này. Trợ giúp
Kubernetes nêu rõ Connection riêng phải trùng Connection của Môi trường khi
matching; lựa chọn này không cho phép retarget sang cluster khác.

Điều kiện áp dụng giữ editor từng dòng với năm field chuẩn; dòng rỗng là
wildcard tường minh. Modes/copy/preview thuộc T02B/T19. Phần nâng cao chứa
Tham số cấu hình và Quy tắc tạo tài nguyên liên quan dưới dạng JSON object;
giữ placeholder/JSON keys và giải thích validation/không nhận credential.
Đổi driver/type cập nhật controls hợp lệ nhưng giữ dữ liệu đã nhập để người
dùng có thể quay lại; collapse không xóa dữ liệu. Chỉ gửi inputs hợp lệ của
cách tạo đang chọn. Không tự xem trước hoặc triển khai.

Nhãn, trợ giúp, aria-label và trạng thái trang dùng tiếng Việt theo glossary;
technical IDs, module, driver, payload và tên người dùng giữ nguyên. Lỗi server
chưa có code mapping đầy đủ dùng thông báo Việt an toàn, không regex dịch raw
message (VI-03 thuộc T20).
