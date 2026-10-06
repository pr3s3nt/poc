---
id: UC-03-SPEC
artifact: use-case-specification
status: current
last_reviewed: 2026-10-02
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
2. **MS-02:** Platform Engineer chọn `execution_profile` áp dụng cho Definition (hoặc để trống nếu dùng chung), rồi khai báo Matching Criteria bằng các field `env_type`, `app_id`, `env_id`, `res_id` và `class`.
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

- **BR-01:** Definition chỉ được xét cho node có cùng Resource Type và có `execution_profile` trống hoặc bằng profile của Application. Profile là điều kiện lọc trước matching, không cộng điểm specificity.
- **BR-02:** Matching chọn criterion hợp lệ có specificity cao nhất; happy path yêu cầu đúng một Definition thắng.
- **BR-03:** `execution_profile` là thuộc tính của Definition, không phải field Matching Criterion. Hai Definition PostgreSQL theo profile dùng cùng 5 field chuẩn để xét context sau khi lọc profile; `env_type` vẫn là loại Environment, không đại diện target nội bộ/cloud.
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
- **BR-08 (MVP registration):** Chỉ đăng ký implementation mà runtime hiện có
  thể chạy: Terraform module nhúng `vpc`, `eks`, `aurora` cho Resource Type
  tương ứng; Kubernetes cho `k8s-namespace`/`postgres`; existing-cluster cho
  `k8s-cluster`. Terraform/existing-cluster cần connection tường minh đúng kind,
  thuộc Organization và `READY`. Kubernetes dùng connection của Application lúc
  deploy nếu Definition không chỉ định connection riêng.
- **BR-09:** Đăng ký Terraform Definition phải kiểm tra module tồn tại, các
  biến được khai báo và output Resource Type có thể được cung cấp. Source URL
  từ xa không được chấp nhận trong MVP.

## Luồng nội bộ

```text
UC-03 Register Resource Definition
├── Parse Definition document
├── Validate Resource Type and Driver
├── Validate Driver Inputs and references
├── Validate optional Execution Profile guard and Matching Criteria
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
- Planner đã hỗ trợ optional Execution Profile guard trước năm Matching Criteria chuẩn, Driver Inputs, Resource References, provision rules, fixed-point expansion và kiểm tra Terraform contract. Seeded PostgreSQL Definitions áp dụng cho mọi Application cùng profile.
- Terraform execution của MVP chỉ hỗ trợ các module `vpc`, `eks` và `aurora` được nhúng trong binary. Conformance harness có thể inspect `source.url[@rev][/path]`, nhưng runtime chưa tải hoặc execute Terraform source từ xa.
- API/UI đăng ký và nghiệp vụ `RegisterResourceDefinition` cùng normalized
  PostgreSQL persistence đã có; production-grade RBAC còn thiếu. Catalog seed
  vẫn cung cấp Definition mặc định.
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

## Registration validation policy (2026-10-02)

- **BR-10:** New public Definition IDs follow UC-02 BR-05. Existing catalog
  entries and referenced Resource Type IDs are not renamed or revalidated as
  newly created IDs. Definition IDs have a separate namespace from Type IDs.
- **BR-11:** Driver Inputs are a strict known-field object: only `values` at
  root, and `variables` plus Terraform-only `source` within `values`.
  `values` and `variables` are required objects. Kubernetes/existing-cluster
  must omit `source`; Terraform source contains only the supported `module`.
  Reject unknown fields, wrong container shapes and null values before saving.
- **BR-12:** Validate variable names and literal types against the selected
  driver contract. Terraform uses the embedded module's declared types,
  including element types of lists/maps. Kubernetes namespace accepts only
  `name: string`; Kubernetes PostgreSQL accepts `database`, `username`, `image`,
  `storage`, `namespace` (all strings); existing-cluster accepts `name` and
  `kubeContext` (strings). PostgreSQL options and existing-cluster values may
  be omitted where runtime supplies a default/connection value; namespace name
  must be supplied by Definition or a required string `name` input in its
  Resource Type contract; an optional/wrong-type input does not satisfy this
  requirement. Terraform
  required inputs may be supplied by Resource Type params or executor as
  defined in BR-09; do not require those values at registration.
- **BR-13:** Valid context/resource placeholders remain supported. A complete
  placeholder can defer value-type checking until planning/execution; literal
  values and the literal elements of collections are checked at registration.
  Malformed placeholders, including nested `${...}` inside an unescaped
  placeholder body, are rejected. Interpolated strings are only suitable
  for string fields; escaped placeholders remain literals. Validation errors
  identify the field without quoting its submitted value.
- **BR-14:** No raw credential or secret Driver Input support is added. Reject
  `secret_refs`, credential fields and executor-owned `master_password`; the
  Kubernetes PostgreSQL executor continues generating its password. AWS access
  keys belong to UC-04 Secret Store, never a Definition.

## Workload rendering Definitions

- **BR-15:** `workload` may use `score-k8s` only with `internal-k8s`, no connection
  override/provision rules/resource inputs/outputs, and exactly one literal
  `render_bundle` variable referring to the installed `score-k8s-internal-v1`
  bundle ID. Unknown/unavailable bundles fail before insert; the server-owned source
  fingerprint records the bundle digest at registration as audit provenance only
  (it is not a later equality gate; plans pin the installed bundle snapshot).
  Existing BR-11 nesting remains valid.
- **BR-16:** Registration does not accept templates, command provisioners, URLs
  or raw credentials. Workload execution belongs to UC-06 after UC-08 outputs;
  the driver is not registered as a resource provisioner.

See [workload rendering contract](../../architecture/contracts/workload-rendering.md).
