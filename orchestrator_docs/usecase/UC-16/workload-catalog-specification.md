---
id: UC-16-WORKLOAD-CATALOG-SPEC
artifact: use-case-specification
status: draft
last_reviewed: 2026-10-28
---

# Catalog workload — mở rộng specification cho issue #24

Phạm vi người dùng đã yêu cầu: cơ chế tổng quát và UI hiện tại → Thêm Workload →
hộp chọn loại → cấu hình. Đây là specification mở rộng đang review; không thay
behavior hiện hành cho tới khi contracts và implementation được nghiệm thu.
[Shared contract](../../architecture/contracts/workload-catalog.md) sở hữu data shape.

## Main flow

1. WC-01: Developer mở Application/Environment hiện tại, chọn Thêm Workload.
2. WC-02: Dialog liệt kê template hợp lệ theo scope; tìm/lọc; Escape/Hủy giữ trang
   gốc và focus. Không thêm một catalog landing page bắt buộc.
3. WC-03: Chọn template tạo local draft có version/defaults; chưa gọi provisioning.
4. WC-04: Cấu hình image, container, Service/route, bindings, storage, health và
   resources. Scheduled job có lịch/timezone/policy thay cho replica/rollout UI.
5. WC-05: Chọn resource mới hoặc reuse binding hợp lệ; MySQL không tự thêm backup.
   Resource creation ở bước này chỉ là desired dependency, chưa chạy Terraform.
6. WC-06: Dùng Name/Value và source picker hiện hành; secret create riêng qua UC-12;
   không nhập cú pháp nội bộ hoặc secret plaintext trong template/Score.
7. WC-07: Validate/schema/CAS rồi Save draft; quay về ứng dụng có pending change.
8. WC-08: Preview và Deploy qua luồng hiện hành; xem resource graph, manifests và
   tác động dữ liệu trước khi apply. Save không tự deploy.

## Rules/acceptance

- WC-09: Template được chọn là pin; mở Edit không tự áp default version mới.
- WC-10: Round-trip giữ tất cả fields kể cả advanced/imported; mismatch dùng
  advanced view hoặc báo lỗi, không drop field.
- WC-11: Stateful mount identity giữ qua restart/update/redeploy. Remove giữ dữ
  liệu, Environment delete bị chặn khi còn retained storage; resource delete riêng.
- WC-12: CronJob là workload độc lập; MySQL có thể chạy không backup hoặc cleanup.
- WC-13: Bật/tắt Service độc lập category; public route cần Service port hợp lệ;
  worker/job không bị ép public endpoint.
- WC-14: UI/loading/empty/catalog failure/validation/BUSY/stale/permission có trạng
  thái rõ; không fake empty khi API lỗi; đổi scope bỏ late response và cảnh báo draft.
- WC-15: Không dùng dữ liệu thật/migration/adoption namespace cũ cho acceptance Passbolt.

## Cross-use-case ownership (requirements đề xuất)

| Owner | IDs | Behavior cần bổ sung |
|---|---|---|
| UC-03 | WC-CAT-01..03 | Publish/retire immutable template/schema; role scope; compatibility renderer; không hardcode tên mẫu |
| UC-05 | WC-PRE-01..03 | Pin/validate template, fields/mounts; Preview safe manifests/data effects; stale admission |
| UC-06 | WC-DEP-01..04 | Resource-first + secret prerequisites; nested CronJob Pod; runtime-specific readiness; truthful scheduled status |
| UC-07 | WC-LIFE-01..03 | Retain data, explicit delete; last-reference tracking; CronJob active-job removal policy |
| UC-08 | WC-RES-01..03 | Terraform storage/MySQL extensibility, stable identity; delayed PVC binding; outputs/reference only |
| UC-09 | WC-OBS-01..02 | CronJob Job history/logs by ownership; deployment status distinct from last Job result |
| UC-12 | WC-SEC-01..03 | Resource/job secret consumers; file projection; revision retention until last live consumer |

Các IDs trên là planned coverage, không khẳng định đã implement. Owner specs
được link tới extension này để review; không duplicate normative rules.
