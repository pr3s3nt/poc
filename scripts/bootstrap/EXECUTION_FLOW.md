# Bootstrap và acceptance

## Catalog schemaVersion 2 — issue #30

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

## Definitions trực tiếp — issue #33

SchemaVersion 3 chạy theo thứ tự sau; schemaVersion 2 ở trên là luồng Catalog.

1. Đọc input môi trường, validate files/credentials, image pins, Definition JSON,
   criteria, Terraform pins và template-source pin. Không lấy latest.
2. Đăng nhập Console; preflight kind/Fleet/VSO, accounts/Types/store/CD,
   schema-8 Generic Workload capabilities, sources và existing Definition conflicts.
3. Dùng lại/tạo Git access, Kubernetes account và Resource Types qua Console.
4. Dùng lại immutable template source; nếu thiếu thì prepare Git/ZIP/Inline qua
   Console và đối chiếu pin. Tạo Definitions trực tiếp từ file, xác nhận form
   review rồi read-back; không tạo revision khi đã khớp.
5. Cấu hình/reuse Secret Store, Verify CD và default theo input. Mode `bootstrap`
   dừng ở đây với BOOTSTRAPPED, không khẳng định acceptance app đã đạt.
6. Mode `all` tạo/dùng lại acceptance app theo identity receipt; phải không có
   Catalog. Không sửa hoặc bỏ ghim Catalog của app có sẵn.
7. Chọn Secret Store, bổ sung config/secret/workloads còn thiếu; Preview/Deploy
   chỉ khi có thay đổi. Unknown/interrupted operation không tự replay.
8. Read-back Generic Workload và template pins; xác minh Fleet GitRepo/Bundle
   đúng commit, readiness; mở app qua URL Tailscale và kiểm tra app/jobs/DB/cache.
9. Lưu safe identities/pins/report ngoài Git; chỉ SUCCEEDED khi acceptance đạt.
   Rerun giữ nguyên revisions, deployments, Fleet commits và resource/PVC/job IDs.

Input dev/staging độc lập theo Console/cluster/criteria/state. Có mẫu staging
không cấp quyền chạy staging; runtime nghiệm thu issue #33 giới hạn kind.

## Git-only follow-up and backend PVC

Before browser mutation, `gitOnly` resolves Types/Definition/Workload/rules files
from exact HTTPS Git commits and verifies file digests into a private external
cache. Definition/module/template pins are independent: documents carry complete
resource configuration, while product inspection verifies Git module/template
content and provider contracts. No Catalog release or installed fallback is used.
The six-Definition profile adds persistent-storage; backend mounts it at `/data`.
Final acceptance verifies Bound PVC, file value/hash and receipt reuse. The E2E
Git/PVC scenario additionally verifies namespace/controller ownership and exact
Pod UID before recreation, proves retained PVC/file and runs bootstrap again.
