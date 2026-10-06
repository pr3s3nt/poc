---
id: UC-03-UI-API
artifact: use-case-api-mapping
status: current
last_reviewed: 2026-10-02
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
