---
id: UC-04-UI-STATES
artifact: use-case-ui-states
status: current
last_reviewed: 2026-09-30
related: UC-04
---

# UC-04 UI states

- Loading/empty/ready: tải và hiển thị connection của Organization.
- Verifying: khóa submit và chỉnh form đang gửi khi backend kiểm tra API/RBAC;
  không để response thành công xóa input mới chưa được submit.
- Error: hiển thị lý do từ chối, giữ nguyên form.
- Registered: nạp lại list và reset form; chỉ connection `READY` được hiển thị.
- Registered/load error: báo đúng registration đã commit, reset form và tách lỗi
  reload với Retry list; không tự đăng ký lại. Lỗi tải ban đầu có Retry và không
  giả danh empty list.
