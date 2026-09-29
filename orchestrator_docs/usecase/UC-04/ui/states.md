---
id: UC-04-UI-STATES
artifact: use-case-ui-states
status: current
last_reviewed: 2026-09-29
related: UC-04
---

# UC-04 UI states

- Loading/empty/ready: tải và hiển thị connection của Organization.
- Verifying: khóa submit khi backend kiểm tra API/RBAC.
- Error: hiển thị lý do từ chối, giữ nguyên form.
- Registered: nạp lại list và reset form; chỉ connection `READY` được hiển thị.
