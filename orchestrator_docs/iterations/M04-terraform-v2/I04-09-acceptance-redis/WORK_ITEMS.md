---
id: I04-09-WORK-ITEMS
artifact: implementation-checklist
status: historical
last_reviewed: 2026-10-14
---

# I04-09 — Work items

Canonical scope/criteria/checks ở [charter](README.md), không định nghĩa lại
product requirements. Checklist local đã verified theo [biên bản](../../../verification/2026-10-14-i04-09-acceptance-redis.md); live browser thuộc I04-11/12.

- [x] TTL/prefix đủ deterministic assertions; không log credential/secret data.
- [x] Build three images với immutable tag/digest cho real scenarios.
- [x] Unit/CLI không dựng video giả; browser app flow nằm trong A01/A02/A03.

- [x] Ghi actual changed code/test paths và planned-test IDs đã được hiện thực.
- [x] Claude chạy checks; Codex review diff và independently verify tương ứng.
- [x] Evidence/cleanup/remaining blockers vào dated verification, không chép log vào plan.
- [x] Commit/push task-only khi checks/review đạt theo AGENTS.md; không force-push.
- [x] Xóa nhánh Claude sau merge; lưu handoff cloud khi bị chặn/hủy; giữ điểm duyệt cuối.
