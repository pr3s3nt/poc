---
id: UC-02-UI-STATES
artifact: use-case-ui-states
status: current
last_reviewed: 2026-09-30
related: UC-02
---

# UC-02 UI states

- Loading: đang nạp catalog của Organization.
- Empty: chưa có type; form vẫn dùng được.
- Ready: hiện danh sách và form.
- Saving: khóa submit và chỉnh form đang gửi để tránh gửi lặp hoặc mất input
  chưa gửi khi response thành công reset form.
- Error: lỗi tải hoặc lưu được hiển thị; form không bị xóa khi lưu lỗi.
- Created: danh sách được nạp lại và form trở về rỗng.
- Created/load error: registration đã commit vẫn được báo thành công; lỗi nạp
  lại catalog được báo riêng với Retry. Không giữ nguyên form như thể POST chưa
  thành công hoặc tự đăng ký lại. Lỗi tải ban đầu có Retry và không giả danh
  empty catalog.
