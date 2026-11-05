---
id: UC-16-WORKLOAD-CATALOG-SPEC
artifact: use-case-specification
status: draft
last_reviewed: 2026-10-30
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
   Khi chưa có dependency lưu trữ, Thêm volume mount mở chọn/tạo resource lưu
   trữ tại chỗ; xác nhận thêm dependency và mount vào form cùng lúc. Hủy không
   thay đổi form. Nếu catalog chưa có loại storage hợp lệ, nêu rõ lý do thay vì
   chỉ khóa nút không hướng dẫn.
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
  Với bundle standard của platform, Deployment có persistent volume mount dùng
  `Recreate` khi cập nhật: dừng Pod cũ trước khi tạo Pod mới, chấp nhận gián đoạn
  để tránh rollout chờ volume RWO đang gắn ở node khác. Quy tắc dựa trên mount,
  không dựa tên template/ứng dụng hay nhà cung cấp storage. Deployment không có
  persistent mount và workload chỉ gắn secret file giữ policy hiện tại.
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

## Approved custom workload expansion — 2026-11-05

- WC-16: The custom form exposes PVC, emptyDir, hostPath, nonsecret ConfigMap files and
  scoped Secret-file mounts without requiring a catalog preset. Preset editing
  keeps its pin. Custom execution follows GW-06 and still uses Definition matching.
- WC-17: Every container has App/Init/Sidecar selection. Init and sidecar share
  explicit ordering. Preserve env/resources/command and mounts across role changes;
  deleting/renaming a referenced container must be corrected before Save, never
  silently retarget a mount. Reject regular init probes and an all-init workload.
- WC-18: Developers define named temporary/config volumes and mount them in any
  container. ConfigMap content is labelled nonsecret; Secret files select UC-12
  keys only, with path/read mode and no readback. Existing file keys absent from
  the catalog remain visible invalid choices until corrected. Catalog failure
  preserves the draft and provides retry, not a fake empty list.
- WC-19: Save/import/edit/applied reconstruction retain lifecycle, ordering,
  source definitions and mounts losslessly. Preview shows those changes and
  rejects a selected source without advertised capabilities before provisioning.
  Existing PVC identity, retention and explicit delete rules remain unchanged.

- WC-20: Complete the reviewed basic form gaps: per-container ports, Service
  protocol, optional zero Deployment replicas, annotations in Workload advanced,
  and HTTP/TCP/exec probes with explicit timing (GW-07). Keep omitted defaults
  omitted and imported explicit values intact. Nonsecret annotation and command
  fields do not add a secret-literal mode.

Secret-file creation reuses UC-12 create-only semantics, optionally reading a
UTF-8 local file up to 64 KiB into the masked create form. Read failure, NUL or
invalid UTF-8 cannot silently alter data. Only the UC-12 request receives bytes;
the workload stores the newly created scoped reference. Cancel makes no write.

- WC-21 (approved 2026-11-06): Named hostPath sources accept an absolute clean
  Linux node path and the Kubernetes type enum (omitted/empty, Directory,
  DirectoryOrCreate, File, FileOrCreate, Socket, CharDevice, BlockDevice). Mounts
  target app/init/sidecar with readOnly and optional clean relative subPath.
  New form sources default to Directory; new hostPath mounts default read-only
  but permit explicit read-write. Existing imported values remain unchanged.
  Explain at source selection that the path belongs to the scheduled node,
  exposes node files and may differ across nodes; cluster policy may reject it.
  No node directory is read, created or removed by Save/Preview or cleanup.
  A source must advertise host-path-volumes-v1 as well as pod-volumes-v1 before
  provisioning. Preview exposes path/type/mount mode; guards reject changed
  declared sources or mount intent. No new node scheduling or privilege control.

- WC-22 (approved 2026-11-06): Each app/init/sidecar container owns one
  Volumes & files section with Add volume or file. The picker offers PVC, emptyDir,
  ConfigMap, hostPath and Secret, creating a named source and its first mount
  together or reusing an existing workload source. The container is implicit.
  Cancel/Escape leaves all desired state unchanged. Source settings are shared;
  editing a source is explicitly labelled and updates its existing users. Removing
  a mount never deletes a shared source, retained PVC dependency or Secret.
  Existing imports/unused sources remain lossless and reusable. Mounts whose
  target container was renamed/deleted remain visibly repairable before Save,
  rather than hidden or silently reassigned. Secret/storage creation targets the
  invoking container and retains existing scope/late-response guards. The global
  Storage, Pod volume sources, Container volume mounts and Secret files sections
  are removed. Existing API/data shape, source pins and Preview/Deploy are retained.
