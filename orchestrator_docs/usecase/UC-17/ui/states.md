---
id: UC-17-UI-STATES
artifact: use-case-ui-states
status: current
last_reviewed: 2026-10-23
---

# UC-17 states

| State | Hiển thị |
|---|---|
| Loading | “Đang tải báo cáo…” (`role=status`, `aria-busy`); giữ bộ lọc. |
| Lỗi tải | “Không thể tải báo cáo.” + “Thử lại”; theo T20 code, không raw error. |
| Không có quyền | Developer không thấy mục/trang (shell gate). API 403 hiển thị “Bạn không có quyền xem báo cáo này.” |
| Application không tồn tại | “Không tìm thấy ứng dụng” + nút về Toàn tổ chức. |
| Chưa có dữ liệu | Thẻ hiển thị “Chưa có dữ liệu” cho `NO_DATA`; số đếm có nguồn hiển thị 0; bảng phân bố “Chưa có công việc trong kỳ này.” CSV vẫn xuất được. |
| Chưa thu thập | Giá trị “Chưa thu thập”, muted; không 0, không “—”. |
| Độ phủ thiếu | Thẻ thời gian hiển thị “Độ phủ x/y” màu cảnh báo khi x < y. |
| Nguồn cleanup không khả dụng | Chỉ trong chi tiết chỉ số và CSV: “Chưa gồm cleanup tài nguyên (nguồn không khả dụng).” |
| Dữ liệu kiểm thử | Nhãn “Dữ liệu kiểm thử” ở header và dòng đầu CSV. |
| Chi tiết đang tải/lỗi | Trong dialog: “Đang tải bản ghi…” / “Không thể tải bản ghi.” + “Thử lại”. |
| Baseline lưu | Nút khóa khi đang gửi; thành công đóng dialog, báo “Đã lưu mốc so sánh.” và tải lại báo cáo. |
| Baseline lỗi | `INVALID_REQUEST` giữ form, nêu field; `STALE_VERSION` “Mốc so sánh vừa được thay đổi. Tải lại để xem phiên bản mới.” + “Tải lại”; lỗi khác giữ form. |
| Đổi phạm vi khi đang tải | Bỏ phản hồi muộn của phạm vi cũ. |
