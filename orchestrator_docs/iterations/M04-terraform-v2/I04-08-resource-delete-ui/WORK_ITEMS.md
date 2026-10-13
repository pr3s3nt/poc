---
id: I04-08-WORK-ITEMS
artifact: implementation-checklist
status: deferred
last_reviewed: 2026-10-13
---

# I04-08 — Work items

Canonical scope/criteria/checks ở [charter](README.md), không định nghĩa lại
product requirements. Tất cả checklist đang pending trừ checkpoint tài liệu
được ghi rõ trong verification; không mark test pass khi chưa chạy.

- [ ] Test direct unauthorized API, foreign 404, no-secret projection và retained dependencies.
- [ ] Persist authorization_mode USER_CONFIRMED vs RUN_CLEANUP, confirmed scope/actor/hash.
- [ ] Không thêm dependencies sau xác nhận, không delete platform, không script giả UI confirmation.
- [ ] Engine fake tests không là proof live deletion; proof thuộc A01.

- [ ] Ghi actual changed code/test paths và planned-test IDs đã được hiện thực.
- [ ] Claude chạy checks; Codex review diff và independently verify tương ứng.
- [ ] Evidence/cleanup/remaining blockers vào dated verification, không chép log vào plan.
- [ ] Commit/push task-only khi checks/review đạt theo AGENTS.md; không force-push.
- [ ] Đóng tmux task hoặc lưu handoff khi bị chặn/hủy; tuân stage approval gate.
