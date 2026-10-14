---
id: I04-08-WORK-ITEMS
artifact: implementation-checklist
status: historical
last_reviewed: 2026-10-14
---

# I04-08 — Work items

Canonical scope/criteria/checks ở [charter](README.md), không định nghĩa lại
product requirements. Kết quả/giới hạn ở [biên bản](../../../verification/2026-10-14-i04-08-resource-delete-ui.md); mục live còn I04-11.

- [x] Test direct unauthorized API, foreign 404, no-secret projection và retained dependencies.
- [x] Persist authorization_mode USER_CONFIRMED vs RUN_CLEANUP, confirmed scope/actor/hash.
- [x] Không thêm dependencies sau xác nhận, không delete platform, không script giả UI confirmation.
- [x] Engine fake tests không là proof live deletion; proof thuộc A01.

- [x] Ghi actual changed code/test paths và planned-test IDs đã được hiện thực.
- [x] Claude chạy checks; Codex review diff và independently verify tương ứng.
- [x] Evidence/cleanup/remaining blockers vào dated verification, không chép log vào plan.
- [x] Commit/push task-only khi checks/review đạt theo AGENTS.md; không force-push.
- [x] Xóa nhánh Claude sau merge; lưu handoff cloud khi bị chặn/hủy; giữ điểm duyệt cuối.

- [x] Root-only default; dependency checkbox blockers và cascading unchecks có giải thích.
- [x] Backend chỉ xóa selected set; atomic reject cả tập sai, không thêm dependency.
- [ ] A01 live tick thêm ít nhất một dependency và assert unchecked còn: I04-11; local fake headed proof đã đạt.
