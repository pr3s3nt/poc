# UC-03 — Register Resource Definition and Matching Criteria

## Mục tiêu

Đăng ký Resource Definition để xác định cách provision một Resource Type theo Execution Profile và deployment context.

## Primary actor

- Platform Engineer

## Tiền điều kiện

- **PRE-01:** Organization đã tồn tại.
- **PRE-02:** Resource Type tương ứng đã được đăng ký qua UC-02.
- **PRE-03:** Driver được tham chiếu đã được orchestrator hỗ trợ.
- **PRE-04:** Driver Account hoặc cluster connection cần thiết đã được cấu hình qua UC-04.
- **PRE-05:** Platform Engineer có quyền quản lý Resource Definition.

## Trigger

**TRG-01:** Platform Engineer gửi Resource Definition cùng Matching Criteria cần đăng ký.

## Main success scenario

1. **MS-01:** Platform Engineer cung cấp Definition ID, Resource Type, Driver Type và Driver Inputs.
2. **MS-02:** Platform Engineer khai báo Matching Criteria theo Environment Type tương ứng với `aws-eks` hoặc `internal-k8s`.
3. **MS-03:** Orchestrator validate cấu trúc Resource Definition.
4. **MS-04:** Orchestrator kiểm tra Resource Type, Driver và Driver Account/connection được tham chiếu tồn tại.
5. **MS-05:** Orchestrator validate Driver Inputs, resource references và provision rules.
6. **MS-06:** Orchestrator kiểm tra Definition ID chưa tồn tại trong Organization.
7. **MS-07:** Orchestrator xác nhận Definition có thể cung cấp output contract của Resource Type.
8. **MS-08:** Orchestrator lưu Resource Definition và Matching Criteria.
9. **MS-09:** Definition sẵn sàng được matching khi UC-05/UC-06 lập kế hoạch deployment.

## Luồng biến thể trong happy path

- **VAR-01 — `aws-eks`:** Definitions của `network`, `k8s-cluster` và `postgres` dùng Terraform Driver cùng AWS Driver Account.
- **VAR-02 — `internal-k8s`:** `k8s-cluster` resolve connection có sẵn; `postgres` và namespace dùng Kubernetes executor.

## Hậu điều kiện

- **POST-01:** Resource Definition được lưu với ID duy nhất trong Organization.
- **POST-02:** Definition liên kết với một Resource Type và một Driver hợp lệ.
- **POST-03:** Cloud Definitions có thể provision VPC, EKS và Aurora bằng Terraform.
- **POST-04:** Internal Definitions có thể đại diện cluster có sẵn và provision PostgreSQL StatefulSet bằng Kubernetes executor.
- **POST-05:** Hai PostgreSQL Definitions cùng triển khai contract outputs của Resource Type `postgres`.
- **POST-06:** Driver Inputs và provision rules sẵn sàng cho quá trình dựng Resource Graph.

## Quy tắc nghiệp vụ

- **BR-01:** Definition chỉ được xét cho node có cùng Resource Type.
- **BR-02:** Matching chọn criterion hợp lệ có specificity cao nhất; happy path yêu cầu đúng một Definition thắng.
- **BR-03:** Matching Criteria phải phân biệt `aws-eks` và `internal-k8s` bằng Environment Type/Execution Profile context.
- **BR-04:** Resource references tạo dependency `consumer -> provider` và referenced output phải tồn tại.
- **BR-05:** Provider-specific implementation nằm trong Definition/executor, không nằm trong Score contract.

## Luồng nội bộ

```text
UC-03 Register Resource Definition
├── Parse Definition document
├── Validate Resource Type and Driver
├── Validate Driver Inputs and references
├── Validate Execution Profile Matching Criteria
├── Validate common Resource Type output contract
├── Persist Definition and Criteria
└── Publish Definition for deployment matching
```

## Definition mapping tối thiểu

| Execution Profile | Resource | Implementation |
|---|---|---|
| `aws-eks` | `network` | AWS VPC qua Terraform |
| `aws-eks` | `k8s-cluster` | Amazon EKS qua Terraform |
| `aws-eks` | `postgres` | Amazon Aurora PostgreSQL qua Terraform |
| `internal-k8s` | `k8s-cluster` | Cluster connection đã đăng ký |
| `internal-k8s` | `postgres` | StatefulSet và Service trên Kubernetes |

## Trạng thái implementation hiện tại

- Planner hiện tại đọc Resource Definition từ YAML fixture.
- Planner đã hỗ trợ Matching Criteria, Driver Inputs, resource references và co-provision rules trong phạm vi challenge.
- Chưa có Definition cho implicit VPC/EKS, Kubernetes executor, API, persistent storage hoặc Driver registry.

## Ngoài phạm vi happy path

- **OOS-01:** Cập nhật, vô hiệu hóa hoặc xóa Resource Definition.
- **OOS-02:** Phát hiện criteria chồng lấn ngay tại thời điểm đăng ký.
- **OOS-03:** Versioning và migration Definition.
- **OOS-04:** Kiểm tra ảnh hưởng tới Active Resources hiện có.
- **OOS-05:** Secret Driver Inputs và credential rotation.
- **OOS-06:** RBAC chi tiết và audit history.
