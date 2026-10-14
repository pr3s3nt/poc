---
id: I04-10-WORK-ITEMS
artifact: implementation-checklist
status: current
last_reviewed: 2026-10-14
---

# I04-10 — Work items

Canonical scope/criteria/checks ở [charter](README.md), không định nghĩa lại
product requirements. Code/checks local đạt theo [biên bản](../../../verification/2026-10-14-i04-10-integration.md);
live proof còn I04-11/12, branch cleanup ghi sau push.

- [x] Preflight/evidence directory ngoài Git, private artifacts giữ khi backend acknowledgement lỗi.
- [x] UI cho mọi business steps, script chỉ preflight/environment prep/cleanup.
- [x] Test supervisor/process isolation bằng low-cost owned fixtures; no autonomous D12 claims.

- [x] Ghi actual changed code/test paths và planned-test IDs đã được hiện thực.
- [x] Claude chạy checks; Codex review diff và independently verify tương ứng.
- [x] Evidence/cleanup/remaining blockers vào dated verification, không chép log vào plan.
- [x] Commit/push task-only khi checks/review đạt theo AGENTS.md; không force-push.
- [x] Xóa nhánh Claude sau merge; lưu handoff cloud khi bị chặn/hủy; giữ điểm duyệt cuối.
