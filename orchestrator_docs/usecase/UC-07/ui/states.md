---
id: UC-07-UI-STATES
artifact: use-case-ui-states
status: current
last_reviewed: 2026-10-12
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
