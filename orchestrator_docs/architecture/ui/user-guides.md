---
id: UI-USER-GUIDES
artifact: shared-ui-design
status: current
last_reviewed: 2026-11-05
---

# Hướng dẫn sử dụng trong Console

## Behavior được duyệt

- **GUIDE-01:** Sidebar thêm **Hướng dẫn sử dụng**, route `/ui/guides`, dùng
  shell/icon/active state/collapse hiện hành. Nhấn mở nội dung hướng dẫn ngay
  trong Console; reload/deep link giữ trang. Yêu cầu đăng nhập như các trang khác.
- **GUIDE-02:** DEV xem hướng dẫn DEV. Platform Engineer/Admin cũng xem hướng
  dẫn DEV vì có quyền tạo ứng dụng; các bài Platform Engineer bổ sung sau chỉ
  dành cho Platform Engineer/Admin. Task này tích hợp một bài DEV đã duyệt,
  không tự viết bài Platform Engineer hoặc tạo nhóm nội dung rỗng.
- **GUIDE-03:** Hiển thị [bài acceptance app](../../guides/dev/acceptance-app.md)
  theo thứ tự đã duyệt, có mục lục theo các bước, bảng cấu hình, ảnh thật và
  thao tác mở ảnh kích thước đầy đủ. Các ảnh ban đầu phải phản ánh app trống,
  các ảnh sau lưu phải là draft; không dùng app đã deploy minh họa lúc tạo mới.
- **GUIDE-04:** Nội dung văn bản có một nguồn Markdown trong repository, được
  đóng gói lúc build. Ảnh minh họa đã duyệt là static product assets cùng origin,
  không phụ thuộc server bản nháp/Tailscale hoặc filesystem của máy tác giả.
  Không đưa browser session, credential, logs hoặc capture runner vào sản phẩm.
- **GUIDE-05:** Tái sử dụng `.page`, `.content-panel`, màu, typography và Dialog
  hiện hành. Nội dung bài tiếng Việt; UI controls dùng VI/EN message catalogs.
  Không tự dịch hoặc đổi bài đã duyệt khi đổi ngôn ngữ Console.

## Trình bày và states

Một tiêu đề trang **Hướng dẫn sử dụng**, tên bài DEV và mục lục ngắn theo các
mục cấp hai, tiếp theo là nội dung. Chọn mục lục cuộn tới mục tương ứng. Ảnh có
alt/caption, xem lớn bằng bàn phím hoặc chuột; đóng bằng nút hoặc Escape, trả
focus về ảnh. Bảng rộng cuộn trong nội dung, không đẩy tràn shell. Ảnh tải lỗi
có thông báo và nút thử lại riêng; phần chữ vẫn đọc được. Không render HTML tùy
ý từ Markdown. Nội dung nguồn/verification dành cho maintainer không xuất hiện
trong bài sử dụng; không thêm mô tả implementation vào flow của DEV.

Không thêm thao tác tạo app/deploy tự động khi mở tài liệu. Đọc hướng dẫn không
thay đổi dữ liệu ứng dụng, secret, Kubernetes hoặc cloud.
