---
id: I04-02-WORK-ITEMS
artifact: implementation-checklist
status: historical
last_reviewed: 2026-10-13
---

# I04-02 — Work items

Canonical scope/criteria/checks ở [charter](README.md), không định nghĩa lại
product requirements. Kết quả ở
[verification](../../../verification/2026-10-13-i04-02-module-registry.md).

- [x] Giữ insert-only org/key và năm criteria weights.
- [x] Publish source/module/version/schema metadata an toàn; không raw source/cache path.
  Class metadata/UI còn I04-07.
- [x] Test stale module pin, no-match/ties/output-secret mapping và no silent upgrade.

- [x] Ghi actual changed code/test paths và planned-test IDs đã được hiện thực.
- [x] Claude chạy checks; Codex review diff và independently verify tương ứng.
- [x] Evidence/cleanup/remaining blockers vào dated verification, không chép log vào plan.
- [x] Commit/push task-only khi checks/review đạt theo AGENTS.md; không force-push.
- [x] Đóng tmux task hoặc lưu handoff khi bị chặn/hủy; tuân stage approval gate.
