---
id: I04-00-WORK-ITEMS
artifact: implementation-checklist
status: current
last_reviewed: 2026-10-13
---

# I04-00 — Work items

Canonical scope/criteria/checks ở [charter](README.md), không định nghĩa lại
product requirements. Tất cả checklist đang pending trừ checkpoint tài liệu
được ghi rõ trong verification; không mark test pass khi chưa chạy.

- [x] Review ID mapping và canonical destinations ở reconciliation, a897d6b/3367f7e history.
- [x] Render PNG và rà stale references toàn repository.
- [x] Lưu verification/handoff; dừng chờ duyệt chặng 1.

- [ ] Ghi actual changed code/test paths và planned-test IDs đã được hiện thực.
- [ ] Claude chạy checks; Codex review diff và independently verify tương ứng.
- [ ] Evidence/cleanup/remaining blockers vào dated verification, không chép log vào plan.
- [ ] Commit/push task-only khi checks/review đạt theo AGENTS.md; không force-push.
- [ ] Đóng tmux task hoặc lưu handoff khi bị chặn/hủy; tuân stage approval gate.

- [ ] Người dùng duyệt chặng 1; chỉ sau đó mới activate I04-01 và giao code.
