# UC-09 — View Deployment Status and Resource Graph

## Mục tiêu

Cho phép Developer xem trạng thái deployment, Resource Graph và thông tin resource của một Environment.

## Primary actor

- Developer

## Supporting actor

- Platform Engineer

## Tiền điều kiện

- **PRE-01:** Application và Environment đã tồn tại.
- **PRE-02:** Có ít nhất một Deployment record.
- **PRE-03:** Actor có quyền xem Environment.

## Trigger

**TRG-01:** Developer mở hoặc truy vấn thông tin một Deployment trong Environment.

## Main success scenario

1. **MS-01:** Developer chọn Application, Environment và Deployment.
2. **MS-02:** Orchestrator đọc Deployment record và Deployment Set tương ứng.
3. **MS-03:** Orchestrator đọc persisted workload/resource status và Active Resources của Deployment.
4. **MS-04:** Orchestrator dựng read model gồm trạng thái tổng thể, thời gian và actor kích hoạt.
5. **MS-05:** Orchestrator thêm Resource Graph, matched Definitions và provision batches vào read model.
6. **MS-06:** Orchestrator lọc secret outputs rồi thêm Active Resources cùng non-secret outputs.
7. **MS-07:** Orchestrator trả deployment view cho Developer.

## Hậu điều kiện

- **POST-01:** Không có trạng thái runtime hoặc persisted state nào bị thay đổi.
- **POST-02:** Developer có đủ thông tin để xác nhận deployment happy path thành công.
- **POST-03:** Thông tin secret không xuất hiện trong response.

## Quy tắc nghiệp vụ

- **BR-01:** UC-09 chỉ đọc snapshot đã persist; không tự gọi provision/apply để refresh runtime.
- **BR-02:** Deployment view phải gắn đúng Application, Environment và Deployment ID.
- **BR-03:** Chỉ output được phân loại non-secret mới được hiển thị.
- **BR-04:** Graph, matched Definition và batches phải là artifact của đúng lần planning đã tạo Deployment đó.

## Luồng nội bộ

```text
UC-09 View Deployment Status
├── Load Deployment record
├── Load Deployment Set and Resource Graph
├── Load workload and resource statuses
├── Filter secret information
└── Return deployment view
```

## Trạng thái implementation hiện tại

- Deployment, plan snapshot, resource progress, Active Resources và Workload Instances đã được lưu trong state store; query API trả status, graph, batches, resources, workloads và redacted outputs.
- Web Console đã có Deployment Details và live read-only verification trên kind/AWS deployment.
- History/filter/comparison đầy đủ và PostgreSQL read model chưa hoàn thiện; đây là Phase 6 bước 4.

## Ngoài phạm vi happy path

- **OOS-01:** Streaming hoặc real-time status updates.
- **OOS-02:** Driver và Kubernetes logs.
- **OOS-03:** So sánh hai deployments.
- **OOS-04:** Alerting và notifications.
- **OOS-05:** Metrics, tracing và audit views.
- **OOS-06:** Tùy chỉnh quyền xem outputs theo RBAC.
