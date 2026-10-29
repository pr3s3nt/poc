# Workload catalog — review mock

Issue: https://github.com/pr3s3nt/poc/issues/24

UI sản phẩm với dữ liệu giả, dùng để duyệt thiết kế; chưa thay đổi canonical
product behavior. Entry riêng, fixture transport chặn toàn bộ API trong bộ nhớ, không provision/deploy.
Refresh reset dữ liệu. Sidebar dùng AppShell thật; navigation/sign-out được
chặn trong mock. Sidebar collapse preference vẫn dùng cơ chế sản phẩm.

## Chạy

Từ `frontend/`:

```sh
npx vite build --config src/prototypes/workload-catalog/vite.config.ts
npx vite preview --config src/prototypes/workload-catalog/vite.config.ts --host 127.0.0.1 --port 5294 --strictPort
```

Mở `http://127.0.0.1:5294/ui/catalog-mock.html`. Xem trong tailnet: thay
loopback bằng IP Tailscale; không dùng Funnel. Build ở
`/tmp/orchestrator-workload-catalog-mock`, ngoài Git.

## Luồng duyệt

- Bắt đầu bằng ApplicationHomePage hiện hành; bấm Thêm Workload mở Dialog chọn loại.
- Tìm/lọc 5 mẫu trong Dialog, chọn mẫu rồi nhập cấu hình. Không bắt backend có database.
- Stateful application: thêm volume mới hoặc storage fixture có sẵn, mount
  path/dung lượng/read-only. Một replica trong mock, retention là đề xuất.
- Scheduled job: lịch, timezone, concurrency, command; dùng chung resources,
  volumes và bảng biến. Chưa validate đầy đủ cron expression.
- Lưu bản nháp mô phỏng, xem tóm tắt, quay lại sửa.
- Ví dụ Passbolt: MySQL, storage và 3 workload độc lập. Nút cấu hình backup
  mở form với MySQL/storage reuse; không gắn CronJob bắt buộc vào MySQL.

## Ranh giới

Template/field là fixture để duyệt; schema-driven catalog, version pinning,
API, Preview/Deploy, CronJob history/logs và quản trị template chưa implement.
Biến chỉ minh họa input/reference, chưa tái hiện đầy đủ source picker/secret
editor sản phẩm. Không nhập secret thật. Image/path/version, CPU/memory/probe
và backup destination chưa phải cấu hình nghiệm thu. Chưa quyết định
StorageClass, retention đầy đủ, secret file và backup backend.
Không migration/adopt/dọn hệ thống cũ; chưa có application template tổng hợp.

## Nguồn và review

Đọc shared UI design, UC-16 context/specification/UI và Git render bundle
contract. Đã mở ApplicationsPage sản phẩm bằng Playwright với API fixture,
xem screenshot trước khi thiết kế. Import AppShell, Button và styles/index.css;
dùng page header, tabs, panels, field-grid và bảng sản phẩm.
Labels riêng trong vi.json, không thay navigation hoặc product route.

Đối chiếu browser ở 1440px: giữ shell, typography và spacing; thay đổi ở nội
dung tạo workload. Cấu hình nâng cao đóng mặc định, resource/volume chỉ hiện
khi thêm. Banner nêu dữ liệu giả; secret hint tại vùng nhập. Đã kiểm tra
StatefulSet volume draft, CronJob draft và ví dụ Passbolt trên browser.
Screenshot/logs ở `/tmp/catalog-ui-evidence`, ngoài Git.

## Validation

- Typecheck, full frontend lint, product build và isolated mock build: pass.
- Focused AppShell + WorkloadEditorPage: 29/29 pass với một worker.
- Browser trên Tailscale: filter/search/empty state, optional resource add/remove,
  StatefulSet storage draft, CronJob draft, backup reuse; không có API request
  hoặc JavaScript error. Đã xem screenshot đối chiếu với Console.
- Full suite mặc định lần đầu: 869 pass, 50 fail, 3 worker errors, nhiều timeout;
  chạy lại với 2 workers: 77 files / 919 tests pass, không hạ assertions hoặc tăng timeout.
- Không chạy backend/Kubernetes/cloud: task chỉ tạo mock UI và issue.
- Working tree ban đầu sạch; không sửa user-owned change.

Browser bổ sung: invalid mount validation, sidebar collapse giữ draft và edit
roundtrip pass. Preview phục vụ trong tmux `catalog_mock_24`, port 5294.

## Điều chỉnh theo phản hồi người dùng — 2026-10-28

Giữ trang ứng dụng hiện tại, không đưa catalog thành trang đầu mới. Import trực
tiếp ApplicationHomePage với fixture transport (mọi API kể cả mutation được
chặn trong bộ nhớ). Thêm Workload mở shared Dialog: focus trap, Escape/Hủy và
focus return. Chọn loại đóng Dialog và mở form. Quay lại về trang ứng dụng.
Các nút ngoài luồng duyệt trên trang gốc bị chặn; tabs môi trường vẫn hoạt động.
Đã mở trang sản phẩm ApplicationHomePage bằng browser trước khi sửa, đối chiếu
screenshot mock với reference. Không thêm hướng dẫn dài hoặc navigation mới.

Validation điều chỉnh: TypeScript, lint prototype, mock build, docs checker và
whitespace pass. Focused AppShell/ApplicationHomePage: 26/26 pass. Browser
home → Dialog → CronJob/Stateful form, Escape/Hủy focus return pass; không có
network API request hoặc JS error. Đã xem screenshot Dialog trên trang gốc.
Không rerun full suite vì thay đổi chỉ ở isolated prototype; lượt trước 919/919
pass được giữ làm baseline. Git identity thiếu nên chưa commit/push.
