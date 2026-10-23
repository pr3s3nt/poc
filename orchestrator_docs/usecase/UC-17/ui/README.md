---
id: UC-17-UI
artifact: use-case-ui-design
status: current
last_reviewed: 2026-10-23
related: UC-17, UC-09
---

# UC-17 UI — Hiệu quả platform

## UX outcome

PE/Admin mở “Hiệu quả platform” từ sidebar và thấy ngay **Toàn tổ chức**
trong 30 ngày. Bảng phân bố cho phép đi sâu vào Application rồi Workload. Bấm
một chỉ số mở chi tiết với cách tính, độ phủ và bản ghi nguồn; từ bản ghi mở
lịch sử triển khai hiện có. Không có tab Dữ liệu. Xuất CSV giữ phạm vi/kỳ đang
xem. Baseline là dialog “Mốc so sánh”.

Thiết kế kế thừa mock đã được người dùng duyệt (commit `1873b1e`, thư mục
`frontend/design/platform-effectiveness/`) và hai phản hồi của issue #12: tổng
quan tổ chức và bỏ tab Dữ liệu. Chỉ desktop web (quyết định người dùng
2026-10-23); không có layout mobile riêng.

## Read in this order

1. [Screens and layout](screens.md)
2. [States](states.md)
3. [API mapping](api-mapping.md)
4. [UC-17 specification](../specification.md)
5. [Shared shell](../../../architecture/ui/README.md)
