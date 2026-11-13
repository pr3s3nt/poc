---
id: UC-16-USER-GUIDE-SPEC
artifact: use-case-specification
status: current
last_reviewed: 2026-11-13
related: UC-12, UC-16
---

# Hướng dẫn sử dụng trong Console — issue #82

Phạm vi người dùng duyệt: thêm mục User Guide trong sidebar; giai đoạn này chỉ
viết bài nhập biến môi trường. Bài hướng dẫn giải thích behavior hiện hành của
[UC-16](specification.md), [ENVFILE](envfile-specification.md) và UC-12, không thay
đổi grammar, API, Secret hoặc Preview/Deploy.

## GUIDE-01 — Entry và phạm vi

Mọi vai trò đã đăng nhập mở được mục **Hướng dẫn sử dụng / User Guide** ở sidebar,
route `/ui/user-guide`. Mục này có icon, active state, accessible name khi thu gọn
và giữ pattern shell hiện hành. Đăng nhập vẫn dùng boundary UC-00. Route hỗ trợ
refresh, back/forward. Trang có mục lục theo bài; hiện chỉ xuất bản **Viết và nhập
biến môi trường**, không hiển thị placeholder cho chương chưa viết.

## GUIDE-02 — Nội dung theo tác vụ

Bài giải thích biến thuộc từng container; cách mở Add/Edit Workload, nhập bảng
Tên/Giá trị, chọn nguồn; tên biến hợp lệ; năm loại nguồn và điều kiện metadata.
Có mẫu .env với dữ liệu giả, grammar quoted/unquoted/comment/escape, literal @,
không nội suy hoặc shell; upload UTF-8 tối đa 1 MiB, merge có review khác chỉnh
trực tiếp toàn draft; tải mẫu/nội dung không resolve Secret/output.

Bí mật nhập trực tiếp dùng marker `# @secret` ngay trước một literal hoặc đánh dấu
Bí mật ở bảng, rồi xác nhận tạo Secret. Hướng dẫn nói rõ kho/phạm vi app-env, tên
mặc định, reuse tường minh và không overwrite, kết quả từng dòng, không tự replay
unknown outcome, Secret đã tạo không mất khi hủy workload. Không đoán secret từ
tên biến. Giá trị đang nhập có thể hiện trong editor/file; ví dụ không dùng secret
thật. Kết thúc bằng Save draft → Preview → Deploy và cách sửa lỗi thường gặp.

## GUIDE-03 — Trình bày và validation

Nội dung và nhãn theo VI/EN message catalogs. Dùng heading, list, table, code block
và mục lục anchor có focus/keyboard; chỉ một h1, không theme hoặc shell mới. Code
block được cuộn trong khung, không gây tràn trang. Nội dung chỉ đọc, không form,
không mutation, không tự resolve nguồn, không phụ thuộc Application được chọn.
Nội dung đóng gói cùng frontend, không có data-loading/error state riêng; auth
và tải catalog giữ boundary hiện hành. UI design ở [screens](ui/screens.md#user-guide--issue-82).

Kiểm tra route/auth/navigation, VI/EN, collapsed sidebar, keyboard, deep link và
refresh; đối chiếu ví dụ bằng parser hiện hành. Browser với backend thật kiểm
reference Applications và link feature theo [runbook](../../operations/tailscale-handoff.md).
