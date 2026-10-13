---
id: I04-03-WORK-ITEMS
artifact: implementation-checklist
status: current
last_reviewed: 2026-10-13
---

# I04-03 — Work items

Canonical scope/criteria/checks ở [charter](README.md), không định nghĩa lại
product requirements. Tất cả checklist đang pending trừ checkpoint tài liệu
được ghi rõ trong verification; không mark test pass khi chưa chạy.

- [ ] Resolve provider theo scoped Connection/EKS identity, không host env fallback.
- [ ] Persist stages trước/sau call; actual state acknowledgement trước READY.
- [ ] Destroy original module/target/state, reverse graph, physical absence checks; giữ incomplete IDs.
- [ ] Không implement D12/D13/D14 hoặc claim full resume.

- [ ] Ghi actual changed code/test paths và planned-test IDs đã được hiện thực.
- [ ] Claude chạy checks; Codex review diff và independently verify tương ứng.
- [ ] Evidence/cleanup/remaining blockers vào dated verification, không chép log vào plan.
- [ ] Commit/push task-only khi checks/review đạt theo AGENTS.md; không force-push.
- [ ] Đóng tmux task hoặc lưu handoff khi bị chặn/hủy; tuân stage approval gate.
