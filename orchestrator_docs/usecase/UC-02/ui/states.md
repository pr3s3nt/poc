---
id: UC-02-UI-STATES
artifact: use-case-ui-states
status: current
last_reviewed: 2026-10-12
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

## Trạng thái T04

- Chi tiết hiển thị schema nhận từ GET, kể cả schema rỗng; không sửa catalog.
- Sao chép tạo draft độc lập, ID rỗng; người dùng sửa/xác nhận trước đăng ký.
  Reload không ghi đè draft. Saving khóa copy và controls của form đang gửi.
- Duplicate 409 giữ ID/schema và báo lỗi Việt; không cập nhật record nguồn.
- Loading/empty/error/retry phân biệt rõ, dùng tiếng Việt; lỗi server dùng
  fallback Việt an toàn, không hiển thị raw English error. Mapping code theo contract lỗi T20.
- Created/load error vẫn báo đã đăng ký; Retry chỉ GET, không lặp POST.
- CTA chỉ có cho type hỗ trợ; custom contract không có CTA triển khai.

## Lỗi theo code — T20

Áp dụng [public HTTP error contract](../../../architecture/contracts/operation-contracts.md#public-http-error-contract--t20--vi-03)
và [shared error presentation](../../../architecture/ui/README.md#trình-bày-lỗi-theo-code--t20--vi-03).
Known code chọn hướng dẫn Việt cụ thể; unknown/missing code dùng fallback Việt
an toàn, không hiển thị/parse raw `error`, submitted values hoặc provider cause.
Status/scoping và guards hiện hành giữ nguyên. Giữ draft/confirmation theo flow;
BUSY không mất edits, stale cần reload/review hoặc Preview mới. Retry tường minh
không tự replay mutation; lỗi tải lại không phủ nhận kết quả đã commit.
