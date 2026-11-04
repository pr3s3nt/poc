# Bootstrap và acceptance — issue #30

Đầu vào mới dùng schemaVersion 2, tách dev/staging/production. Mọi giá trị runtime
đến từ config; credential từ file riêng. Legacy catalog-only CLI vẫn được giữ.

1. Validate toàn bộ config, file credential và Workload trước khi mở browser.
2. Đăng nhập, đọc health/account/Types/Git/Catalog/Secret Store/CD; báo prerequisites.
3. Qua Console: dùng lại cấu hình khớp, tạo phần thiếu; khác thì BLOCKED.
4. Definitions/modules/rendering templates được lấy từ Git Catalog, pin tag rõ.
5. Kiểm tra Vault Kubernetes auth; cấu hình CD binding và default trước tạo app.
6. Tìm app bằng key đã ghi nhận hoặc name + subdomain ổn định; xác minh identity.
7. Chọn store/release, bổ sung biến/secret/workload. Không ghi đè pending khác.
8. Theo dõi operation đang chạy; Preview/Deploy chỉ khi có thay đổi hợp lệ.
9. Kiểm tra app/job thật qua URL Tailscale; giữ app, dữ liệu và access service.
10. Upgrade bật mặc định, chỉ đến phiên bản cấu hình, không latest; chạy lại no-op.

Mỗi mutation xác nhận qua read-back. Timeout không tự replay; lần chạy sau đọc
server. Local report không thay thế trạng thái server. Không rotate secret hoặc
nâng/downgrade về baseline khi đã đạt đích. Git/CD/Vault/cluster capabilities là
prerequisites; script không tự cấp cluster rights hoặc cài đè shared controllers.

Nghiệm thu: fresh, unchanged rerun, partial/resume, conflict, upgrade on/off,
rerun sau upgrade và restart platform. Kịch bản E2E thuộc e2e/; logs/evidence và
credential ngoài Git. Không reset/xóa dữ liệu để tạo platform trống.
