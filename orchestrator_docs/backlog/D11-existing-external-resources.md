---
id: D11
artifact: backlog-item
status: deferred
last_reviewed: 2026-10-13
related: [UC-02, UC-03, UC-04, UC-08, UC-12]
---

# D11 — Echo/External: dùng tài nguyên có sẵn sau v2

## Scope và lý do hoãn

Người dùng quyết định hoãn cơ chế Echo/External để v2 tập trung Terraform
tạo và quản lý tài nguyên, score-k8s sinh manifest workload và delivery triển
khai ứng dụng. [Custom driver](D10-custom-resource-drivers.md) cũng hoãn sau v2.

Nhu cầu tương lai là đăng ký Definition cung cấp outputs và secret references
cho database, cache hoặc API đã tồn tại, do bên khác quản lý. Dịch vụ có thể
truy cập qua mạng cluster, private hoặc Internet; vị trí mạng không xác định
ownership hoặc bắt buộc một loại driver.

## Nội dung cần quyết định khi mở task

- Contract outputs, secret references, validation và matching Definition.
- UI đăng ký dịch vụ có sẵn và gắn bindings vào workload.
- Ownership chỉ tham chiếu: gỡ binding/ứng dụng không xóa dịch vụ bên ngoài.
- Phân biệt metadata hợp lệ với connectivity/authentication thực tế.
- Phạm vi kiểm thử và quyền cleanup đối với fixture do test tự tạo.

## Ranh giới v2

Không triển khai Echo/External driver, lựa chọn UI "Dùng dịch vụ có sẵn" hoặc
acceptance riêng cho cơ chế này trong v2. Bỏ bước acceptance chọn lại dịch vụ
qua Echo/External đã được đề xuất trước đó. Happy path Terraform trên k8s-4f,
AWS managed và AWS kết hợp vẫn nằm trong kế hoạch; giữ hai ngoại lệ đã chốt.

Việc dùng Connection k8s-4f có sẵn và kế hoạch tiếp quản tài nguyên v1 là các
phạm vi riêng, không bị loại bỏ bởi quyết định hoãn generic Echo/External.

## Điều kiện hoàn thành dự kiến

Sau khi chốt ADR/specification, người dùng gắn được tài nguyên có sẵn qua UI;
workload nhận đúng outputs/secret bindings và dịch vụ bên ngoài còn nguyên
khi gỡ binding hoặc ứng dụng. Record này không thay đổi behavior hiện tại và
không cho phép chạy external verification.
