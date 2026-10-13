---
id: I04-01-WORK-ITEMS
artifact: implementation-checklist
status: current
last_reviewed: 2026-10-13
---

# I04-01 — Work items

Canonical scope/criteria/checks ở [charter](README.md), không định nghĩa lại
product requirements. Tất cả checklist đang pending trừ checkpoint tài liệu
được ghi rõ trong verification; không mark test pass khi chưa chạy.

- [ ] Claude sửa ports/domain/PostgreSQL/state HTTP adapter; key không nằm database hay logs.
- [ ] Test idempotent same-serial writes, wrong-key, deadline không tự unlock và lost owner.
- [ ] Test persisted dependency edges/attempt intent và historical snapshot retention.
- [ ] Codex review diff và independently verify trên PostgreSQL cô lập, cleanup database test.

- [ ] Ghi actual changed code/test paths và planned-test IDs đã được hiện thực.
- [ ] Claude chạy checks; Codex review diff và independently verify tương ứng.
- [ ] Evidence/cleanup/remaining blockers vào dated verification, không chép log vào plan.
- [ ] Commit/push task-only khi checks/review đạt theo AGENTS.md; không force-push.
- [ ] Đóng tmux task hoặc lưu handoff khi bị chặn/hủy; tuân stage approval gate.
