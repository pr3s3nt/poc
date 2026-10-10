---
id: UC-03-UI-API
artifact: use-case-api-mapping
status: current
last_reviewed: 2026-10-10
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
are visible to planning without restart. No remote execution or secret Driver
Inputs are added by this mapping.

## Registration policy

Definition ID and Driver Inputs follow specification BR-10–BR-14. Unknown nested keys, unsupported variables, nulls and wrong literal types return 400 with a safe field path; advanced JSON preserves valid placeholders. Form retains input on failure.

## Workload renderer variant (ADR-010)

The same Definition registration endpoint accepts Type `workload`, driver
`score-k8s`, profile `internal-k8s`, and `values.variables.render_bundle`.
The Console supplies an installed bundle ID and hides connection/provision/
arbitrary-variable inputs for this variant. Server-owned `sourceFingerprint`
pins the bundle; unavailable bundles and unsupported pairs are validation errors.

## Resource form projection (T02)

Form Cấu hình tài nguyên dùng GET `/resource-types`, `/resource-definitions`
và `/connections`; các list do server scope theo Organization của phiên.
Lọc workload khỏi list/type selector và không cung cấp driver existing-cluster
trong form. Đây là projection UI; public API compatibility chỉ đổi tại T17,
workload renderer contract phía trên vẫn được giữ cho T03.

POST giữ body/semantics hiện hành: Terraform gửi `executionProfile: "aws-eks"`,
`connectionKey` AWS READY và embedded `source.module` theo type; Kubernetes gửi
`executionProfile: "internal-k8s"` hoặc `""`, bỏ `source` và mặc định
`connectionKey: ""`. Override Kubernetes chỉ lấy READY kind KUBERNETES và gửi
key kỹ thuật. Criteria giữ năm field, loại field rỗng; wildcard gửi `[{}]`.
Không thay matcher, backend defaults, seed hay Driver Inputs policy.

Frontend kiểm ID rỗng/shape và JSON trước POST; 409 có thông báo trùng ID Việt.
Các lỗi khác có fallback Việt an toàn và giữ form; không parse/regex raw backend
English message. Mapping field/code đầy đủ thuộc T20.
