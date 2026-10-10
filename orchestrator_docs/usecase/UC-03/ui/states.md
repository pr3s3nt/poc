---
id: UC-03-UI-STATES
artifact: use-case-ui-states
status: current
last_reviewed: 2026-10-10
related: UC-03
---

# UC-03 UI states

- Loading: đang tải Resource Types và Definitions.
- Empty: chưa có Definition; form vẫn dùng được nếu có Resource Type.
- Ready: list và form hiển thị.
- Saving: khóa submit và chỉnh form đang gửi; không để response thành công xóa
  input mới chưa được submit.
- Error: giữ nguyên form khi JSON/contract/connection không hợp lệ.
- Created: nạp lại danh sách, reset form.
- Created/load error: giữ thông báo registration đã commit; reset submitted
  form, tách lỗi reload khỏi lỗi POST và cho Retry tải list, không tự đăng ký
  lại. Lỗi tải ban đầu có Retry và không giả danh empty catalog.

## Trạng thái form T02

- Loading/error của Loại tài nguyên hoặc Kết nối không ghi đè metadata đang
  nhập; có nút Thử lại. Không giả lỗi tải thành catalog rỗng.
- Kết nối lọc READY theo kind của driver. Không có lựa chọn hợp lệ thì giải
  thích rõ; Terraform không submit khi thiếu Connection, Kubernetes vẫn dùng
  Connection của Môi trường khi không override.
- ID rỗng/không hợp lệ có lỗi Việt rõ; duplicate 409 giữ toàn bộ form để sửa
  ID và thử lại. Không chuẩn hóa ID âm thầm.
- Submit lỗi giữ metadata, criteria, JSON và lựa chọn; không tự replay POST.
- Đổi type/driver hoặc mở/đóng nâng cao không mất JSON/criteria hoặc lựa chọn
  trước đó; controls và payload chỉ dùng driver/type đang có hiệu lực.
- Created/reload error phân biệt đăng ký thành công với lỗi tải danh sách;
  Thử lại chỉ tải list, không đăng ký lại.
