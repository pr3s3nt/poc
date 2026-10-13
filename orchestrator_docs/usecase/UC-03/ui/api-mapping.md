---
id: UC-03-UI-API
artifact: use-case-api-mapping
status: current
last_reviewed: 2026-10-14
related: UC-03
---

# UC-03 HTTP mapping

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
