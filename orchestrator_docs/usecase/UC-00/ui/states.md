---
id: UC-00-UI-STATES
artifact: use-case-ui-states
status: current
last_reviewed: 2026-10-12
related: UC-00
---

# UC-00 UI states

| State | UI behavior |
|---|---|
| Initial | Empty username/password inputs; primary action enabled. |
| Client validation | Inline required-field message; no API request. |
| Submitting | Button reads `Đang đăng nhập…`; both inputs and submit action are disabled. |
| Invalid credentials | Generic form-level message: `Tên đăng nhập hoặc mật khẩu không đúng.` Never reveal which field failed. |
| Network/API error | Form-level message with retryable `Đăng nhập` action; do not clear username. |
| Session expired | Redirect to Sign in with one info callout: `Phiên đăng nhập đã kết thúc. Đăng nhập lại để tiếp tục.` |
| Success | Redirect directly to UC-01 Applications home. |

Validation bắt buộc: “Nhập tên đăng nhập.” / “Nhập mật khẩu.”. Lỗi mạng/API
dùng “Không thể đăng nhập. Kiểm tra kết nối và thử lại.”; không hiện raw backend
error. Khôi phục phiên hiển thị “Đang tải Orchestrator…”. Đăng xuất vẫn xóa
context local ngay cả khi request thất bại; session expiry giữ thông báo Việt.

## Lỗi theo code — T20

Áp dụng [public HTTP error contract](../../../architecture/contracts/operation-contracts.md#public-http-error-contract--t20--vi-03)
và [shared error presentation](../../../architecture/ui/README.md#trình-bày-lỗi-theo-code--t20--vi-03).
Known code chọn hướng dẫn Việt cụ thể; unknown/missing code dùng fallback Việt
an toàn, không hiển thị/parse raw `error`, submitted values hoặc provider cause.
Status/scoping và guards hiện hành giữ nguyên. Giữ draft/confirmation theo flow;
BUSY không mất edits, stale cần reload/review hoặc Preview mới. Retry tường minh
không tự replay mutation; lỗi tải lại không phủ nhận kết quả đã commit.
