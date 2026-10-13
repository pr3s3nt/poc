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
- [x] Commit/push task-only khi checks/review đạt theo AGENTS.md; không force-push.
- [x] Đóng tmux task hoặc lưu handoff khi bị chặn/hủy; tuân stage approval gate.

- [x] Owner setup + hai runtime logins và permission-denial tests trên PostgreSQL thật.
- [x] Người dùng deferred TLS theo issue #2; contract/docs khớp, không cần code change.

Core/role-A checkpoint trước đây chưa đóng task; final corrections ở dưới
đã hoàn tất và commit/push tại `04fda0f`.

## Kết thúc

I04-01 hoàn tất sau final corrections; [835 mandatory SQL tests + cleanup](../../../verification/2026-10-13-i04-01-complete.md). TLS deferred, không blocker. Chuyển I04-02. Các handoff trước là lịch sử.
