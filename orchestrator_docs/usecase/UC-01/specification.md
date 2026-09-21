# UC-01 — Manage Application and Environment

## Mục tiêu

Tạo Application, chọn Execution Profile và quản lý các Environment dùng để thực hiện deployment.

## Primary actor

- Platform Engineer

## Supporting actor

- Organization Administrator

## Tiền điều kiện

- **PRE-01:** Organization đã tồn tại.
- **PRE-02:** Platform Engineer có quyền quản lý Application và Environment.
- **PRE-03:** Một trong hai Execution Profile được hỗ trợ: `aws-eks` hoặc `internal-k8s`.

## Trigger

**TRG-01:** Platform Engineer yêu cầu tạo một Application và Environment.

## Main success scenario

1. **MS-01:** Platform Engineer tạo Application với ID và tên duy nhất.
2. **MS-02:** Platform Engineer chọn Execution Profile `aws-eks` hoặc `internal-k8s`.
3. **MS-03:** Platform Engineer chọn connection phù hợp với profile theo VAR-01 hoặc VAR-02.
4. **MS-04:** Orchestrator lưu Application cùng runtime configuration; Cloud runtime ban đầu có trạng thái `PENDING`.
5. **MS-05:** Platform Engineer tạo Environment thuộc Application.
6. **MS-06:** Orchestrator gán Environment Type phù hợp với Execution Profile và khởi tạo Deployment Set rỗng.
7. **MS-07:** Orchestrator tạo namespace identity theo Environment.
8. **MS-08:** Orchestrator lưu Environment và trả về thông tin đã tạo.

## Luồng biến thể trong happy path

- **VAR-01 — `aws-eks`:** tại MS-03, Platform Engineer chọn AWS region và Driver Account đã cấu hình trong UC-04; chưa tạo VPC/EKS.
- **VAR-02 — `internal-k8s`:** tại MS-03, Platform Engineer chọn cluster connection trạng thái `READY` đã đăng ký trong UC-04.

## Hậu điều kiện

- **POST-01:** Application có một Execution Profile cố định.
- **POST-02:** Với Cloud, Application ở trạng thái chờ lazy-provision VPC/EKS trong UC-06/UC-08 đầu tiên.
- **POST-03:** Với Internal, Application tham chiếu một Kubernetes cluster có sẵn.
- **POST-04:** Mỗi Environment có Deployment Set rỗng và namespace identity riêng.
- **POST-05:** Application và Environment sẵn sàng được sử dụng trong UC-06.

## Quy tắc nghiệp vụ

- **BR-01:** Application thuộc đúng một Organization và Application ID là duy nhất trong Organization.
- **BR-02:** Environment thuộc đúng một Application và Environment ID là duy nhất trong Application.
- **BR-03:** Execution Profile không đổi trong happy path sau khi Application có Active Resources.
- **BR-04:** `aws-eks` dùng VPC/EKS scope Application; `internal-k8s` không tạo VPC/EKS.
- **BR-05:** namespace identity có scope Environment và phải ổn định qua các deployment.

## Luồng nội bộ

```text
UC-01 Manage Application and Environment
├── Create Application
├── Assign Execution Profile
├── Bind AWS account/region or internal cluster
├── Initialize Application runtime state
├── Create Environment
├── Initialize Deployment Set and namespace identity
└── Persist configuration
```

## Trạng thái implementation hiện tại

- Seed catalog đã tạo hai Application mẫu với profile cố định (`acceptance` dùng `internal-k8s`, `acceptance-cloud` dùng `aws-eks`) và Environment `dev`; API/Web Console có thể liệt kê chúng.
- Planning context đã dùng Application, Environment, profile, connection, region, namespace identity và runtime status; VPC/EKS có application scope.
- API/UI tạo Application/Environment và PostgreSQL persistence cho UC-01 chưa có; dữ liệu hiện được seed vào state store khi process khởi động.

## Ngoài phạm vi happy path

- **OOS-01:** Đổi Execution Profile sau khi Application đã có Active Resources.
- **OOS-02:** Xóa Application, EKS hoặc VPC.
- **OOS-03:** Clone và promotion Environment.
- **OOS-04:** RBAC chi tiết, concurrent update và audit history.
