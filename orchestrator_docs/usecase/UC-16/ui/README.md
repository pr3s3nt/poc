---
id: UC-16-UI
artifact: use-case-ui-design
status: current
last_reviewed: 2026-10-24
related: UC-16, UC-01, UC-05, UC-12
---

# UC-16 UI — Workload configuration

## UX outcome

Developer thêm/sửa workload trong staging hoặc production. Mỗi container có
một bảng Biến môi trường: Tên biến + Giá trị nhập trực tiếp. “Chọn từ…” mở
một hộp tìm cấu hình dùng chung/output/Service, không liệt kê biến cục bộ.
Secret vẫn thuộc Application + Environment; form tạo tại chỗ giữ UC-12. Settings
UC-12 vẫn quản lý key dùng chung; Save workload chỉ tạo pending change,
Preview và Deploy vẫn tường minh. Public routes giữ contract hiện hành.

DEVEX-ENV được duyệt 2026-10-24; thay checklist và blanket literal prohibition.
Trạng thái và lịch sử: xem [handoff](../../../implementation/workload-environment-editor.md).

## Read in this order

1. [Screens and layout](screens.md)
2. [States and validation](states.md)
3. [UC-16 specification](../specification.md)
4. [Shared shell](../../../architecture/ui/README.md)

The interactive HTML preview used during review is not product code. This
package is the durable UI design; API mapping follows contract design.

## T08 UI scope

[Form cơ bản/nâng cao](screens.md#t08--form-cơ-bản-và-nâng-cao-nhãn-tiếng-việt)
và [trạng thái](states.md#t08--trạng-thái-nâng-cao-và-cổng-công-khai) sở hữu
progressive disclosure, tóm tắt defaults và lựa chọn một cổng sau thao tác thêm
route. Editor/bảng Biến môi trường dùng catalog Việt/Anh theo ADR-020; giữ Score semantics, typed params,
import lossless, aliases, BUSY/stale guards và explicit Save → Preview → Deploy.
T25 chỉ nhận `publicRoutes`, từ chối import/draft `publicPort` mà không tự chuyển
đổi; T30 sở hữu rename references.

[T30 HTTP mapping](api-mapping.md) dẫn tới shared OC-12 contract.
