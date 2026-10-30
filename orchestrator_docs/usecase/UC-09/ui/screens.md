---
id: UC-09-UI-SCREENS
artifact: use-case-ui-screens
status: current
last_reviewed: 2026-10-28
related: UC-09
---

# UC-09 UI screens

Application home shows recent deployments for the selected staging or
production Environment. Each row has status, action, workload and time; a
`View all` action opens Environment deployment history.

History lists deployments newest first, with a status filter and a link to each
detail. Detail displays persisted Deployment status and failure reason, actor,
timestamps, workload/resource status, graph matches and provision batches.
Secret outputs remain redacted by the backend; the UI does not request cluster
logs or perform runtime refresh. Resource resolved inputs are not part of the
API response. Workload rows come from the selected Deployment snapshot, never
from mutable current Environment state. Each workload row includes its persisted
manifest digest to identify the artifacts recorded by that run.

## Lịch sử triển khai — T14

Trang dùng nhãn “Lịch sử triển khai”, bộ lọc “Trạng thái”, “Tất cả”, nút
“Tải lại” và “Thử lại”. Các trạng thái SUCCEEDED/FAILED/DEPLOYING/PROVISIONING/
PLANNING hiển thị lần lượt là “Thành công”, “Thất bại”, “Đang triển khai”,
“Đang tạo tài nguyên”, “Đang lập kế hoạch”; giá trị query vẫn là enum gốc.
Hành động DEPLOY/UPDATE/REMOVE dùng nhãn triển khai/cập nhật/gỡ bỏ. Thời gian
hiển thị theo locale tiếng Việt; actor thiếu dùng “Không rõ người thực hiện”.
Tên ứng dụng, workload, actor có sẵn, IDs và staging/production giữ nguyên.

Danh sách giữ thứ tự server và scope Application/Environment đang chọn.
Mở một dòng giữ nguyên deployment ID trong route detail. Tải lại chỉ đọc lịch
sử đã lưu, không refresh cluster hoặc provision. Không thêm so sánh deployment;
Việt hóa nội dung trang detail thuộc T15.

## Chi tiết triển khai — T15

Trang có tiêu đề “Chi tiết triển khai”, liên kết về lịch sử của môi trường và
nút “Tải lại” chỉ đọc snapshot đã lưu. Nhãn metadata, trạng thái triển khai/
Workload/tài nguyên, hành động, graph và các đợt tạo tài nguyên dùng tiếng Việt;
thời gian dùng locale vi-VN. Workload/Container/Service, tên người dùng, IDs,
descriptor, output keys, đường dẫn, module và staging/production giữ nguyên.
Scope và danh tính lịch sử lấy từ Deployment đã chọn; không thay bằng đích
hiện tại của Environment hoặc thêm thông tin đích mà API không trả.

Thông báo thất bại dùng hướng dẫn tiếng Việt an toàn, không hiển thị nguyên văn
failure reason hoặc lỗi transport. Output bí mật có nhãn “Đã che”; marker
`***redacted***` không được diễn giải thành giá trị thật. Outputs đã phân loại
non-secret giữ nguyên dữ liệu; resolved inputs không được render. Graph giữ
hướng consumer → provider, matched Definition và thứ tự batches của snapshot.
Không thêm rollback, log hoặc live refresh.

## Tiến trình Terraform

Deployment detail shows resource Terraform stages/attempt/status, module pin,
original target and safe failure reason in Vietnamese. Poll persisted scoped
progress during long provisioning; no provider calls from query. Interrupted
execution says needs operator verification; FAILED does not imply cleanup done.
Cleanup progress distinguishes pending/failed/verified-complete and remaining
safe resource IDs. Never display raw CLI logs/state/plan/tfvars/provider cause.

## Chi tiết tài nguyên trong Environment

Route `/ui/applications/{applicationId}/environments/{environment}/resources/{resourceId}`
được mở từ danh sách/graph tài nguyên của Environment hoặc history resource link.
Hiển thị descriptor làm “Tên tài nguyên”, Type/class, status, module/revision,
owner/generation/target và safe outputs/consumers. Resource UNREFERENCED vẫn
truy cập được sau khi Workload đã gỡ. Root/target không lấy từ tab Environment
mới nếu đây là instance của generation cũ.

PE/Admin có nút Xóa tài nguyên và luồng xác nhận theo UC-07. Trang hiển thị
progress REQUESTED/CHECKING/DESTROYING/VERIFYING, tập đã xác nhận, kết quả
absence checks và IDs còn lại nếu lỗi. Poll safe persisted query; không raw
Terraform log/state/plan. Developer đọc metadata theo quyền Environment nhưng
không thấy hoặc thực hiện nút Xóa.

## Desktop visual review — 2026-10-23

Use [accepted prototype](../../../../frontend/design/deployment-preview/README.md)
and [shared review contract](../../../architecture/contracts/deployment-review.md).
Keep existing AppShell/sidebar, application title/environment tabs, buttons,
typography and panels. Graph on left, selected node inspector on right; Overview,
Changes and Code stay linked to the same canonical node ID. Code shows red/green
line diff, old/new line numbers, after-only/copy and expand/collapse.

Cloud placement groups actual AWS/VPC/EKS namespace identities; Aurora/ElastiCache
remain outside EKS when so selected. Internal shows retained cluster and authored
namespace/resources. No hardcoded seven/six-node diagram or region/account fixture.
Canvas arrows may reverse canonical consumer→provider to show execution order,
but label direction clearly and preserve all reasons in dependency detail.
Shared nodes appear once; unknown placement is labeled unresolved.

Pending Preview uses existing explicit Deploy; standalone remains read-only.
During execution show observed node states, stage/time/wait reasons and counts;
never render prototype scenario/failure selectors, fake timer, pause or reset.
No separate mobile design is required by this task; preserve existing shell.

### Shared review presentation — 2026-10-23

The same graph/inspector follows the accepted desktop prototype: icon/title/state
hierarchy, an observed readiness/stage/time card, safe configuration properties,
provider-grouped dependency reasons and one selected code file. Derive the
readiness count from persisted facts; dependency levels are not a claim of
parallel execution. Keep exact run correlation and unknown/error states intact.

## Native telemetry — TEL

Approved reference: [mock](../../../../frontend/design/logging-monitoring/README.md).
Application home puts the telemetry panel below staging/production tabs, before
Workloads. Four clickable tiles show ERROR lines, unusual events, CPU peak and
memory peak, with interval control and compact trends. Notable rows show
workload/time/recovered versus current state. Counting/threshold details live in
accessible help. Retain common shell, buttons, cards and message catalogs.

Logs detail retains app/environment, with workload, interval, literal search and
recognized-level filters, older-page navigation, details/copy and follow/pause.
Native Kubernetes combines stdout/stderr, so the product uses a level filter
instead of the mock's stream selector. Metrics shows CPU (mCPU) and RAM (MiB),
per-workload historical series, null gaps, true peaks and actual latest state.
Summary clicks preselect the related filters/focus; filter edits clear old
context. No fixture timestamps, scenario toolbar or synthesized metrics enter
these product views. Keyboard and mouse can inspect chart points.

Narrow application views wrap the four telemetry summary cards into two columns
and keep the detail charts in one column. Generated resource descriptors wrap
inside their rows while retaining visible status; they do not widen the viewport.

## Optional CD / reconciliation — ADR-030

[Interaction design](../../UC-01/ui/delivery-reconciliation.md) defines the Environment opt-in, transition review
and independent sync/health/drift presentation. Required design, not implemented UI.
