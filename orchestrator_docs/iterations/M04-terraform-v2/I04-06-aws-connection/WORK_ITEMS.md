---
id: I04-06-WORK-ITEMS
artifact: implementation-checklist
status: deferred
last_reviewed: 2026-10-13
---

# I04-06 — Work items

Canonical scope/criteria/checks ở [charter](README.md), không định nghĩa lại
product requirements. Tất cả checklist đang pending trừ checkpoint tài liệu
được ghi rõ trong verification; không mark test pass khi chưa chạy.

- [ ] Kubernetes upload/store behavior giữ nguyên; AWS form không nhận backend config.
- [ ] Permission verification read-only, không AWS provision.
- [ ] Kiểm duplicate/scoping/failed credential cleanup và server role gate.

- [ ] Ghi actual changed code/test paths và planned-test IDs đã được hiện thực.
- [ ] Claude chạy checks; Codex review diff và independently verify tương ứng.
- [ ] Evidence/cleanup/remaining blockers vào dated verification, không chép log vào plan.
- [ ] Commit/push task-only khi checks/review đạt theo AGENTS.md; không force-push.
- [ ] Xóa nhánh Claude sau merge; lưu handoff cloud khi bị chặn/hủy; giữ điểm duyệt cuối.
