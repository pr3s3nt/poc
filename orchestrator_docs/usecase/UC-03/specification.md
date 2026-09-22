---
id: UC-03-SPEC
artifact: use-case-specification
status: current
last_reviewed: 2026-09-22
---

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
2. **MS-02:** Platform Engineer khai báo Matching Criteria bằng các field `env_type`, `app_id`, `env_id`, `res_id` và `class`.
3. **MS-03:** Orchestrator validate cấu trúc Resource Definition và yêu cầu ít
   nhất một Matching Criterion.
4. **MS-04:** Orchestrator kiểm tra Resource Type, Driver và Driver Account/connection được tham chiếu tồn tại.
5. **MS-05:** Orchestrator validate Driver Inputs, resource references và provision rules.
6. **MS-06:** Orchestrator kiểm tra Definition ID chưa tồn tại trong Organization.
7. **MS-07:** Orchestrator xác nhận Definition có thể cung cấp output contract của Resource Type.
8. **MS-08:** Orchestrator lưu Resource Definition và Matching Criteria.
9. **MS-09:** Definition sẵn sàng được matching khi UC-05/UC-06 lập kế hoạch deployment.

## Luồng biến thể trong happy path

- **VAR-01 — `aws-eks`:** Definitions của `vpc`, `k8s-cluster` và `postgres` dùng Terraform Driver cùng AWS Driver Account.
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
- **BR-03:** Matching Criteria phân biệt `aws-eks` và `internal-k8s` bằng chính các field chuẩn, thường là `app_id` (Execution Profile gắn cố định vào Application) hoặc `env_type`. Không có field riêng cho Execution Profile.
- **BR-06:** Matching Criteria chỉ gồm năm field chuẩn với trọng số cố định:

  | Field | Trọng số |
  |---|---|
  | `env_type` | 1 |
  | `app_id` | 2 |
  | `env_id` | 4 |
  | `res_id` | 8 |
  | `class` | 16 |

  Một criterion chỉ match khi **mọi** field đã khai báo bằng đúng giá trị của context; specificity là tổng trọng số của các field đã khai báo. Với mỗi Definition lấy criterion điểm cao nhất, sau đó lấy Definition điểm cao nhất; hai Definition bằng điểm là `AMBIGUOUS_DEFINITION`, không có Definition nào match là `NO_MATCHING_DEFINITION`.
- **BR-04:** Resource references tạo dependency `consumer -> provider` và referenced output phải tồn tại.
- **BR-05:** Provider-specific implementation nằm trong Definition/executor, không nằm trong Score contract.
- **BR-07:** Lệnh đăng ký Definition phải có ít nhất một Matching Criterion;
  thiếu criteria hoặc `criteria: []` bị từ chối trước persistence. Một criterion
  `{}` khai báo tường minh là wildcard hợp lệ với specificity `0`.

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
| `aws-eks` | `vpc` | AWS VPC qua Terraform |
| `aws-eks` | `k8s-cluster` | Amazon EKS qua Terraform |
| `aws-eks` | `postgres` | Amazon Aurora PostgreSQL qua Terraform |
| `internal-k8s` | `k8s-cluster` | Cluster connection đã đăng ký |
| `internal-k8s` | `postgres` | StatefulSet và Service trên Kubernetes |

## Trạng thái implementation hiện tại

- Seed catalog đã có Definitions cho implicit VPC/EKS/namespace, existing cluster, Aurora và PostgreSQL StatefulSet; executor registry chọn Terraform/Kubernetes/existing-cluster adapter theo matched Definition.
- Planner đã hỗ trợ năm Matching Criteria chuẩn, Driver Inputs, Resource References, provision rules, fixed-point expansion và kiểm tra Terraform contract.
- Terraform execution của MVP chỉ hỗ trợ các module `vpc`, `eks` và `aurora` được nhúng trong binary. Conformance harness có thể inspect `source.url[@rev][/path]`, nhưng runtime chưa tải hoặc execute Terraform source từ xa.
- API quản trị, PostgreSQL persistence và nghiệp vụ `RegisterResourceDefinition` vẫn thuộc Phase 6 bước 5; hiện catalog được seed khi process khởi động.
- Driver enum ngắn, `ConnectionKey`, `source.module` và context placeholder mở
  rộng là contract nội bộ của MVP, không phải Humanitec public contract. Mapping
  boundary và `driver_inputs.secret_refs` được phân loại tại
  [compatibility matrix](../../implementation/humanitec-compatibility.md) và D06.

## Ngoài phạm vi happy path

- **OOS-01:** Cập nhật, vô hiệu hóa hoặc xóa Resource Definition.
- **OOS-02:** Phát hiện criteria chồng lấn ngay tại thời điểm đăng ký.
- **OOS-03:** Versioning và migration Definition.
- **OOS-04:** Kiểm tra ảnh hưởng tới Active Resources hiện có.
- **OOS-05:** Secret Driver Inputs và credential rotation.
- **OOS-06:** RBAC chi tiết và audit history.
- **OOS-07:** Tải, cache và execute Terraform source từ xa; MVP runtime chỉ dùng module nhúng đã kiểm soát.
