---
id: UC-03-SPEC
artifact: use-case-specification
status: current
last_reviewed: 2026-10-13
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
- **PRE-04:** Provider binding có contract hợp lệ. AWS dùng Connection của
  Environment lúc Preview/Deploy; registration không cần Environment cụ thể.
  Kubernetes bind target của Environment hoặc outputs của node EKS.
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

- **VAR-01:** Trên `aws-eks`, Terraform tạo VPC/EKS/namespace, Aurora/
  ElastiCache hoặc Redis/PostgreSQL in-cluster theo class. Mọi AWS provider
  dùng cùng Connection/account/region đã chọn cho Environment.
- **VAR-02:** Trên `internal-k8s`, cluster node lấy Connection của Environment
  theo ADR-013. Namespace, Redis và PostgreSQL vẫn matching Definition và dùng
  module Terraform với Kubernetes provider.

## Hậu điều kiện

- **POST-01:** Resource Definition được lưu với ID duy nhất trong Organization.
- **POST-02:** Definition liên kết với một Resource Type và một Driver hợp lệ.
- **POST-03:** Definition AWS cung cấp module Terraform cho VPC, EKS,
  ElastiCache và Aurora; class in-cluster trên EKS vẫn hợp lệ.
- **POST-04:** Definition Kubernetes cung cấp module Terraform cho namespace,
  Redis/PostgreSQL; cluster có sẵn chỉ được tham chiếu bằng system binding.
- **POST-05:** Hai PostgreSQL Definitions cùng triển khai contract outputs của Resource Type `postgres`.
- **POST-06:** Driver Inputs và provision rules sẵn sàng cho quá trình dựng Resource Graph.

## Quy tắc nghiệp vụ

- **BR-01:** Definition chỉ được xét cho node có cùng Resource Type và có `execution_profile` trống hoặc bằng profile của Environment. Profile là điều kiện lọc trước matching, không cộng điểm specificity.
- **BR-02:** Matching cần đúng một Definition thắng theo specificity.
  Không match, đồng hạng cao nhất hoặc binding của winner sai thì dừng; không
  đổi class/target hoặc chọn ứng viên khác.
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
- **BR-08:** Chỉ đăng ký cặp Type/driver/module trong registry đã kiểm chứng:
  Terraform cho namespace/Redis/PostgreSQL Kubernetes và VPC/EKS/ElastiCache/
  Aurora AWS; score-k8s cho Workload theo BR-15. Mọi public registration
  `existing-cluster` hoặc reserved system key đều bị từ chối trước insert.
  AWS Definition không nhận Connection/account/region override, kể cả Aurora.
  Kubernetes dùng target Environment hoặc target EKS từ dependency; Connection
  riêng nếu khai báo phải cùng target Environment. Binding sai bị chặn trước
  execution, không fallback credential host hoặc Definition khác.
- **BR-09:** Module phải từ nguồn Admin cho phép, pin full Git commit hoặc
  artifact digest, kiểm checksum/nội dung và các dependency module. Fetch/cache/
  inspect hoàn tất trước insert; kiểm đủ input/output mappings, provider pins
  và contract Type. Nguồn không hợp lệ, branch/latest, code drift hoặc output
  mismatch bị từ chối theo [module contract](../../architecture/contracts/terraform-modules.md).

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

## Definition mapping

| Profile | Resource | Implementation |
|---|---|---|
| aws-eks | VPC/EKS | Terraform AWS |
| aws-eks | Redis/PostgreSQL cloud-managed | Terraform ElastiCache/Aurora |
| internal-k8s / aws-eks | Namespace, Redis/PostgreSQL in-cluster | Terraform Kubernetes |
| internal-k8s | Cluster có sẵn | System Connection binding ADR-013 |
| internal-k8s / aws-eks | Workload | score-k8s, bắt buộc có Definition/bundle |

## Ngoài phạm vi happy path

- **OOS-01:** Cập nhật, vô hiệu hóa hoặc xóa Resource Definition.
- **OOS-02:** Phát hiện criteria chồng lấn ngay tại thời điểm đăng ký.
- **OOS-03:** Versioning và migration Definition.
- **OOS-04:** Kiểm tra ảnh hưởng tới Active Resources hiện có.
- **OOS-05:** Secret Driver Inputs và credential rotation.
- **OOS-06:** RBAC chi tiết và audit history.
- **OOS-07:** Chạy nguồn ngoài allowlist, module mutable và arbitrary command.
  Tải/cache/chạy module immutable trong allowlist thuộc phạm vi BR-09.

## Registration validation policy (2026-10-02)

- **BR-10:** New public Definition IDs follow UC-02 BR-05. Existing catalog
  entries and referenced Resource Type IDs are not renamed or revalidated as
  newly created IDs. Definition IDs have a separate namespace from Type IDs.
- **BR-11:** Driver Inputs là object có field đã định nghĩa. Giữ
  `values.variables`; Terraform `values.source` chứa module/source identity,
  revision/subdirectory/digest theo module contract. Input/output mappings và
  provider bindings là metadata được kiểm strict; không nhận field tùy ý/null.
  score-k8s không nhận Terraform source hoặc credential.
- **BR-12:** Kiểm variable và literal type theo module contract đã inspect,
  gồm element type list/map, required/default và constraints. Required input
  có thể được Type params, context, provider output hoặc executor cung cấp;
  không bắt PE nhập giá trị chưa có khi đăng ký. Namespace name phải có nguồn
  string bắt buộc hợp lệ; param của Workload ưu tiên hơn Definition fallback.
- **BR-13:** Valid context/resource placeholders remain supported. A complete
  placeholder can defer value-type checking until planning/execution; literal
  values and the literal elements of collections are checked at registration.
  Malformed placeholders, including nested `${...}` inside an unescaped
  placeholder body, are rejected. Interpolated strings are only suitable
  for string fields; escaped placeholders remain literals. Validation errors
  identify the field without quoting its submitted value.
- **BR-14:** Không nhận raw secret/credential trong Driver Inputs, kể cả
  `secret_refs` hoặc executor-owned password. Executor tạo password theo
  resource identity, lưu secret ref trước apply và dùng lại khi retry.
  AWS access keys chỉ nhập qua UC-04, không thuộc Definition.

## Workload rendering Definitions

- **BR-15:** Workload dùng score-k8s cho `internal-k8s` hoặc `aws-eks`,
  với bundle tương thích đã cài. Không Connection override, provision rule
  hay input/output tài nguyên; có đúng một literal `render_bundle`. Thiếu/
  unavailable bundle bị chặn. Fingerprint registration là audit; Plan pin
  snapshot bundle/binary/content đang được cài và Preview cũ bị stale khi đổi.
- **BR-16:** Registration does not accept templates, command provisioners, URLs
  or raw credentials. Workload execution belongs to UC-06 after UC-08 outputs;
  the driver is not registered as a resource provisioner.

See [workload rendering contract](../../architecture/contracts/workload-rendering.md).

## Environment execution target references

Environment Settings theo [ADR-012](../../architecture/decisions/ADR-012-environment-stores-and-transitions.md)
cung cấp profile/region via `${context.env.profile}` and
`${context.env.region}`. Product resolution rejects `${context.app.profile}` and
`${context.app.region}` as unknown context values; no server rewrite or legacy
fallback is allowed (T21/LEG-04). Application identity context remains unchanged.
AWS infrastructure references may
use `vpc.default#@infra`; parser expands @infra to the Environment path including target generation.
Cloud names use context.infra.resourceName (bounded Environment-specific hash identifier); context.infra.name is a readable stem only.
Seeded Definitions use these scope-aware values. Platform-authored hardcoded
application VPC/EKS references are rejected by product Environment
planning with guidance rather than introducing cross-environment
resources. Matching giữ specificity/tie; AWS chỉ dùng account của Environment.

## Implicit internal cluster binding

- **BR-17:** Internal cluster node bypasses user Definition matching; mọi public registration dùng driver `existing-cluster` bị từ chối trước insert; system-owned reserved Definition keys cannot be registered publicly for any driver. Existing authored cluster Definitions remain readable for compatibility but do not control the implicit node. Other resource matching/weights remain unchanged.

See [ADR-013](../../architecture/decisions/ADR-013-implicit-existing-cluster.md).

## Installed bundle discovery (T18)

- **BR-18:** Platform Engineer/Admin của Organization đã authenticate được đọc
  registry bundle bất biến của backend đang chạy qua GET `/api/v1/render-bundles`.
  Response chỉ gồm ID, version và status `AVAILABLE` cho entries thỏa cùng
  availability checks của BR-15; bỏ entries không hợp lệ. Registry dùng chung
  giữa các Organization; quyền truy cập lấy từ session, không nhận org từ request.
  Không trả digest, path, binary, template source hoặc credentials; không gọi
  renderer, inspector, persistence hay provider. Danh sách sắp theo ID, rỗng là `[]`.
- **BR-19:** Console lấy lựa chọn từ endpoint: không có bundle thì khóa đăng ký
  và hướng dẫn liên hệ Kỹ sư nền tảng; một bundle tự chọn khi draft chưa có lựa
  chọn; nhiều bundle cần chọn tường minh. Loading/error khóa đăng ký, có retry
  và giữ ID/criteria/lựa chọn; refresh và late replies không ghi đè lựa chọn
  người dùng. Lựa chọn không còn trong list không được submit; server vẫn kiểm
  availability tại POST và từ chối trước insert nếu bundle không còn available.

## Matching preview trước đăng ký (T19)

- **BR-20:** PE/Admin có thể gửi Definition chưa lưu và context tường minh tới
  POST `/api/v1/resource-definitions/matching-preview`. Organization lấy từ
  session; app/env phải tồn tại trong Organization đó. Profile, env_type và
  Connection lấy từ Environment trong một read-only snapshot, không tin giá trị
  target do client gửi. Context gồm `applicationId`, `environmentId`, `resourceId`
  và `class`; thiếu resourceId/class trả `INCOMPLETE`, không kết luận tie.
- **BR-21:** Snapshot catalog chỉ thay Definition cùng key bằng candidate trong
  bộ nhớ. Dùng `BestCriterion` và `ConnectionMismatch` của planner; BR-01/06
  giữ nguyên. Trả eligibility và criterion/score tốt nhất từng Definition,
  winner hoặc các ties ở điểm thắng cao nhất. Tie thấp hơn không ambiguous.
  Connection conflict chỉ chặn winner duy nhất; không chọn ứng viên thay thế.
  Đúng implicit internal k8s-cluster node (class internal, res_id của
  Connection descriptor) giữ system binding ADR-013, không materialize record;
  Workload không có match trả NO_MATCH; không chọn renderer khác và không render.
- **BR-22:** Kết quả chỉ phản ánh snapshot đã đọc: marker deterministic từ
  catalog/Environment target và version, không phải save/deploy token. Không
  registration, insert, system admission, inspector/executor, Vault, kube/cloud
  calls hoặc SQL writes. Snapshot reads qua persistence được phép. Không validate
  đầy đủ Driver Inputs/output contract hoặc bảo đảm candidate đăng ký được;
  registration và planner vẫn kiểm lại độc lập. Request bounded/known-field;
  response/error không chứa inputs, credentials, raw provider diagnostics.

## T27 — Current seed and scope boundary

Seed inserts only missing Definitions; no earlier template form is upgraded.
ENVIRONMENT is the only execution infrastructure scope. Invalid/retired scopes
fail before planning/execution; matching profiles, criteria weights and ties remain unchanged.

## Capability metadata và typed authoring (T31)

- **BR-23:** GET `/api/v1/resource-definition-capabilities` chỉ đọc cho
  PE/Admin của Organization. Registry đã inspect cung cấp cặp Type/module,
  source/revision an toàn, provider requirements, class, schema, defaults và
  executor-owned inputs. Request metadata không fetch module, chạy provider
  hoặc ghi database. Không suy Type từ tên Terraform vars; registration và
  Preview/Deploy vẫn kiểm độc lập.
- **BR-24:** Executor-owned variables được nhận diện riêng và không có control
  tác giả trong typed form; master_password/credentials vẫn bị từ chối theo
  BR-14. Chỉ công bố defaults đã có ở module/executor, không tự đoán CIDR,
  storage, engine/capacity. Optional omitted giữ runtime default; params không
  được materialize lại vào Definition. Override default/preset là opt-in; giá trị Definition là fallback cho param,
  param thực của workload vẫn có ưu tiên cao hơn theo contract planner.
- **BR-25:** Typed literals giữ JSON types và BR-13 placeholders. Namespace có
  preset `${context.env.namespace}`, Terraform name có preset
  `${context.infra.resourceName}`; VPC output refs dùng `vpc.default#@infra`
  cho id/cidr/subnetIds theo module hiện hành. Preset chỉ áp dụng tường minh,
  không overwrite override. Advanced JSON round-trip lossless các field hợp lệ;
  trường hợp không biểu diễn an toàn giữ ở advanced, không discard âm thầm.
- **BR-26:** Provision editor giữ descriptor và params cùng flags
  `is_dependent`/`match_dependents` của UC-06 BR-08. Reference edge luôn
  consumer -> provider; provider-first scheduling không đổi. Metadata/form không
  tự matching-preview/provision hoặc gọi external provider. Unsupported/null/
  credential inputs giữ strict rejection, không nhận template/command hoặc nguồn module ngoài BR-09.
