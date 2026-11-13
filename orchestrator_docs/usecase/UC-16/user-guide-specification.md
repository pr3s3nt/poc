---
id: UC-16-USER-GUIDE-SPEC
artifact: use-case-specification
status: current
last_reviewed: 2026-11-13
related: UC-12, UC-16
---

# Hướng dẫn sử dụng trong Console — issue #82

Phạm vi người dùng duyệt và bổ sung ngày 2026-11-13: đặt User Guide cuối sidebar,
viết bài nhập biến môi trường và bài Sử dụng Workload. Bài hướng dẫn giải thích behavior hiện hành của
[UC-16](specification.md), [ENVFILE](envfile-specification.md) và UC-12, không thay
đổi grammar, API, Secret hoặc Preview/Deploy.

## GUIDE-01 — Entry và phạm vi

Mọi vai trò đã đăng nhập mở được mục **Hướng dẫn sử dụng / User Guide** ở sidebar,
route `/ui/user-guide`. Entry nằm cuối danh sách Điều hướng chính, sau cả mục
Lịch sử triển khai chưa khả dụng; footer tài khoản/ngôn ngữ giữ nguyên. Mục này
có icon, active state, accessible name khi thu gọn
và giữ pattern shell hiện hành. Đăng nhập vẫn dùng boundary UC-00. Route hỗ trợ
refresh, back/forward. Trang có mục lục theo bài: **Viết và nhập biến môi trường**
(mặc định) và **Sử dụng Workload** (`/ui/user-guide?topic=workloads`). Chỉ một bài
hiện tại được render, có mục lục anchor riêng. Đổi bài cập nhật URL, đưa focus
về tiêu đề bài và cuộn về đầu bài; refresh/history giữ bài được chọn. Topic không
hợp lệ trở về bài mặc định. Không hiển thị placeholder cho chương chưa viết.

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

## GUIDE-04 — Sử dụng Workload

Bài mới hướng dẫn chọn ứng dụng/môi trường, chọn template và version được công
bố hoặc tự cấu hình; phân biệt Deployment, StatefulSet (hiện một replica) và
CronJob. Theo [hướng dẫn authoring](../../operations/workload-authoring.md),
[WD](workload-defaults-specification.md), [WC](workload-catalog-specification.md),
UC-16 và UC-07 hiện hành: tên DNS cố định sau lưu đầu (63 ký tự, CronJob 52),
container/image/roles, default mờ và Restore defaults, biến môi trường (link sang
bài hiện có), Service/target port và endpoint riêng khi được yêu cầu, dependency
và resource reuse, compute/probes/command/security, storage/PVC/retention, CronJob
schedule/policy, Save → Preview → Deploy, cập nhật/xóa pending/undo và lỗi thường
gặp. Service/Dependencies hiện trực tiếp, không hướng dẫn người dùng tìm chúng
trong nhóm nâng cao cũ. Không hướng dẫn tab nhập Workload đang ẩn.

Có ví dụ web theo trường form với dữ liệu minh họa và cấu hình cổng rõ ràng; không
khẳng định đã deploy workload qua việc đọc hướng dẫn. Nội dung giải thích riêng
namespace/workload/container, biến theo container, tài nguyên bền không bị xóa
ngầm và required platform setup trước Deploy. Không tạo requirement nghiệp vụ mới.
VI/EN, link bài/anchor và active article dùng shared shell/style; backend/API và
semantics Save/Preview/Deploy giữ nguyên.
