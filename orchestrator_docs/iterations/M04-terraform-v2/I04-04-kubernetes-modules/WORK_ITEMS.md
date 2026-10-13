---
id: I04-04-WORK-ITEMS
artifact: implementation-checklist
status: deferred
last_reviewed: 2026-10-13
---

# I04-04 — Work items

Canonical scope/criteria/checks ở [charter](README.md), không định nghĩa lại
product requirements. Tất cả checklist đang pending trừ checkpoint tài liệu
được ghi rõ trong verification; không mark test pass khi chưa chạy.

- [ ] Ghi release manifest với exact binary/provider/module digests.
- [ ] Redis auth và tls output theo contract; PostgreSQL database/username params giữ schema.
- [ ] Validation/plan fixtures không thay live acceptance; không mutate k8s-4f ở task chuẩn bị.

- [ ] Ghi actual changed code/test paths và planned-test IDs đã được hiện thực.
- [ ] Claude chạy checks; Codex review diff và independently verify tương ứng.
- [ ] Evidence/cleanup/remaining blockers vào dated verification, không chép log vào plan.
- [ ] Commit/push task-only khi checks/review đạt theo AGENTS.md; không force-push.
- [ ] Đóng tmux task hoặc lưu handoff khi bị chặn/hủy; tuân stage approval gate.
