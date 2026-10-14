---
id: I04-11-WORK-ITEMS
artifact: implementation-checklist
status: deferred
last_reviewed: 2026-10-13
---

# I04-11 — Work items

Canonical scope/criteria/checks ở [charter](README.md), không định nghĩa lại
product requirements. Tất cả checklist đang pending trừ checkpoint tài liệu
được ghi rõ trong verification; không mark test pass khi chưa chạy.

- [ ] Thất bại thì chẩn đoán/sửa/checks; cleanup lần cũ sạch trước lượt mới.
- [ ] Không thêm fake fallback hoặc giảm assertions khi live fail.
- [ ] Handoff video A01/E01/E02, SQL validation/cleanup proof và estimate; tự sang I04-12 khi pass và k8s-4f sạch.

- [ ] Ghi actual changed code/test paths và planned-test IDs đã được hiện thực.
- [ ] Claude chạy checks; Codex review diff và independently verify tương ứng.
- [ ] Evidence/cleanup/remaining blockers vào dated verification, không chép log vào plan.
- [ ] Commit/push task-only khi checks/review đạt theo AGENTS.md; không force-push.
- [ ] Xóa nhánh Claude sau merge; lưu handoff khi bị chặn/hủy; gửi báo cáo cuối, không chờ duyệt (2026-10-15).
