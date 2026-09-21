---
id: UC-04-SPEC
artifact: use-case-specification
status: current
last_reviewed: 2026-09-21
---

# UC-04 — Configure Execution Profile Connections

## Mục tiêu

Cấu hình credentials và connection cần thiết cho hai Execution Profile `aws-eks` và `internal-k8s`.

## Primary actor

- Platform Engineer

## Supporting actors

- Kubernetes Cluster/Operator
- AWS Cloud

## Tiền điều kiện

- **PRE-01:** Organization đã tồn tại.
- **PRE-02:** AWS credentials hoặc Kubernetes credentials cần thiết đã được tạo.
- **PRE-03:** Platform Engineer có quyền quản lý Driver Account và cluster connection.

## Trigger

**TRG-01:** Platform Engineer yêu cầu cấu hình một AWS Driver Account hoặc đăng ký Kubernetes cluster nội bộ.

## Main success scenario

1. **MS-01:** Platform Engineer chọn Execution Profile cần cấu hình.
2. **MS-02:** Platform Engineer cung cấp connection data theo VAR-01 hoặc VAR-02.
3. **MS-03:** Orchestrator kiểm tra identity, connectivity và quyền tối thiểu của connection.
4. **MS-04:** Orchestrator lưu secret reference an toàn, không lưu credential thô trong domain record.
5. **MS-05:** Orchestrator lưu connection cùng trạng thái `READY`.
6. **MS-06:** Connection sẵn sàng để được Application lựa chọn trong UC-01 và Definition tham chiếu trong UC-03.

## Luồng biến thể trong happy path

- **VAR-01 — `aws-eks`:** cung cấp AWS Driver Account, allowed regions và Terraform backend configuration; MS-03 xác minh quyền tạo/đọc VPC, EKS, Aurora và quản lý Terraform state.
- **VAR-02 — `internal-k8s`:** cung cấp Cluster ID, endpoint và credential reference; MS-03 gọi Kubernetes API để xác minh quyền tạo namespace, StatefulSet, Service và workload.

## Hậu điều kiện

- **POST-01:** `aws-eks` có Driver Account và Terraform backend hợp lệ; chưa tạo VPC hoặc EKS.
- **POST-02:** `internal-k8s` có ít nhất một cluster connection ở trạng thái `READY`.
- **POST-03:** Resource Definition có thể tham chiếu Driver Account hoặc cluster connection.
- **POST-04:** UC-06 có đủ thông tin để lazy-provision Cloud infrastructure hoặc dùng cluster nội bộ.

## Quy tắc nghiệp vụ

- **BR-01:** Connection thuộc một Organization và có ID duy nhất trong Organization.
- **BR-02:** Domain record chỉ giữ secret reference; secret value không được trả về qua read model thông thường.
- **BR-03:** UC-04 chỉ đăng ký/xác minh connection, không provision application infrastructure.
- **BR-04:** Chỉ connection trạng thái `READY` mới được UC-01 và UC-06 sử dụng.

## Luồng nội bộ

```text
UC-04 Configure Execution Profile Connections
├── Select aws-eks or internal-k8s
├── Register and verify AWS Driver Account
├── Register and verify internal Kubernetes cluster
├── Securely store credentials
└── Publish connection for Application configuration
```

## Trạng thái implementation hiện tại

- Seed catalog đã đăng ký internal Kubernetes connection và AWS connection metadata; bootstrap wire Kubernetes, existing-cluster và Terraform adapters theo Execution Profile.
- Internal verification dùng kube context đã cấu hình; AWS verification dùng local default credential chain, region và account ID truyền khi khởi động process.
- API/UI đăng ký connection, Secret Store bền vững và workflow verify credential/connectivity của UC-04 chưa có. Terraform vẫn dùng local state directory, chưa có backend registry bền vững.

## Ngoài phạm vi happy path

- **OOS-01:** Provision VPC/EKS; trách nhiệm này thuộc UC-06 và UC-08.
- **OOS-02:** Humanitec-style Agent, private cluster tunnel và GitOps mode.
- **OOS-03:** Cài đặt Operator tự động và credential rotation.
- **OOS-04:** Health monitoring, reconnect, RBAC chi tiết và audit history.
