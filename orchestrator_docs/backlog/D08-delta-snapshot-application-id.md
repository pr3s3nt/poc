---
id: D08
artifact: backlog-item
status: deferred
last_reviewed: 2026-09-22
---

# D08 — Ownership của Application identity trên Deployment Delta Snapshot

Cột `deployment_delta_snapshots.application_id` trong
[database schema](../architecture/database/schema.md) là dữ liệu có thể suy ra
qua `Deployment -> Environment -> Application`. FK
`deployments.delta_snapshot_id NOT NULL UNIQUE` bảo đảm mỗi Deployment tham
chiếu đúng một Snapshot và một Snapshot được tham chiếu bởi tối đa một
Deployment; nó không bắt buộc mọi Snapshot phải được Deployment tham chiếu.

Domain/transaction hiện có ý định quan hệ một-một, nhưng schema vẫn có thể chứa
Snapshot orphan hoặc `application_id` không khớp Application suy ra từ
Deployment. Cột này có thể là denormalization có chủ ý cho tenant filtering,
partitioning hoặc row-level security, nhưng rationale và integrity mechanism
chưa được quyết định.

## Deferred decision

Khi thực hiện, chọn một hướng rồi cập nhật schema, ERD, domain model và
persistence tests:

- chuẩn hóa: bỏ `application_id`, suy ra Application qua Deployment và cân nhắc
  để Snapshot giữ `deployment_id` unique FK nhằm enforce ownership trực tiếp;
- denormalize có chủ ý: giữ `application_id`, ghi rõ query/security rationale và
  chọn cơ chế toàn vẹn khả thi như composite foreign keys sau khi mang tenant
  key qua các bảng liên quan, hoặc database trigger. CHECK constraint thông
  thường không thể tự so sánh dữ liệu qua các bảng.

Quyết định phải bao gồm orphan policy, delete behavior và test chứng minh
Snapshot không thể thuộc sai Application.
