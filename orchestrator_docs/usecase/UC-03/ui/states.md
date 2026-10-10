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

## Trạng thái editor T02B

- Tải/lỗi danh sách ứng dụng có thông báo Việt và Thử lại; không sửa criteria
  đang nhập. Môi trường chỉ lấy từ ứng dụng tương ứng; response đến muộn hoặc
  refresh không ghi đè edits/lựa chọn mới.
- Criteria nâng cao nhiều dòng hoặc chứa field không biểu diễn được trong chế
  độ đơn giản được giữ lossless; không âm thầm bỏ field khi chuyển chế độ.
- Copy chỉ áp dụng khi người dùng thao tác tường minh, clone các dòng; thay
  đổi nguồn hoặc tải lại catalog không cập nhật bản đã sao chép.
- Context xem trước chưa đủ có trạng thái cần thêm context. Cảnh báo chồng
  lấn chỉ nêu nguy cơ; không coi trợ giúp frontend là matching authoritative.
- Submit lỗi giữ chế độ, criteria và metadata; submit thành công reset wildcard.
