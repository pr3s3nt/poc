---
id: UC-03-UI-V3-PROTOTYPE
artifact: use-case-ui-prototype
status: draft
last_reviewed: 2026-10-21
related: UC-03
---

# Bản mẫu giao diện Resource Definition v3 — issue #8

Bản mẫu v3 là mốc review giao diện cho
[issue #8](https://github.com/pr3s3nt/poc/issues/8), chưa phải implementation
sản phẩm hay specification thay thế. Specification hiện hành và phạm vi issue
quy định behavior; các file dưới đây cố định bố cục và tương tác đã review.

## File thiết kế được lưu trong Git

- [Hướng dẫn mở và giới hạn bản mẫu](../../../../frontend/design/resource-definition-v3/README.md)
- [Giao diện HTML](../../../../frontend/design/resource-definition-v3/index.html)
- [Editor nâng cao JavaScript](../../../../frontend/design/resource-definition-v3/advanced.js)
- [Ảnh tab Cấu hình](../../../../frontend/design/resource-definition-v3/configuration.png)
- [Ảnh tab Điều kiện áp dụng](../../../../frontend/design/resource-definition-v3/matching.png)
- [Ảnh thiết lập nâng cao](../../../../frontend/design/resource-definition-v3/advanced.png)

Mở HTML cùng thư mục với JavaScript hoặc chạy server theo README. Không cần
backend, tài khoản hay credential. Issue liên kết tới commit cố định để agent
triển khai không phụ thuộc server preview tạm thời hoặc file trong `/tmp`.

## Ranh giới thiết kế

Giữ hai tab Cấu hình / Điều kiện áp dụng và style Console hiện tại. Phần nâng
cao vẫn có input/output mapping, provider binding và tài nguyên đi kèm. Giữ
backend contract hiện hành, bao gồm module pins, secret rules, CAS và matching
máy chủ. Không dùng v4, không thêm Usage, không bắt tên biến Terraform trùng
Resource Type và không chuyển payload sang contract Humanitec.

Dữ liệu, metadata và trạng thái trong bản mẫu là minh họa. Lưu, kiểm tra Git và
matching không gọi API hoặc tạo tài nguyên. JSON nâng cao minh họa một fragment,
không thay thế Definition DTO đầy đủ. Khi triển khai phải dùng metadata thật,
giữ validation và draft/round-trip safeguards hiện hành; không copy fixture,
giới hạn validation hoặc kết quả giả thành behavior sản phẩm.
