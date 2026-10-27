---
id: UC-03-UI-API
artifact: use-case-api-mapping
status: current
last_reviewed: 2026-10-28
related: UC-03
---

# UC-03 HTTP mapping

Current authoring/edit/account/matching follows the V02, V03 and V04–V06
replacement sections below and UC-03 specification. Earlier T02–T31 descriptions
are historical wherever they name profiles, an Environment target, insert-only
editing, builtin cluster restrictions or client-only matching preview. Only
unchanged typed inputs/interaction details survive. Predecessor: commit96eb602.

`GET /api/v1/resource-definitions` lists the session Organization's catalog.
`POST /api/v1/resource-definitions` is Platform Engineer/Admin registration,
not update. Body is one bounded known-field Definition object, with at least one
criterion (explicit `{}` wildcard is valid) and only existing runtime-supported
driver/type pairs. Unknown fields/trailing/non-object bodies are rejected.
Success 201, validation 400, duplicate 409, missing session 401, wrong role 403,
oversize 413. Unknown store/inspector causes are generic 500 without raw content;
do not expose catalog, module, credential or process diagnostics as validation.
Validation of the caller's own document remains actionable. A duplicate cannot
replace the original Definition/criteria. Newly registered matching Definitions
are visible to planning without restart. Module immutable thuộc allowlist được đăng ký theo module contract; raw
secret Driver Inputs vẫn bị từ chối.

## Registration policy

Definition ID and Driver Inputs follow specification BR-10–BR-14. Unknown nested keys, unsupported variables, nulls and wrong literal types return 400 with a safe field path; advanced JSON preserves valid placeholders. Form retains input on failure.

## Workload renderer variant (ADR-010)

The same Definition registration endpoint accepts Type `workload`, driver
`score-k8s`, profile `internal-k8s` hoặc `aws-eks` tương thích bundle, và
`values.variables.render_bundle`.
The Console supplies an installed bundle ID and hides connection/provision/
arbitrary-variable inputs for this variant. Server-owned `sourceFingerprint`
pins the bundle; unavailable bundles and unsupported pairs are validation errors.

## Resource form projection (T02)

Form Cấu hình tài nguyên dùng GET `/resource-types`, `/resource-definitions`
và `/connections`; các list do server scope theo Organization của phiên.
Lọc workload khỏi list/type selector và không cung cấp driver existing-cluster
trong form. Public POST từ chối mọi driver `existing-cluster` với validation
400 trước insert, bất kể key; reserved system key được bảo vệ cho mọi driver.
GET vẫn đọc records có sẵn; không thêm API xóa hoặc compatibility đăng ký cũ.
Workload renderer contract phía trên vẫn được giữ.

POST Definition giữ `key`, `resourceType`, `driverType`, `executionProfile`,
`driverInputs` và criteria. Terraform source identity/revision/digest, typed
variables, output mapping và provider bindings theo
[module contract](../../../architecture/contracts/terraform-modules.md).
AWS không nhận Connection override; Kubernetes symbolic binding chỉ resolve
Environment/EKS target. Server kiểm allowlist/digest/contracts trước insert.
Criteria giữ năm fields, wildcard gửi `[{}]`; không đổi specificity.

Frontend kiểm ID rỗng/shape và JSON trước POST; 409 có thông báo trùng ID Việt.
Các lỗi khác có fallback Việt an toàn và giữ form; không parse/regex raw backend
English message. Mapping field/code theo contract lỗi T20.

## Editor criteria projection (T02B)

GET `/api/v1/applications` cung cấp ứng dụng và metadata môi trường thực của
Organization trong phiên cho dropdown. Không suy ID từ tên hoặc dùng
`env_type` thay `env_id`. GET `/resource-definitions` cung cấp nguồn copy;
chỉ sao chép `criteria` thành giá trị độc lập. Loading/error/retry không làm
phát sinh POST và không thay criteria đang sửa.

Bốn chế độ đều serialize theo năm field hiện hành; wildcard là `[{}]`, loại
field rỗng. Không đổi registration contract, specificity hoặc profile guard.
Xem trước phạm vi/cảnh báo frontend không persist/provision và không gọi
endpoint mới. Matching preview authoritative dùng OC-04c ở T19.

## Trang Mẫu dựng ứng dụng projection (T03)

GET `/resource-definitions` cung cấp list scoped theo Organization; UI lọc
`resourceType === "workload"`. GET `/applications` phục vụ editor criteria chung.
POST cùng endpoint gửi `key`, `resourceType: "workload"`,
`driverType: "score-k8s"`, profile từ bundle selection (`internal-k8s`/`aws-eks`),
`driverInputs: {values: {variables: {render_bundle: <ID đã chọn từ registry>}}}`
và criteria chuẩn. Không gửi connection override, provision hoặc source.
GET `/api/v1/render-bundles` theo OC-04b trả `renderBundles` với ID/version/status
`AVAILABLE`; chỉ PE/Admin, session org bắt buộc. Không thêm matching-preview ở T18.
Status/scoping/insert-only và BR-15/16 không đổi. Frontend giữ form khi POST
lỗi, 409 có thông báo trùng ID Việt, lỗi bundle có thông báo Việt an toàn.

## Điều hướng từ Loại tài nguyên (T04)

CTA từ UC-02 mở `/ui/platform/resource-definitions?resourceType=<key>`.
Chỉ chọn sẵn type khi catalog tải thành công xác nhận key có cặp hỗ trợ trong metadata T31; query thiếu/không hỗ trợ không tạo lựa chọn hợp lệ. Prefill không
gửi POST hoặc ghi đè dữ liệu người dùng khi reload/response đến muộn.

## Matching preview API (T19)

POST `/api/v1/resource-definitions/matching-preview` theo OC-04c nhận candidate
chưa lưu và context tường minh từ editor chung ở cả hai form. Profile/env_type/
target do server load; payload criteria/driver values không dịch. 200 matching
statuses tách khỏi 4xx/5xx transport/validation errors. Kết quả không đảm bảo Save
hoặc Deploy và không thay POST registration; retry không tự replay registration.

## Lỗi theo code — T20

Áp dụng [public HTTP error contract](../../../architecture/contracts/operation-contracts.md#public-http-error-contract--t20--vi-03)
và [shared error presentation](../../../architecture/ui/README.md#trình-bày-lỗi-theo-code--t20--vi-03).
Known code chọn hướng dẫn Việt cụ thể; unknown/missing code dùng fallback Việt
an toàn, không hiển thị/parse raw `error`, submitted values hoặc provider cause.
Status/scoping và guards hiện hành giữ nguyên. Giữ draft/confirmation theo flow;
BUSY không mất edits, stale cần reload/review hoặc Preview mới. Retry tường minh
không tự replay mutation; lỗi tải lại không phủ nhận kết quả đã commit.

## Capability metadata projection (T31)

GET `/api/v1/resource-definition-capabilities` theo OC-04d cung cấp authoritative
supported pairs và variable metadata; Console không fallback whitelist khi lỗi.
Typed và advanced serialize cùng `driverInputs.values.variables`, source identity/revision/digest do PE chọn; output/provider mappings
được kiểm độc lập. Provision/criteria và org/version guards giữ nguyên.
Omitted default/param không được tự thêm; literals số/bool/list/map giữ kiểu JSON,
whole placeholders giữ string để deferred checking. Presets là lựa chọn explicit.

## Verified source pins cho Definition form — I04-07

OC-04d metadata cho PE/Admin phải công bố đủ prepared source/module revision/
subdirectory/content SHA-256 theo
[module source shape](../../../architecture/contracts/terraform-modules.md#driver-inputs-shape),
input/output contract/mapping choices và provider requirements/class metadata.
Console dùng đúng pins đã kiểm chứng; không nhận Admin trust root/source URL
mới hoặc tự đoán revision/digest/default. Source unavailable/drift chặn thao tác
cần metadata, giữ edits và retry read; không silent fallback sang packaged module.
Query không fetch/inspect; preparation/registration inspection vẫn theo OC-04.
Driver Inputs tiếp tục full `values.source`, input_mapping/output_mapping và
provider_bindings đã định nghĩa; Workload form không chọn những fields này.

## V02 API replacement

Use OC-04/04c/04d V02 replacement contract: strict `name`, `driverAccount`,
positive response `version`; POST create, PUT key edit with expected version.
Remove `executionProfile`/`connectionKey` authoring from all Definition consumers,
including rendering templates. Account selector uses V01 `/api/v1/driver-accounts`;
filter compatible kind/READY for UX and independently validate server-side.
AWS provider `{target: account}` with region in Driver Inputs; Kubernetes
resource reference keeps inherited target. Do not add account criteria or infer
from Environment. Matching endpoint remains read-only and authoritative.


## V03 UI/API replacement

[ADR-016](../../../architecture/decisions/ADR-016-definition-selected-workload-targets.md) defines current internal placement. Remove deployment Connection selection after replacement pass; keep Secret Store/configuration/version/error/operation state. Definition authoring supports existing-cluster account choice and workload namespace/cluster references, Type params and provision flags; consumers inherit account. Score import/edit/export preserves metadata.class. Preview shows each workload/resource Definition/account/cluster/namespace and provider-first graph/batches without secrets, and blocks missing/ambiguous/mismatched/unready inputs as appropriate. No Environment target is required. Retired deployment-Connection writes fail explicitly; safe matching and stale-token gates remain. UI multi-cluster V03 unsupported error occurs before execution. Progress/readiness/errors/update/remove use per-workload targets; failure never shown as success.


## V04–V06 current target/dependency behavior

[ADR-017](../../../architecture/decisions/ADR-017-definition-dependency-execution.md)
replaces remaining profile/Environment-target requirements above for the
replacement path.

Keep existing role/scoped Definition/Preview/Deploy operations. Registration
and planning accept typed cluster provenance produced by a matched verified
EKS Definition as well as Internal existing-cluster. Public DTOs show safe
identity/provenance/status; no token/kubeconfig/private secret references.
Deployment profile/Environment account fields do not select infrastructure.
Legacy writes/fields retire under V05.10/V06.08 gates; no standalone no-workload
execution endpoint is introduced.


## Git module form — issue #7

UC-03 GIT-01..06 thay giới hạn selector-only trước đây. Chọn module verified
hoặc Thêm từ Git (HTTPS URL, thư mục, tag/commit); nút Kiểm tra module gọi
POST `/api/v1/terraform-modules/inspect` với `{resourceType,url,path,ref}`.
200 trả `{capability}`; 400 invalid/unavailable module, 401 session, 403 role,
413 oversized; lỗi nội bộ dùng generic 500, không raw Git stderr/URL credential.
Loading/error giữ draft; thay source/type/mode hoặc unmount vô hiệu late reply.
Save chặn tới verified selection; inspect không lưu Definition hay provision.
Form giữ typed mappings/variables/provision/criteria và CAS hiện hành. Selector
không hiện digest; Chi tiết kỹ thuật đóng mặc định chứa full pin. Public HTTPS
không cần credential; chưa có private Git auth/SSH. New Type có thể chọn trước
khi module đầu tiên được inspect. Giữ create/edit/reload và installed module flow.

## V3 projection — issue #8

UI3-01..04 chỉ đổi layout và projection authoring, không đổi endpoint/DTO.
Rule editor serialize cùng năm fields/criteria; zero cases không thành wildcard.
Review draft và Save dùng cùng builder cho `driverInputs.values`, `provision`,
`criteria` và expected `version` khi edit. Mapping/variables/provision dùng
converters và strict validation hiện hành; source từ capability/Git API thật.
Disclosures và tabs không phát sinh write, inspect hay matching tự động.

## Module forms projection — issue #11

UI11-01..05 không đổi endpoint/DTO. Tài khoản từ GET `/driver-accounts`; provider,
binding, biến, executor-owned và region variable từ GET
`/resource-definition-capabilities`; Loại tài nguyên, class, inputs/outputs từ GET
`/resource-types`. Form chỉ sinh `driverInputs.values.variables`,
`provider_bindings`, `input_mapping` và `provision`; server vẫn kiểm tra strict.

## Unified cluster authoring — 2026-10-21

UI3-05: Both methods use the existing GET/POST/PUT resource-definitions endpoints and capability/account metadata. Existing-cluster submits k8s-cluster, driver existing-cluster, its Internal driverAccount and values.variables = {}; no Terraform source, region or credentials. Matching API and expected-version CAS remain unchanged.

## Git render-bundle mapping — ADR-021

[RB-C07](../../../architecture/contracts/git-render-bundles.md#rb-c07--http-operations-and-errors)
is canonical for exact request/response/error shapes.

| UI action | API |
|---|---|
| Kiểm tra bundle | POST `/api/v1/render-bundles/inspect` with url/path/ref |
| Load source choices | GET `/api/v1/render-bundles` (legacy + same-org Git metadata) |
| Reopen saved Git source | GET `/api/v1/render-bundles/{id}` |
| Register/save template | Existing POST/PUT resource-definitions with verified render_bundle and render_inputs; expected version on PUT |
| Compare/apply policy change | Existing pending Preview/Deploy; no separate render/deploy endpoint |

Changing source/inputs changes draft signature; pin/schema and field state must
come from the same response. Client cannot provide digest/engine overrides.

## ZIP inspection

ZIP-01..04 use `POST /terraform-modules/upload` and `POST /render-bundles/upload`
with multipart file/path and scoped resourceType for Terraform. Success reuses
existing inspect projections; Definition Create/CAS payloads are unchanged.
