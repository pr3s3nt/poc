# Luồng bootstrap Catalog

1. Đọc preset/config và validate nguồn HTTPS trước khi mở browser.
2. Đăng nhập PE/Admin, đọc Resource Types và đối chiếu schema.
3. Cấu hình/kiểm tra Git access theo quy tắc reuse/rotate.
4. Reuse READY Infrastructure Account hoặc đăng ký qua upload kubeconfig.
5. Tạo Resource Types còn thiếu, giữ nguyên schema đã đăng ký.
6. Mở Catalog; reuse nguồn khớp hoặc đăng ký nguồn mới, bind slot tới account ID.
7. Refresh; yêu cầu READY và ít nhất một release đã xác minh.
8. Người dùng chọn release trong Environment Settings, sau đó Preview/deploy.

Lỗi stale/unknown save dừng để kiểm tra Console; không tự replay mutation.
Bootstrap không tạo application/workload hoặc mutate provider.
