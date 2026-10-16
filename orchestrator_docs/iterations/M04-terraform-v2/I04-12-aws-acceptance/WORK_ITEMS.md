---
id: I04-12-WORK-ITEMS
artifact: implementation-checklist
status: historical
last_reviewed: 2026-10-16
---

# I04-12 — Work items

Canonical scope/criteria/checks ở [charter](README.md), không định nghĩa lại
product requirements. Checklist đã hoàn tất theo bằng chứng execution;
[biên bản cuối](../../../verification/2026-10-16-m04-completion.md).

- [x] Chỉ account/region đã giao; mọi create có run tags/expiry và cleanup handles trước apply.
- [x] Thất bại thì chẩn đoán/sửa/checks; cleanup chưa sạch thì không run tiếp hoặc bàn giao complete.
- [x] Codex review độc lập, cập nhật actual paths/evidence/IMP-021 theo kết quả; commit/push task-only, không force.
- [x] Lưu bàn giao cloud/local/fallback sau hoàn tất/bị chặn; gửi báo cáo bằng chứng cuối, không chờ duyệt (2026-10-15).

- [x] Ghi actual changed code/test paths và planned-test IDs đã được hiện thực.
- [x] Writer chạy kiểm tra trong môi trường có sẵn; Codex review và chạy toàn bộ kiểm tra độc lập, không tính skip của writer là pass.
- [x] Evidence/cleanup/remaining blockers vào dated verification, không chép log vào plan.
- [x] Commit/push task-only khi checks/review đạt theo AGENTS.md; không force-push.
- [x] Xóa nhánh Claude sau merge; lưu handoff khi bị chặn/hủy; gửi báo cáo cuối, không chờ duyệt (2026-10-15).
