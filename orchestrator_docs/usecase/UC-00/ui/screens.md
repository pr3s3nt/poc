---
id: UC-00-UI-SCREENS
artifact: use-case-ui-screens
status: current
last_reviewed: 2026-10-29
related: UC-00
---

# UC-00 UI screens

## Sign in

The unauthenticated page is split into a dark brand panel and a calm form panel.
The visual separation makes sign-in feel like product entry, not a technical
admin form.

| Area | Content and behavior |
|---|---|
| Brand panel | `Orchestrator`, `Nền tảng nội bộ cho nhà phát triển`, Vietnamese product message and low-contrast infrastructure line art. |
| Form heading | `Chào mừng trở lại`, `Đăng nhập vào không gian làm việc` and `Đăng nhập để tạo và triển khai ứng dụng.` |
| Inputs | Required `Tên đăng nhập` and `Mật khẩu`; password is never retained in browser state after submit. |
| Primary action | `Đăng nhập`; disabled only while the request is in flight. |
| Helper text | `Dùng tài khoản kiểm thử local của bạn.` No self-registration, password-reset or social-login affordance. |

On success, redirect to `/ui/applications`. Do not show a separate success page.

T10 dùng text và accessible names tiếng Việt theo glossary; giữ username,
password payload và auth/session behavior. Brand message: “Xây dựng. Triển khai.
Làm việc nhanh hơn.”; trợ giúp: “Hạ tầng giúp bạn tập trung vào ứng dụng.”
Mật khẩu dùng input type password, xóa khỏi state sau submit, không ghi vào evidence.
Mọi ô nhập mật khẩu trong sản phẩm có nút hiện/ẩn nội dung: mặc định che (type
password), bấm `Hiện` chuyển sang plain text (type text), bấm `Ẩn` che lại; nút
là `type="button"` với accessible name riêng, không submit form.

## Đổi mật khẩu (BR-08)

Mọi User đã đăng nhập mở trang `/ui/account` từ nút `Đổi mật khẩu` ở footer
sidebar, cạnh `Đăng xuất`. Trang nằm trong authenticated shell hiện hành.

| Area | Content and behavior |
|---|---|
| Page heading | `Đổi mật khẩu` với mô tả ngắn: đổi thành công kết thúc mọi phiên đăng nhập, cần đăng nhập lại. |
| Inputs | `Mật khẩu hiện tại`, `Mật khẩu mới`, `Xác nhận mật khẩu mới`; đều type password với nút hiện/ẩn plain text. |
| Primary action | `Đổi mật khẩu`; disabled khi request đang chạy. |

Validation inline: bắt buộc nhập, mật khẩu mới tối thiểu 8 ký tự, xác nhận phải
khớp. Thành công chuyển về Sign in với thông báo `Mật khẩu đã thay đổi. Đăng
nhập lại bằng mật khẩu mới.`; không có trang success riêng.

## Người dùng (BR-09)

Sidebar thêm mục `Người dùng` tại `/ui/platform/users`, chỉ hiện và chỉ render
cho role ADMIN; Developer và Platform Engineer không thấy mục này và deep link
không render trang (backend vẫn là nguồn kiểm quyền cuối cùng).

| Area | Content and behavior |
|---|---|
| Danh sách | Bảng `Tên đăng nhập` · `Vai trò` · `Trạng thái` của các account trong Organization; không hiển thị hash hay mật khẩu. Mỗi hàng có action `Đặt lại mật khẩu` (BR-11). |
| Form tạo | `Tên đăng nhập`, `Mật khẩu` (type password, có nút hiện/ẩn) và `Vai trò` (DEVELOPER / PLATFORM_ENGINEER / ADMIN). |
| Primary action | `Tạo tài khoản`; thành công hiện `Đã tạo tài khoản {username}.` và refresh danh sách. |

Validation inline: username chỉ gồm chữ thường, số, `.`, `_`, `-`; mật khẩu tối
thiểu 8 ký tự. Trùng username báo lỗi tại field username; mật khẩu được xóa
khỏi form ngay sau submit, không giữ trong state sau khi gửi.

### Đặt lại mật khẩu (BR-11)

Action `Đặt lại mật khẩu` ở mỗi hàng mở dialog cho account đó; Admin không thấy
và không cần mật khẩu hiện tại của account.

| Area | Content and behavior |
|---|---|
| Dialog heading | `Đặt lại mật khẩu của {username}` với mô tả ngắn: đặt lại thành công kết thúc mọi phiên đăng nhập của tài khoản này. |
| Inputs | `Mật khẩu mới`, `Xác nhận mật khẩu mới`; đều type password với nút hiện/ẩn plain text. |
| Primary action | `Đặt lại mật khẩu`; disabled khi request đang chạy. `Hủy` đóng dialog không thay đổi gì. |

Validation inline: bắt buộc nhập, mật khẩu mới tối thiểu 8 ký tự, xác nhận phải
khớp. Thành công đóng dialog và hiện `Đã đặt lại mật khẩu của {username}.`;
mật khẩu không giữ trong state sau khi gửi.
