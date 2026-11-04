# Hướng dẫn AI cho repo tài nguyên

Đọc README.md và bộ setup đi kèm trước khi sửa. Đây là nguồn Git cho Orchestrator,
không phải repo ứng dụng hoặc trình cài đặt cluster.

- Giữ root terraform/, templates/, bootstrap/ để dùng input mặc định.
- Ưu tiên thay tham số input riêng tư; giữ nguyên ${context...}, ${resources...},
  ${templateSources...}. ${bootstrap.NAME} được công cụ bootstrap thay từ input.
- Không đưa secret vào tham số, Git URL hoặc file tracked.
- Chọn full commit đã review; dùng locker từ đúng release Orchestrator và đúng
  HTTPS origin để tính lại pins. Không bỏ kiểm tra hash hoặc tắt TLS.
- Kiểm tra contract Terraform/template trước khi chạy thật. Bộ mã AWS không
  cấp quyền tạo cloud resource; xác định target/quyền đã có trong phiên làm việc.
- Không tự publish, deploy hoặc reset dữ liệu ngoài yêu cầu được cấp quyền.
- Báo rõ kiểm tra offline và nghiệm thu staging thật; không suy kết quả thật từ mẫu.
