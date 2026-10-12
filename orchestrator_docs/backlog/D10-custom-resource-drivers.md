---
id: D10
artifact: backlog-item
status: deferred
last_reviewed: 2026-10-13
related: [UC-02, UC-03, UC-04, UC-08]
---

# D10 — Custom Resource Drivers sau v2

## Scope và lý do hoãn

V2 tập trung Terraform để tạo và quản lý tài nguyên trên Kubernetes, cloud
hoặc dịch vụ có provider phù hợp. Echo/External cũng được hoãn sau v2, theo
[D11](D11-existing-external-resources.md). Đây là phạm vi kế hoạch, không phải
tuyên bố capability đã được triển khai.

Custom driver được chủ ý hoãn sau v2. Nhu cầu là tích hợp hệ thống có API riêng
để tạo/quản lý tài nguyên khi Echo/External và Terraform chưa đáp ứng, đồng thời
trả outputs theo Resource Type contract cho cùng luồng bindings/rendering.
Tài nguyên truy cập qua Internet không tự động đòi hỏi custom driver.

## Nội dung cần quyết định khi mở task

- Contract đăng ký và gọi driver, phiên bản, inputs/outputs và lỗi.
- Cách chạy driver: endpoint hay runner/plugin; chưa chọn cơ chế cụ thể.
- Authentication, credential/secret boundary và phạm vi truy cập.
- Ownership và lifecycle create/update/delete, idempotency, retry và timeout.
- Matching, metadata/UI và tương thích với graph, state và resource bindings.

## Điều kiện hoàn thành dự kiến

Sau khi chốt ADR/specification, đăng ký và sử dụng được một tích hợp cụ thể
không cần thêm logic theo loại tài nguyên vào core backend; outputs tuân thủ
contract và lifecycle/cleanup tuân thủ ownership đã chốt. Phạm vi acceptance
của hạng mục này sẽ được quyết định riêng, không mở rộng acceptance v2.

## Tham chiếu

- [D03 — Remote Terraform execution](D03-remote-terraform-runtime.md): cơ chế
  Terraform là hạng mục riêng, không phụ thuộc việc triển khai custom driver.
- [Humanitec custom drivers](https://developer.humanitec.com/platform-orchestrator/docs/integration-and-extensions/drivers/custom-drivers/register-custom-drivers/):
  tài liệu tham khảo, không cam kết tương thích API Humanitec.

Record này không cho phép triển khai custom driver hoặc chạy external verification.
