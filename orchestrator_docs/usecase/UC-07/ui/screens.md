---
id: UC-07-UI-SCREENS
artifact: use-case-ui-screens
status: current
last_reviewed: 2026-10-13
related: UC-07, UC-16
---

# UC-07 screens

Reuse [UC-16 screens](../../UC-16/ui/screens.md), not a duplicate workload
editor. Application home visibly identifies the selected Environment. Edit
loads current/pending Score and Save creates pending change only. Delete
confirmation names workload and Environment, explains deferred execution and
allows cancel. Pending deletion offers Undo before deployment.

Preview panel shows update/remove target, resource changes and explicit Deploy
control. Completion shows per-workload result with a linkable Deployment ID
where available and refreshes list/history. Removed workloads disappear only
after the successful current Set commit. Other workloads remain visible and
unchanged. Failed/partial runs retain unfinished changes and explain that a
fresh Preview is required; no automatic rollback or secret output rendering.

## T12 — Nhãn thao tác trên trang ứng dụng

Xóa phải xác nhận tên Workload và `staging`/`production`, giải thích runtime
chỉ đổi sau Triển khai. Nhãn Chờ xóa và Hoàn tác dùng tiếng Việt. Xem trước
và kết quả dịch hành động/trạng thái, giữ token, IDs, per-workload retry và
links lịch sử hiện hành. Partial/failure yêu cầu xem trước mới, không tự rollback
hoặc hiển thị raw lỗi có thể chứa bí mật.

## Xóa tài nguyên tại resource detail

Từ Environment mở chi tiết tài nguyên theo UC-09. PE/Admin có nút “Xóa tài
nguyên”; Developer không thấy nút. Root không UNREFERENCED, còn consumer,
platform, thiếu metadata hoặc Environment busy thì khóa và nêu lý do.

Khi bấm, tải deletion preview. Hộp thoại có Application/Environment/generation,
tên kỹ thuật root và danh sách root/dependency cùng Environment/generation.
Chỉ root được tick mặc định, dependency để trống; không hiển thị platform.
Người dùng tự tick thêm; khóa mục có consumer ngoài tập đã tick và nêu lý do.
Bỏ tick tự bỏ tick các mục trở nên không hợp lệ, kèm giải thích; không tự tick
thêm. Phần xác nhận chỉ liệt kê đúng các mục đã tick. Cảnh báo **Xóa vĩnh viễn dữ liệu, không thể hoàn tác**; ô “Nhập lại tên
tài nguyên” phải khớp tên server hiển thị, không suy từ tên người dùng nhập.
Không gõ đúng hoặc chưa tải/validate scope thì nút xác nhận bị khóa.

Hủy không gọi destroy. Xác nhận gửi pinned token + tên + selectedResourceIds; đổi scope/versions thì
đóng hoặc invalidate hộp thoại, yêu cầu review mới. UI theo dõi operation đã
nhận qua REQUESTED/CHECKING/DESTROYING/VERIFYING, kết quả từng resource và
remaining IDs. Success chỉ sau absence verification; không có Undo dữ liệu.
Cleanup script độc lập là safety net, không thay thao tác UI trong acceptance.
