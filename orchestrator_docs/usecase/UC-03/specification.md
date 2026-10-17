---
id: UC-03-SPEC
artifact: use-case-specification
status: current
last_reviewed: 2026-10-17
---

# UC-03 — Register Resource Definition and Matching Criteria

## Mục tiêu

Tạo và sửa Resource Definition để xác định driver, inputs, account và Matching Criteria của một Resource Type; kiểm tra matching trên máy chủ trước khi sử dụng.

## Hợp đồng V02 — áp dụng từ 2026-10-17

Phần này sở hữu hợp đồng V02. Các ID requirement cũ được giữ và ánh xạ
sang hợp đồng mới bên dưới; predecessor truy xuất bằng Git tại `96eb602`. Quy tắc nguồn module bất biến, validation
schema/placeholders/mappings/provision, bí mật và weighted criteria tiếp tục có
hiệu lực. System cluster theo ADR-013 và cơ chế render hiện tại chỉ được thay
bằng Generic Workload ở V03; V02 không đăng ký public existing-cluster.

- **V02-01 (MS-01..09, BR-01/03/08):** Definition có ID kỹ thuật `key`,
  `name` hiển thị bắt buộc, `resourceType`, `driverType`, `driverInputs`,
  `driverAccount` khi driver cần account, `criteria`, `provision`, `version`.
  ID tuân UC-02 BR-05; Name là text không rỗng sau trim, tối đa253 ký tự,
  không control characters; ID và Resource Type bất biến khi sửa.
  `executionProfile` và `connectionKey` không còn là public authoring fields;
  không chuyển field cũ hoặc suy account từ Environment.
- **V02-02 (BR-01/02/03/06/07/21):** Cùng Resource Type, xét đúng năm field
  `env_type`, `app_id`, `env_id`, `res_id`, `class`; AND trong dòng, OR giữa
  các dòng; lấy điểm tốt nhất mỗi Definition, rồi unique maximum với trọng số
  1/2/4/8/16. Không lọc ngầm theo profile/account/region/nhóm triển khai.
  Không criteria/[] bị từ chối; `[{}]` là wildcard điểm0. Không match và hòa
  điểm cao nhất là hai kết quả khác nhau. Winner sai account/driver thì báo
  lỗi winner, không bỏ qua để chọn ứng viên khác. Definition ID khác `res_id`.
- **V02-03 (BR-08/11/12/14/23/24):** Danh mục driver do backend công bố từ
  capability registry thực: Terraform cho modules đã verified và score-k8s
  cho render bundle hiện có; không quảng cáo Generic Workload lifecycle.
  AWS Terraform bắt buộc `driverAccount` trỏ key AWS READY cùng Organization;
  region là literal string hợp lệ tại `driverInputs.values.variables.region`,
  không lấy Environment region hoặc verificationRegions làm phạm vi cho phép.
  AWS provider binding là `{target: account}`. Kubernetes Terraform nhận
  `{target: resource, resource_ref: <cluster descriptor>}`; không chọn account
  lại, UI hiển thị nguồn cluster dependency. Trong giai đoạn trước V03,
  `{target: environment}` còn là binding Kubernetes legacy có nhãn rõ,
  không trở thành account selector hoặc matching filter. score-k8s không nhận
  driverAccount; existing-cluster vẫn dành system ở V02.
- **V02-04 (BR-09/11..16/23..26):** Giữ verified pins, input/output mapping,
  typed variables/defaults/params và placeholders/reference/provision flags.
  Region là input PE được phép cấu hình cho AWS; credential/password/tags
  executor-owned vẫn không editable. Unknown/null/raw secret bị từ chối an toàn.
  Validation metadata mô tả account requirement, account kind và target source;
  không tự suy driver từ tên Type. Source unavailable phải giữ draft và có retry.
- **V02-05 (BR-20..22):** Matching preview đọc một scoped snapshot của app,
  env, Types, Definitions và account metadata; không cần Environment có
  Connection READY để tính matching thường. Context env_type lấy server;
  app/env/resourceId/class như OC-04c. Candidate chỉ replace cùng key trong
  memory. Trả criterion/score, unique winner/ties/no-match và lý do, safe
  account key/name/type/version, effective region hoặc cluster source reference
  của winner. Không trả credential/ref/config/endpoint. Thiếu context trả
  INCOMPLETE. Không resolve secret, inspector, render, external calls hay writes.
  Marker bao phủ catalog/account/context versions; đổi draft/context/session
  hoặc metadata làm stale kết quả và vô hiệu late reply. Kết quả không phải
  token Save/Deploy; server kiểm lại ở registration/planning/pre-provision.
- **V02-06:** Người dùng chốt 2026-10-17: PE/Admin được sửa Name, Driver Type,
  Inputs, Account, criteria và provision, giữ ID/Resource Type. Gửi expected
  version; một CAS transaction cập nhật Definition + criteria + verified bundle
  association và tăng version; race409 giữ winner, rollback không lưu nửa chừng.
  Verify/inspect ngoài transaction; không ghi đè khi chưa rõ kết quả COMMIT,
  không tự replay mutation. UI giữ draft và yêu cầu tải lại/review khi stale.
- **V02-07:** Sửa chỉ ảnh hưởng Preview/Deploy mới; không sửa persisted plan,
  snapshot, current/applied resource, original target/state/module/ownership hoặc
  cleanup pins. Preview cũ phải stale khi Definition/account đổi trước admission. Execution
  đã admit dùng snapshot/pins gốc, không đọc Definition mutable để đổi cấu hình.
  Đổi account/region không tự cho phép di chuyển/adopt state/resource đang có;
  reuse mismatch vẫn fail closed theo lifecycle/ownership contract. Không thêm
  xóa Definition hoặc tự provision khi Save/Preview.
- **V02-08:** Fresh SQL/JSON baseline/seed và acceptance fixture dùng hợp đồng
  mới; không migration/upsert dữ liệu cũ. Seed không dựa profile để tránh tie:
  dùng class/context tường minh hoặc chỉ một default cho từng nhu cầu. Không
  thêm criterion theo account/group. Old incompatible state fail rõ, không
  auto-reset. Codex chỉ reset đúng dev instance/ownership đã được cho phép.
- **V02-09:** Tất cả nơi tiêu thụ Definition (catalog, render template,
  matching preview, planner và pre-provision) dùng cùng matching/account contract.
  Runtime không được dùng Environment identity thay driverAccount. Giữ locking,
  fencing, secret isolation và pins. Không claim live multi-account AWS hoặc
  Generic Workload đã pass trong V02; live V03–V06 không thuộc task này.

## Primary actor

- Platform Engineer

## Tiền điều kiện

- **PRE-01:** Organization đã tồn tại.
- **PRE-02:** Resource Type tương ứng đã được đăng ký qua UC-02.
- **PRE-03:** Driver được tham chiếu đã được orchestrator hỗ trợ.
- **PRE-04:** Driver metadata/account binding hợp lệ theo V02-03/04; không cần
  Environment cụ thể để đăng ký Definition.
- **PRE-05:** Platform Engineer có quyền quản lý Resource Definition.

## Trigger

**TRG-01:** Platform Engineer gửi Resource Definition cùng Matching Criteria cần đăng ký.

## Main success scenario

1. **MS-01:** Platform Engineer cung cấp Definition ID, Name, Resource Type, Driver Type, Driver Inputs và Driver Account phù hợp.
2. **MS-02:** Khai báo năm Matching Criteria; không có Definition profile guard.
3. **MS-03:** Orchestrator validate cấu trúc Resource Definition và yêu cầu ít
   nhất một Matching Criterion.
4. **MS-04:** Orchestrator kiểm tra Resource Type, Driver và Driver Account được tham chiếu tồn tại.
5. **MS-05:** Orchestrator validate Driver Inputs, resource references và provision rules.
6. **MS-06:** Orchestrator kiểm tra Definition ID chưa tồn tại trong Organization.
7. **MS-07:** Orchestrator xác nhận Definition có thể cung cấp output contract của Resource Type.
8. **MS-08:** Orchestrator lưu Resource Definition và Matching Criteria.
9. **MS-09:** Definition sẵn sàng được matching khi UC-05/UC-06 lập kế hoạch deployment.

## Luồng biến thể trong happy path

- **VAR-01:** Terraform AWS chọn account tại Definition và region từ Driver Inputs;
  PostgreSQL/Redis managed hay in-cluster được chọn bởi class/criteria tường minh.
- **VAR-02:** Kubernetes consumer kế thừa cluster dependency; binding Environment
  legacy và implicit cluster được giữ tới V03, không dùng làm matching filter.

## Hậu điều kiện

- **POST-01:** Resource Definition được lưu với ID duy nhất trong Organization.
- **POST-02:** Definition liên kết với một Resource Type và một Driver hợp lệ.
- **POST-03:** Definition AWS cung cấp module Terraform cho VPC, EKS,
  ElastiCache và Aurora; class in-cluster trên EKS vẫn hợp lệ.
- **POST-04:** Definition Kubernetes cung cấp module Terraform cho namespace,
  Redis/PostgreSQL; cluster có sẵn vẫn system binding trong V02, thay ở V03.
- **POST-05:** Hai PostgreSQL Definitions cùng triển khai contract outputs của Resource Type `postgres`.
- **POST-06:** Driver Inputs và provision rules sẵn sàng cho quá trình dựng Resource Graph.

## Quy tắc nghiệp vụ

- **BR-01:** Cùng Resource Type và năm criteria theo V02-02; không profile filter.
- **BR-02:** Matching cần đúng một Definition thắng theo specificity.
  Không match, đồng hạng cao nhất hoặc binding của winner sai thì dừng; không
  đổi class/target hoặc chọn ứng viên khác.
- **BR-03:** Không có executionProfile/Connection/account/region/group trong
  predicate ngoài năm criteria chuẩn theo V02-02.
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
- **BR-08:** Supported driver/account/provider contracts theo V02-03/04;
  public existing-cluster/reserved keys vẫn bị từ chối trong V02. Winner sai
  binding bị chặn, không fallback credential host hoặc Definition khác.
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
├── Validate five-field Matching Criteria and selected account
├── Validate common Resource Type output contract
├── Persist Definition and Criteria
└── Publish Definition for deployment matching
```

## Definition mapping

| Resource | Driver | Account/target |
|---|---|---|
| VPC/EKS, Aurora/ElastiCache | verified Terraform AWS | Definition AWS Driver Account + Driver Inputs region |
| Namespace, Redis/PostgreSQL in-cluster | verified Terraform Kubernetes | Cluster dependency; explicit legacy Environment binding until V03 |
| Existing internal cluster | system existing-cluster | ADR-013 until V03, no public registration |
| Current rendering template | score-k8s | Installed bundle, no account selection; not Generic Workload lifecycle |

## Ngoài phạm vi happy path

- **OOS-01:** Vô hiệu hóa hoặc xóa Resource Definition; cập nhật thuộc V02-06.
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

- **BR-15:** Workload dùng score-k8s không có profile matching guard,
  với bundle tương thích đã cài. Không Connection override, provision rule
  hay input/output tài nguyên; có đúng một literal `render_bundle`. Thiếu/
  unavailable bundle bị chặn. Fingerprint registration là audit; Plan pin
  snapshot bundle/binary/content đang được cài và Preview cũ bị stale khi đổi.
- **BR-16:** Registration does not accept templates, command provisioners, URLs
  or raw credentials. Workload execution belongs to UC-06 after UC-08 outputs;
  the driver is not registered as a resource provisioner.

See [workload rendering contract](../../architecture/contracts/workload-rendering.md).

## Context và resource references

Các context được hỗ trợ, scoped resource references và @infra vẫn giữ contract;
Application aliases app.profile/app.region đã bị bỏ và không rewrite/fallback.
AWS region/account không được lấy context.env.region/profile để thay input
Definition V02-03. Reference có output hợp lệ tạo graph edge; output declaration
không tự tạo dependency. Legacy Environment graph enrichment chỉ giữ tới V03.

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

- **BR-20:** PE/Admin preview một candidate/context tường minh theo V02-05.
  app/env lấy scoped snapshot, env_type server-derived; resourceId/class thiếu
  trả INCOMPLETE. Ordinary matching không cần configured Environment target.
- **BR-21:** Candidate replace cùng key trong memory; shared five-field matcher,
  highest unique winner/ties; account conflict kiểm sau chọn winner, không fallback.
  Legacy implicit node ADR-013 có thể trả SYSTEM_BINDING tới V03; không write.
- **BR-22:** Marker gồm catalog/account/context versions; safe selected identity,
  effective region/cluster source theo V02-05. Không secret, inspector, render,
  provider call hay writes. Không thay full graph Preview/Save/Deploy token.

## T27 — Current seed and scope boundary

Seed inserts only missing Definitions; no earlier template form is upgraded.
ENVIRONMENT is the only execution infrastructure scope. Invalid/retired scopes
fail before planning/execution; five-field criteria weights and ties remain unchanged.
Fresh V02 seed/account/baseline follows V02-08 without profile disambiguation.

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
