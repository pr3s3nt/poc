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
| Storage | New/reuse managed resource, container, mount path/readOnly, size/class/access mode theo capability |
| File bí mật | Chọn approved key, container/path/mode; không hiển thị bytes |
| Lịch | Cron expression, IANA timezone, suspend, concurrency; chỉ scheduled |
| Nâng cao | command/args riêng, workingDir, probes, requests/limits; Job retry/deadline/history |

Defaults do template; không giả sử /health phù hợp mọi image. Không buộc frontend
có public route hoặc backend có DB; hiển thị theo capability. Cấu hình không được
support phải hiện rõ/giữ advanced, không biến mất. Retain/delete hậu quả ở nơi
quyết định. Không bắt người dùng nhập state/provider/token/UID trong form.

Thêm volume mount vẫn bấm được khi chưa có dependency nếu catalog có loại
storage hợp lệ. Mở shared resource Dialog, chỉ hiện loại có outputs string công
khai `claimName`/`namespace` và reuse phù hợp; một loại duy nhất được chọn sẵn,
alias mới không trùng. Thêm và chọn đưa dependency cùng mount `/data` của
container đầu tiên vào local form; người dùng sửa path/container trước Save.
Hủy/Escape không thay đổi form và trả focus về nút. Khi đã có dependency storage,
nút thêm dòng mount như hiện hành. Nếu không có loại storage hợp lệ thì khóa nút
kèm thông báo ngắn yêu cầu quản trị nền tảng đăng ký loại lưu trữ.

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
