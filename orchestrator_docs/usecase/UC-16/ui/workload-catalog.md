---
id: UC-16-UI-WORKLOAD-CATALOG
artifact: use-case-ui-design
status: draft
last_reviewed: 2026-10-29
---

# Workload catalog UI

[Specification](../workload-catalog-specification.md),
[contract](../../../architecture/contracts/workload-catalog.md),
[mock source](../../../../frontend/src/prototypes/workload-catalog/README.md).

## Entry và form

Giữ ApplicationHomePage/AppShell/navigation/tabs. Thêm Workload mở shared Dialog,
không thay trang ứng dụng. Frontend/Backend/Worker/Stateful/Scheduled là mẫu với
runtime label; chọn mở editor, Edit bỏ qua picker và giữ pin hiện tại. Hủy/Escape
trả focus về nút. Save quay về danh sách pending; Preview/Deploy vẫn ở app home.

| Section | Controls |
|---|---|
| Cơ bản | Tên, image, registry credential ref; container selector khi nhiều container |
| Mạng | Service toggle, ClusterIP/headless, ports table, public route tới Service port |
| Biến môi trường | Name/Value, Chọn từ… và secret create dialog hiện hành |
| Tài nguyên | Typed dependency, class/shared identity, inputs theo Type schema, new/reuse |
| Volume và file (trong từng container) | Một nút thêm; PVC, emptyDir, ConfigMap, hostPath hoặc Secret; tạo mới/dùng lại nguồn; path/mode; container đích ngầm định |
| Lịch | Cron expression, IANA timezone, suspend, concurrency; chỉ scheduled |
| Nâng cao | command/args riêng, workingDir, probes, requests/limits; Job retry/deadline/history |

Defaults do template; không giả sử /health phù hợp mọi image. Không buộc frontend
có public route hoặc backend có DB; hiển thị theo capability. Cấu hình không được
support phải hiện rõ/giữ advanced, không biến mất. Retain/delete hậu quả ở nơi
quyết định. Không bắt người dùng nhập state/provider/token/UID trong form.

Trong từng container, “Thêm volume hoặc file” mở shared Dialog chọn loại và
nguồn mới/đã có. Với PVC mới, chuyển sang resource Dialog hiện hành; xác nhận
thêm dependency và mount của đúng container đã mở. Secret mới dùng UC-12 Dialog
hiện hành. Không lồng hai Dialog. Hủy/Escape giữ draft và trả focus. Nguồn
emptyDir/ConfigMap/hostPath mới cùng mount được thêm nguyên tử tại xác nhận.

## Trạng thái và detail

Catalog loading skeleton; empty do filter có clear; unavailable có retry, giữ
form; invalid fields inline; schema changed yêu cầu review lại; stale/BUSY theo
shared rules. Save thành công nhưng reload lỗi không báo chưa save hoặc tự replay.

Continuous: desired/applied pin, pending, replicas Ready/rollout, logs.
Scheduled: schedule/timezone/suspend, last scheduled, active Jobs, last outcome,
history/logs; chưa chạy => Chưa chạy, không Ready giả. Deploy schedule thành công
và Job thành công là hai trạng thái khác nhau. Manual-run/cancel phải có contract
trước khi thêm button; không phải control giả trong product.

## Mock gap

Mock hiện chỉ chứng minh entry/picker và form sơ bộ. Chưa có Service editor,
registry picker, full bindings, secret files, probes/limits, CRUD catalog hay
actual persistence. Không dùng mock làm bằng chứng các phần đó đã hoàn tất.
Nguồn visual: AppShell, ApplicationHomePage, WorkloadEditorPage, shared Dialog,
Button và styles. Tiếp tục đối chiếu browser mỗi khi sửa UI; giữ nội dung ngắn,
trợ giúp kỹ thuật nằm trong disclosure. Nhãn VI/EN từ message catalogs.

### Live Job history

An applied scheduled workload has a `Lịch sử Job` action on its existing home
row. Reuse Dialog/table/buttons and existing scoped log navigation. The dialog
shows loading/error/retry, bounded Kubernetes Job history and observation time;
Pending/Running are observations, not rollout readiness. Empty history means no
retained Job, not a successful run. Disclose history-limit/truncation semantics.
Closing restores focus; changing app/environment drops late responses.

## Custom containers and volumes — WC-16..19

Custom and preset editors use one Volumes & files section within each container.
App/init/sidecar share the same controls. A single Add volume or file opens type
(PVC/emptyDir/ConfigMap/hostPath/Secret), source (new/existing), mount path and
mode. Do not show a container selector. Named sources remain workload-wide, so
other containers can reuse them. Inline mount cards expose source settings under
a disclosure, labelled shared when multiple mounts reference them. Removing a
mount does not delete its source, dependency or Secret. Preserve unused imported
sources in the existing-source picker. If a container rename/removal leaves an
unassigned mount, show a compact repair row with target selection and Remove.
The four previous workload-level volume/file sections are retired. Keep shared
Dialog/Button/field-grid patterns, short VI/EN labels and existing catalog error,
CAS/BUSY/scope behavior. Source/type errors remain visible and must not drop intent.

GW-07 uses per-container advanced port rows and probe controls (enable/type,
HTTP path/scheme, TCP port or exec argv, plus optional timing). Service rows expose
protocol; Workload advanced contains annotation name/value rows. Replicas is
optional and supports zero for Deployment; no value is synthesized at Save.

## HostPath — WC-21 (2026-11-06)

Reuse the named volume source selector and mount rows. HostPath reveals node
path and type (default Directory, optional unchecked type and all Kubernetes enum
values). Show one short note that node files are exposed, paths differ by node
and cluster policy can reject them. New mounts default read-only with an editable
checkbox; imported read-write intent remains unchanged. Keep empty/error/save
states, existing shell/controls and translated VI/EN labels. Preview keeps
path/type/mount mode visible. Do not add a node file browser or local file picker.
