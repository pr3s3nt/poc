---
id: UC-02-UI-STATES
artifact: use-case-ui-states
status: current
last_reviewed: 2026-09-29
related: UC-02
---

# UC-02 UI states

- Loading: đang nạp catalog của Organization.
- Empty: chưa có type; form vẫn dùng được.
- Ready: hiện danh sách và form.
- Saving: khóa submit để tránh gửi lặp.
- Error: lỗi tải hoặc lưu được hiển thị; form không bị xóa khi lưu lỗi.
- Created: danh sách được nạp lại và form trở về rỗng.
