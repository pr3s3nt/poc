---
id: D05
artifact: backlog-item
status: deferred
last_reviewed: 2026-09-22
---

# D05 — Humanitec external deployment lifecycle compatibility

Happy-path product API hiện nhận Score và thực hiện đồng bộ một workload mục
tiêu. Resource provisioning chạy mọi resource-only batch, gần full-style
resource behavior, nhưng không đồng nghĩa toàn deployment lifecycle tương thích
Humanitec.

Các quyết định/khả năng deferred:

- standalone Application-scoped Delta create/get/update/archive API; Humanitec
  Delta còn sửa được và chỉ bị khóa cập nhật sau khi archive;
- deploy bằng `delta_id` hoặc `set_id` và asynchronous status lifecycle;
- content-addressed Deployment Set ID thay UUID riêng với document hash;
- full mode áp dụng toàn desired workload set;
- incremental mode và điều kiện chọn resource cần reprovision;
- migration/compatibility contract cho client hiện tại.

MVP không giả lập lifecycle này bằng một entity cùng tên. Mỗi Deployment lưu
`DeploymentDeltaSnapshot` bất biến, dùng Humanitec-shaped document để audit và
execution. Snapshot shape đã được hiện thực ở I06-06 (IMP-008 đã đóng) và
container resources được giữ tới workload renderer ở I06-07 (IMP-009 đã đóng);
record này không hoãn hai contract đó.
