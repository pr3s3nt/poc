---
id: I04-06-WORK-ITEMS
artifact: implementation-checklist
status: historical
last_reviewed: 2026-10-16
---

# I04-06 — Work items

Canonical scope/criteria/checks ở [charter](README.md), không định nghĩa lại
product requirements. Kết quả actual ở [biên bản](../../../verification/2026-10-14-i04-06-aws-connection.md).

- [x] Kubernetes upload/store behavior giữ nguyên; AWS form không nhận backend config.
- [x] Permission verification read-only, không AWS provision.
- [x] Kiểm duplicate/scoping/failed credential cleanup và server role gate.

- [x] Ghi actual changed code/test paths và planned-test IDs đã được hiện thực.
- [x] Claude chạy checks; Codex review diff và independently verify tương ứng.
- [x] Evidence/cleanup/remaining blockers vào dated verification, không chép log vào plan.
- [x] Commit/push task-only khi checks/review đạt theo AGENTS.md; không force-push.
- [x] Xóa cả nhánh Claude chính/tự sinh sau push merge; checkout riêng đã dọn; không chờ duyệt cuối theo quyết định2026-10-15.
