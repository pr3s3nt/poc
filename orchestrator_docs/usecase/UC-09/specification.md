---
id: UC-09-SPEC
artifact: use-case-specification
status: current
last_reviewed: 2026-10-13
---

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
- **BR-05:** Console liệt kê Deployment và detail trong đúng Environment;
  detail gồm graph, module/revision/target, Terraform stage/attempt/status/
  timestamps và failure reason. Poll persisted query khi đang chạy; không
  gọi provider để refresh hoặc thêm raw log/streaming console.
- **BR-06:** Nếu UC-06 thất bại trong planning trước khi tạo Delta Snapshot,
  Deployment view vẫn trả status/failure reason; Delta, graph và batches có thể
  vắng mặt. Không dựng Delta giả hoặc hiển thị plan của lần khác.
- **BR-07:** Lỗi hiển thị safe code/summary đúng node/stage, gồm input/
  permission/transient/interrupted/cleanup. Không raw CLI/state/tfvars/plan
  JSON/credential/secret. History có raw reason cũ được che trong query, không
  viết lại history. FAILED không đồng nghĩa đã cleanup.

## Luồng nội bộ

```text
UC-09 View Deployment Status
├── Load Deployment record
├── Load Deployment Set and Resource Graph
├── Load workload and resource statuses
├── Filter secret information
└── Return deployment view
```

## Ngoài phạm vi happy path

- **OOS-01:** Streaming hoặc real-time status updates.
- **OOS-02:** Driver và Kubernetes logs.
- **OOS-03:** So sánh hai deployments.
- **OOS-04:** Alerting và notifications.
- **OOS-05:** Metrics, tracing và audit views.
- **OOS-06:** Tùy chỉnh quyền xem outputs theo RBAC.


## Chi tiết tài nguyên và tiến trình xóa

- **RES-01:** Resource detail thuộc đúng Organization/Application/Environment/
  generation, gồm tên kỹ thuật (descriptor), trạng thái, module/target/owner,
  safe outputs, consumers và eligibility. Có thể đọc resource UNREFERENCED
  từ lịch sử sau khi Workload đã gỡ; không cần nó còn trong current graph.
- **RES-02:** Nút Xóa tài nguyên chỉ hiện cho PE/Admin. Eligibility phải có
  dữ liệu kiểm chứng; chưa tải/lỗi/missing metadata không được coi là đủ điều
  kiện. Root có consumer, không UNREFERENCED, platform hoặc Environment busy
  thì khóa nút và nêu lý do. Luồng mutation do UC-07 DEL-* sở hữu.
- **RES-03:** Trang detail/progress hiển thị REQUESTED/CHECKING/DESTROYING/
  VERIFYING, từng resource trong phạm vi đã xác nhận, kết quả absence check và
  remaining physical IDs khi lỗi. Poll scoped persisted operation; không báo
  success trước VERIFYING hoàn tất hoặc lấy trạng thái operation khác.
