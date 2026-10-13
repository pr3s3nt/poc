---
id: UC-07-UI-STATES
artifact: use-case-ui-states
status: current
last_reviewed: 2026-10-13
related: UC-07, UC-16
---

# UC-07 UI states

| State | Behavior |
|---|---|
| Loading/error | No fabricated rows; retry list load; mutation disabled until data available. |
| Edit pending | Preserve input on save error; no runtime success claim. |
| Delete confirmation | Cancel does nothing; confirm marks pending only; Undo restores prior state. |
| Mutation/Preview busy | Prevent duplicate/conflicting actions; no use of a stale token. |
| Deploy busy | Disable conflicting edits/deletes/Undo/Preview and Environment switching until completion. |
| Scope/input changes | Clear obsolete Preview/report; ignore late list/Preview/mutation responses. |
| Version/token conflict | Explain stale state, reload list, require deliberate new Preview; failed reload disables mutation until retry succeeds. Editor preserves unsaved input and requires explicit reload/review before saving again. |
| Success | Show per-target result immediately; refresh selected Environment list/recent history. A failed list reload must not replace or hide the committed deployment outcome; show a separate retryable load error. |
| Failed/partial | Show failed/skipped separately from succeeded; unfinished changes remain retryable through fresh Preview. |
| No effective change | Deploy disabled except existing route-only retry. |

Controls and errors remain keyboard-accessible. Confirmation must be visible
and actuated by user input in review recordings, not bypassed offscreen.

## Lỗi theo code — T20

Áp dụng [public HTTP error contract](../../../architecture/contracts/operation-contracts.md#public-http-error-contract--t20--vi-03)
và [shared error presentation](../../../architecture/ui/README.md#trình-bày-lỗi-theo-code--t20--vi-03).
Known code chọn hướng dẫn Việt cụ thể; unknown/missing code dùng fallback Việt
an toàn, không hiển thị/parse raw `error`, submitted values hoặc provider cause.
Status/scoping và guards hiện hành giữ nguyên. Giữ draft/confirmation theo flow;
BUSY không mất edits, stale cần reload/review hoặc Preview mới. Retry tường minh
không tự replay mutation; lỗi tải lại không phủ nhận kết quả đã commit.

## Xóa tài nguyên

| State | Hành vi |
|---|---|
| Không có quyền | Developer không thấy nút; server vẫn trả 403 nếu gọi mutation. |
| Không đủ điều kiện | Disabled với lý do chưa UNREFERENCED/còn consumer/platform/busy; không mở delete plan hợp lệ. |
| Đang tải phạm vi | Confirm disabled; không coi missing metadata là dependency rỗng. |
| Chọn phạm vi | Chỉ root tick mặc định; dependency để trống. Khóa mục có consumer ngoài tập; bỏ tick kéo theo bỏ các lựa chọn trở nên không hợp lệ, có giải thích. Platform không xuất hiện. |
| Xác nhận | Liệt kê đúng các mục đã tick; cảnh báo mất dữ liệu vĩnh viễn; yêu cầu exact root name. |
| Stale/busy | Không destroy; giữ hướng dẫn, tải và review scope mới; không tự gửi lại tên/token cũ. |
| Mutation mất phản hồi | Reload operation/detail để xác định đã nhận; không tự tạo request mới. |
| REQUESTED/CHECKING/DESTROYING/VERIFYING | Khóa thao tác xung đột; poll đúng operation/scope, giữ kết quả từng instance. |
| SUCCEEDED | Tất cả IDs trong phạm vi đã xác minh hết, resource DESTROYED; history giữ nguyên. |
| FAILED/INTERRUPTED | Hiển thị remaining IDs/safe reason; không coi đã cleanup hoặc tự rollback/replay. |

Đổi Application/Environment/resource bỏ response đến muộn và dialog cũ.
Confirmation có focus/keyboard labels và xuất hiện trong headed video, không
được Playwright bypass qua API. Retry cleanup của operation đã nhận vẫn theo
quyền/owner/fence ban đầu; không tạo user deletion mới với scope rộng hơn.
