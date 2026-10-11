---
id: UC-00-UI-STATES
artifact: use-case-ui-states
status: current
last_reviewed: 2026-10-11
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
