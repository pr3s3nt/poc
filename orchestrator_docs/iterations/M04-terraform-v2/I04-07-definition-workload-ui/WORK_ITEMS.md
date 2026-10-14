---
id: I04-07-WORK-ITEMS
artifact: implementation-checklist
status: historical
last_reviewed: 2026-10-14
---

# I04-07 — Work items

Canonical scope/criteria/checks ở [charter](README.md), không định nghĩa lại
product requirements. Kết quả và giới hạn evidence ở [biên bản](../../../verification/2026-10-14-i04-07-definition-workload-ui.md).

- [x] Giữ labels/groups/Service refs/routes/key aliases, no implicit Save/Deploy.
- [x] Registry metadata authoritative, no hardcoded runtime-support fallback.
- [x] Giữ 33 reference fixtures read-only; v2 tests riêng, không dùng harness bypass.
- [x] Fake adapters chỉ dùng local code tests, không được tính live acceptance.

- [x] Ghi actual changed code/test paths và planned-test IDs đã được hiện thực.
- [x] Claude chạy checks; Codex review diff và independently verify tương ứng.
- [x] Evidence/cleanup/remaining blockers vào dated verification, không chép log vào plan.
- [x] Commit/push task-only khi checks/review đạt theo AGENTS.md; không force-push.
- [x] Xóa nhánh Claude sau merge; lưu handoff cloud khi bị chặn/hủy; giữ điểm duyệt cuối.
