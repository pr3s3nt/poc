---
id: UC-00-UI-SCREENS
artifact: use-case-ui-screens
status: current
last_reviewed: 2026-10-11
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
