---
id: I04-01-WORK-ITEMS
artifact: implementation-checklist
status: historical
last_reviewed: 2026-10-13
---

# I04-01 — Work items

Canonical scope/criteria/checks ở [charter](README.md), không định nghĩa lại
product requirements. Tất cả checklist đang pending trừ checkpoint tài liệu
được ghi rõ trong verification; không mark test pass khi chưa chạy.

- [x] Claude sửa ports/domain/PostgreSQL/state HTTP adapter; key không nằm database hay logs.
- [x] Test idempotent same-serial writes, wrong-key, deadline không tự unlock và lost owner.
- [x] Test persisted dependency edges/attempt intent và historical snapshot retention.
- [x] Codex review diff và independently verify trên PostgreSQL cô lập, cleanup database test.

- [x] Ghi actual changed code/test paths và planned-test IDs đã được hiện thực.
- [x] Claude chạy checks; Codex review diff và independently verify tương ứng.
- [x] Evidence/cleanup/remaining blockers vào dated verification, không chép log vào plan.
- [ ] Commit/push task-only khi checks/review đạt theo AGENTS.md; không force-push.
- [ ] Đóng tmux task hoặc lưu handoff khi bị chặn/hủy; tuân stage approval gate.

- [x] Owner setup + hai runtime logins và permission-denial tests trên PostgreSQL thật.
- [ ] Người dùng chốt phạm vi TLS ngoài loopback; implement/validate phần bị ảnh hưởng.

Core/role-A checkpoint đã verified; chưa coi toàn bộ iteration complete.

## Kết thúc

I04-01 hoàn tất sau final corrections; [835 mandatory SQL tests + cleanup](../../../verification/2026-10-13-i04-01-complete.md). TLS deferred, không blocker. Chuyển I04-02. Các handoff trước là lịch sử.
