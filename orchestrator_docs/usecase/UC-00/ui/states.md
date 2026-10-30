---
id: UC-00-UI-STATES
artifact: use-case-ui-states
status: current
last_reviewed: 2026-10-29
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

## Đổi mật khẩu (BR-08)

| State | UI behavior |
|---|---|
| Initial | Ba input rỗng; primary action enabled. |
| Client validation | Inline field error (bắt buộc, tối thiểu 8 ký tự, xác nhận khớp); no API request. |
| Submitting | Fieldset disabled; button reads `Đang đổi mật khẩu…`. |
| Wrong current password | `401` `INVALID_CREDENTIALS` → inline `Mật khẩu hiện tại không đúng.`; form mở lại để sửa. |
| Network/API error | Form-level `Không thể đổi mật khẩu. Kiểm tra kết nối và thử lại.`; không hiện raw backend error. |
| Success | Local session kết thúc; Sign in hiện `Mật khẩu đã thay đổi. Đăng nhập lại bằng mật khẩu mới.` |

## Người dùng (BR-09)

| State | UI behavior |
|---|---|
| Loading | `Đang tải người dùng…` với `aria-busy`. |
| Load error | `Không tải được danh sách người dùng.` + nút `Thử lại`; retry chỉ GET lại. |
| Client validation | Inline field error cho username/password; no API request. |
| Duplicate username | `409` `DUPLICATE` → inline `Tên đăng nhập đã tồn tại.`; giữ draft username/role, xóa mật khẩu. |
| Network/API error | Form-level `Không thể tạo tài khoản. Kiểm tra kết nối và thử lại.`; không tự replay mutation. |
| Success | `Đã tạo tài khoản {username}.` ở `role="status"`; form reset, danh sách refresh. |

## Đặt lại mật khẩu (BR-11)

| State | UI behavior |
|---|---|
| Initial | Hai input rỗng; primary action enabled; focus vào input đầu. |
| Client validation | Inline field error (bắt buộc, tối thiểu 8 ký tự, xác nhận khớp); no API request. |
| Submitting | Fieldset disabled; button reads `Đang đặt lại…`; dialog không đóng giữa mutation. |
| Account gone | `404` → đóng dialog, refresh danh sách, thông báo account không còn tồn tại. |
| Network/API error | Form-level `Không thể đặt lại mật khẩu. Kiểm tra kết nối và thử lại.`; không hiện raw backend error. |
| Success | Dialog đóng; `Đã đặt lại mật khẩu của {username}.` ở `role="status"`; mọi session của account đó đã bị backend thu hồi. |

Nút hiện/ẩn mật khẩu chỉ đổi `type` của input trong phiên hiển thị, không phải
state nghiệp vụ và không gửi thêm request.
