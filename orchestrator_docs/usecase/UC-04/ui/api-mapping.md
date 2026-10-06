---
id: UC-04-UI-API
artifact: use-case-api-mapping
status: current
last_reviewed: 2026-10-06
---

# UC-04 Connection HTTP mapping

All operations require a session and Platform Engineer/Admin in the session's
Organization. Clients cannot select another Organization in request data.

| Method/path | Body/result | Mutation |
|---|---|---|
| `GET /api/v1/connections` | Public Connection DTO list | None |
| `POST /api/v1/connections/kubernetes/inspect` | `{kubeconfig}` -> `{contexts:[{name,cluster,endpoint}]}` | None; no cluster access |
| `POST /api/v1/connections/kubernetes` | `{name,kubeconfig,context}` -> public Connection DTO | Verify then secret write + insert READY |

The new form uses the upload contract. Existing `{key,clusterId,kubeContext}`
registration contract can remain an explicit legacy compatibility boundary;
never accept a mixed legacy/upload document or fall back between them.
The public DTO includes key/name/kind/authenticationType/config/status and safe
verification metadata. It excludes secretRef, credentials, kubeconfig and raw
provider results. UI displays READY only; AWS registration endpoint is deferred.

Known-field JSON object; document max 1 MiB and envelope bounded to accommodate
JSON escaping (8 MiB ceiling). Unknown fields, mixed shapes and trailing JSON
are rejected. Inspect does not trust file extension and register does not trust
prior inspection or client-supplied metadata. Context is explicit on register;
UI auto-selects when only one is present.

Success GET/inspect 200, register 201; invalid 400, duplicate legacy ID 409,
verification rejection 422, missing session 401, wrong role 403, oversized 413,
unavailable credential store 503, unexpected storage failure generic 500.
Generated key collisions are resolved without overwriting prior connections.
Safe fixed context/API/permission guidance only; no raw parser/kubectl/Vault
errors. Non-READY records and failures do not disclose sensitive fields.

See [shared credential design](../../../architecture/connection-credentials.md)
for store/executor scope and legacy compatibility.
