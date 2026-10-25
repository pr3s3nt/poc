---
id: UC-09-UI-STATES
artifact: use-case-ui-states
status: current
last_reviewed: 2026-10-23
related: UC-09
---

# UC-09 UI states

| Screen | State | Behavior |
|---|---|---|
| Recent/history | Loading/empty | Keep selected Environment visible; show loading or no deployments, never mock rows. |
| Recent/history | Ready | Show newest first, status filter, and detail links. |
| Recent/history | Error | Show error and retry without claiming empty history. |
| Detail | Loading | Keep Application/Environment navigation and show an explicit busy state. |
| Detail | Not found | Show scoped not-found without a retry action; this includes a Deployment outside the session Organization/Application/Environment. |
| Detail | API error | Show a retryable error without rendering stale detail. |
| Detail | Failed | Show safe failure summary and last recorded states. Unknown legacy raw reasons become generic safe guidance. If planning failed before Snapshot, leave Delta/graph/batches absent rather than inventing them. |
| Detail | Succeeded | Show persisted plan, graph, batches, resource/workload states and redacted outputs. |

## Trạng thái lịch sử tiếng Việt — T14

- Loading: “Đang tải lịch sử triển khai…” có role status và aria-busy; giữ
  Application/Environment và bộ lọc nhìn thấy được.
- Empty: “Chưa có triển khai nào trong môi trường này.”; khi lọc dùng thông
  báo tiếng Việt kèm nhãn trạng thái đã chọn, không hiển thị enum như câu tiếng Anh.
- Error: “Không thể tải lịch sử triển khai.” và nút “Thử lại”, không đưa raw
  backend error vào UI và không coi lỗi là danh sách rỗng.
- Đổi app/env/filter hoặc retry xóa danh sách cũ khi tải; phản hồi đến muộn của
  scope/filter cũ không thay kết quả hiện tại. Payload enum và API không đổi.

## Trạng thái chi tiết tiếng Việt — T15

- Loading: “Đang tải chi tiết triển khai…” với role status và aria-busy; giữ
  navigation Application/Environment, xóa detail cũ khi scope/ID thay đổi.
- Not found/inaccessible: “Không tìm thấy triển khai” cùng giải thích an toàn
  trong scope đã chọn; không có nút “Thử lại”, không tiết lộ tenant khác.
- API error: “Không thể tải chi tiết triển khai.” và nút “Thử lại”; không hiện
  raw error, không render detail cũ. Retry chỉ đọc API theo thao tác tường minh.
- Failed: badge “Thất bại”, hướng dẫn Việt an toàn; không có plan thì ghi
  “Không có kế hoạch tạo tài nguyên được lưu cho triển khai này.”, không dựng
  graph/batches giả. Unknown failure dùng fallback, không thay từ bằng regex.
- Succeeded: badge “Thành công”, dữ liệu persisted và output đã che. Các enum
  khác được dịch nhãn nhưng không đổi dữ liệu API; empty Workload/tài nguyên/
  batches có thông báo Việt. Phản hồi hoặc lỗi đến muộn của app/env/ID/lượt đọc
  cũ không thay kết quả hiện tại.

## Lỗi theo code — T20

Áp dụng [public HTTP error contract](../../../architecture/contracts/operation-contracts.md#public-http-error-contract--t20--vi-03)
và [shared error presentation](../../../architecture/ui/README.md#trình-bày-lỗi-theo-code--t20--vi-03).
Known code chọn hướng dẫn Việt cụ thể; unknown/missing code dùng fallback Việt
an toàn, không hiển thị/parse raw `error`, submitted values hoặc provider cause.
Status/scoping và guards hiện hành giữ nguyên. Giữ draft/confirmation theo flow;
BUSY không mất edits, stale cần reload/review hoặc Preview mới. Retry tường minh
không tự replay mutation; lỗi tải lại không phủ nhận kết quả đã commit.


## Metadata và tiến trình tài nguyên

Module/class metadata đang tải, lỗi hoặc unavailable chặn thao tác chưa kiểm
chứng; retry giữ edits, không thay bằng default hay replay mutation. Known
input/provider/integrity error dùng code/field an toàn. BUSY giữ dữ liệu đang
sửa; stale yêu cầu review/Preview mới. Progress lấy persisted stage/attempt,
không raw CLI/state/secret; interruption không được coi là cleanup thành công.

## Resource detail / cleanup

Missing resource/foreign scope là 404, khác lỗi tải. UNREFERENCED vẫn có detail.
Không đủ dữ liệu eligibility thì disabled; Developer không thấy mutation.
Polling giữ đúng operation ID và chỉ kết thúc thành công khi tất cả confirmed
IDs verified absent; partial/remaining IDs hiển thị rõ, late response bị bỏ.
Provider error/unknown cause dùng safe code, không raw state hoặc log.

## Artifact and progress states

- Artifact loading is scoped to selected plan/node; stale replies are discarded.
- AVAILABLE/PARTIAL show safe text plus provenance; unknown/redacted paths are
  visibly labeled. ABSENT is confirmed creation/removal. UNAVAILABLE explains
  missing baseline/content/unsupported render/limit without inventing a diff.
- WAITING lists real unfinished providers; BLOCKED names failed providers;
  PROVISIONING and CHECKING remain distinct. Missing records map UNKNOWN or
  known not-started PENDING, not READY. Plan intent is separate from runtime.
- Poll failure retains last observation marked stale, allows GET retry and never
  replays Deploy. Scope/run change cancels requests; terminal stops polling.
- Progress is a completed-node count, not simulated ETA or percentage of work.

## Application availability diagnosis — 2026-10-23

Known resource-list `503 SERVICE_UNAVAILABLE` is an explicit unavailable state,
not an empty inventory or evidence of Deployment failure. Explain that backend
data is unavailable and offer an explicit GET retry. Keep unknown errors, 500,
malformed lists and network failures as load errors with retry. Do not hide or
rewrite the HTTP 503, synthesize Terraform records or weaken cleanup guards.

## Telemetry states

TEL views distinguish not deployed, waiting for data, missing Metrics Server,
partial/stale/gaps, storage limit, service unavailable and query failure. Keep
retained data while explicitly stale. Missing CPU/RAM/limits are dashes, not
zero or percentages. Logs can work while metrics are unavailable. Collection
success is not app readiness. Environment/filter changes cancel or ignore late
responses; summary/deep links preserve scope and interval.
