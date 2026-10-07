---
id: D09
artifact: backlog-item
status: deferred
last_reviewed: 2026-10-07
related: [UC-01, UC-03, UC-04, UC-06, UC-08]
---

# D09 — Application cluster Connection ownership

## Problem

Luồng triển khai lên Kubernetes có hai cấp có thể chọn cluster đích:
Application giữ Connection lấy từ Organization default khi tạo; Resource
Definition có thể giữ Connection riêng. Executor ưu tiên Connection của
Definition và chỉ fallback về Application khi Definition không có Connection.
Vì vậy Application và Definition `existing-cluster` có thể chỉ đến hai cluster
khác nhau, khiến lựa chọn cluster của app khó hiểu và khó kiểm soát.

Đăng ký Connection mới không tự đổi Organization default. UI tạo Application
chưa cho chọn Connection; đổi default chỉ ảnh hưởng Application tạo sau đó.
Trong lần chuẩn bị test trên cluster công ty, người dùng phải cập nhật database
để đổi default và Connection của Application chưa từng deploy. Đây là triệu
chứng của luồng chọn đích chưa đầy đủ, không phải runbook triển khai chuẩn.

## Current references

- [Connection consumption and compatibility](../architecture/connection-credentials.md).
- [UC-01 specification](../usecase/UC-01/specification.md).
- [Application creation](../../backend/internal/application/application/service.go).
- [Resource provisioning Connection selection](../../backend/internal/application/provisioning/service.go).

## Follow-up to evaluate

- Chốt nguồn có thẩm quyền duy nhất chọn cluster chạy workload của Application.
- Đánh giá phương án Organization default chỉ khởi tạo/gợi ý lựa chọn,
  Application Connection sở hữu cluster đích, và Definition `existing-cluster`
  dùng lựa chọn đó hoặc reject mismatch thay vì âm thầm override.
- Phân biệt Connection cho cluster workload với Connection của resource/provider
  bên ngoài; không loại bỏ khả năng provider riêng khi chưa đánh giá use cases.
- Hoàn thiện UI/API chọn Connection `READY` thuộc Organization khi tạo app.
- Quyết định chính sách đổi Connection cho app chưa deploy và app đã có
  resources/workloads, migration/backfill và khả năng tương thích.
- Sau khi chốt intent, cập nhật architecture/ADR, affected specifications,
  schema nếu cần, planner/executor/route/removal contracts và tests/traceability.

## Exit criteria

Quyết định được chấp nhận và triển khai nhất quán: người dùng xác định được
cluster đích trước Deploy; provisioning, workload, route và removal dùng đúng
execution identity; cấu hình mâu thuẫn bị xử lý theo policy rõ ràng. Có tests
cho default lúc tạo, explicit selection, Definition conflict và lifecycle đổi
Connection. Không cần sửa database thủ công cho luồng chọn cluster thông thường.

Các phương án trên chưa phải accepted requirement; record này chỉ ghi vấn đề
được chủ ý hoãn, không thay đổi behavior hiện tại.
