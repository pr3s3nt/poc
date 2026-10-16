---
id: I04-11-WORK-ITEMS
artifact: implementation-checklist
status: historical
last_reviewed: 2026-10-16
---

# I04-11 — Work items

Canonical scope/criteria/checks ở [charter](README.md), không định nghĩa lại
product requirements. Checklist đã hoàn tất theo bằng chứng execution;
[biên bản cuối](../../../verification/2026-10-16-m04-completion.md).

- [x] Thất bại thì chẩn đoán/sửa/checks; cleanup lần cũ sạch trước lượt mới.
- [x] Không thêm fake fallback hoặc giảm assertions khi live fail.
- [x] Handoff video A01/E01/E02, SQL validation/cleanup proof và estimate; tự sang I04-12 khi pass và k8s-4f sạch.

- [x] Ghi actual changed code/test paths và planned-test IDs đã được hiện thực.
- [x] Writer chạy kiểm tra trong môi trường có sẵn; Codex review và chạy toàn bộ kiểm tra độc lập, không tính skip của writer là pass.
- [x] Evidence/cleanup/remaining blockers vào dated verification, không chép log vào plan.
- [x] Commit/push task-only khi checks/review đạt theo AGENTS.md; không force-push.
- [x] Xóa nhánh Claude sau merge; lưu handoff khi bị chặn/hủy; gửi báo cáo cuối, không chờ duyệt (2026-10-15).
