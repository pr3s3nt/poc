---
id: I04-12-WORK-ITEMS
artifact: implementation-checklist
status: deferred
last_reviewed: 2026-10-13
---

# I04-12 — Work items

Canonical scope/criteria/checks ở [charter](README.md), không định nghĩa lại
product requirements. Tất cả checklist đang pending trừ checkpoint tài liệu
được ghi rõ trong verification; không mark test pass khi chưa chạy.

- [ ] Chỉ account/region đã giao; mọi create có run tags/expiry và cleanup handles trước apply.
- [ ] Thất bại thì chẩn đoán/sửa/checks; cleanup chưa sạch thì không run tiếp hoặc bàn giao complete.
- [ ] Codex review độc lập, cập nhật actual paths/evidence/IMP-021 theo kết quả; commit/push task-only, không force.
- [ ] Lưu bàn giao cloud sau hoàn tất/bị chặn; gửi báo cáo bằng chứng cuối, không chờ duyệt (2026-10-15).

- [ ] Ghi actual changed code/test paths và planned-test IDs đã được hiện thực.
- [ ] Claude chạy checks; Codex review diff và independently verify tương ứng.
- [ ] Evidence/cleanup/remaining blockers vào dated verification, không chép log vào plan.
- [ ] Commit/push task-only khi checks/review đạt theo AGENTS.md; không force-push.
- [ ] Xóa nhánh Claude sau merge; lưu handoff khi bị chặn/hủy; gửi báo cáo cuối, không chờ duyệt (2026-10-15).
