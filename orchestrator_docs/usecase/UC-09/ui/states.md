---
id: UC-09-UI-STATES
artifact: use-case-ui-states
status: current
last_reviewed: 2026-10-11
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
