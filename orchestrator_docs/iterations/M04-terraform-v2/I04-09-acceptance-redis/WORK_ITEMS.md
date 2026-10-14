---
id: I04-09-WORK-ITEMS
artifact: implementation-checklist
status: current
last_reviewed: 2026-10-14
---

# I04-09 — Work items

Canonical scope/criteria/checks ở [charter](README.md), không định nghĩa lại
product requirements. Tất cả checklist đang pending trừ checkpoint tài liệu
được ghi rõ trong verification; không mark test pass khi chưa chạy.

- [ ] TTL/prefix đủ deterministic assertions; không log credential/secret data.
- [ ] Build three images với immutable tag/digest cho real scenarios.
- [ ] Unit/CLI không dựng video giả; browser app flow nằm trong A01/A02/A03.

- [ ] Ghi actual changed code/test paths và planned-test IDs đã được hiện thực.
- [ ] Claude chạy checks; Codex review diff và independently verify tương ứng.
- [ ] Evidence/cleanup/remaining blockers vào dated verification, không chép log vào plan.
- [ ] Commit/push task-only khi checks/review đạt theo AGENTS.md; không force-push.
- [ ] Xóa nhánh Claude sau merge; lưu handoff cloud khi bị chặn/hủy; giữ điểm duyệt cuối.
