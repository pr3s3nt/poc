---
id: UC-04-UI-API
artifact: use-case-api-mapping
status: current
last_reviewed: 2026-10-11
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

## T06 — Projection trang Kết nối (không đổi API)

Upload/paste auto-inspect dùng POST `/connections/kubernetes/inspect` ở bảng
trên với `{kubeconfig}`; revision/request guard chỉ thuộc frontend. Inspect
không verify hoặc ghi credential. Register vẫn chỉ gửi `{name,kubeconfig,context}`
khi người dùng bấm “Kiểm tra và lưu”; server tự parse/validate/verify lại.
Không gửi UI source, timer, suggested-name ownership hay client summary.

Gợi ý tên từ safe context metadata chỉ là draft, không thay key generation
hoặc Organization defaults. Các giới hạn document/name hiện hành giữ nguyên.
Trang dùng hướng dẫn tiếng Việt cố định theo status/operation và fallback an
toàn, không hiển thị raw English/provider error; stable code mapping thuộc T20.
Khi inspect response mất hoặc đến muộn không tự register. Với register network
failure, thông báo chưa biết đã lưu hay chưa và reload list trước retry.

## Secret Store Connection API

PE/Admin GET/POST `/api/v1/secret-stores`, strict bounded register payload:
name/provider/backendAddress/workloadAddress/mount/authMount/TLS/token. Only
VAULT_KV_V2/token is supported initially. Create verifies attempt-owned probe and
capabilities, cleans probe, stores credential privately and insert-only metadata;
201 only after all required steps. Errors remain safe as above.
Developer GET `/api/v1/secret-store-choices` returns scoped READY metadata only.
No token, credential ref, arbitrary secret-read or delete endpoint is exposed.

## T05 — Projection form Kho bí mật (không đổi API)

GET/POST `/api/v1/secret-stores` và role/org gates giữ nguyên. POST gửi
`name`, `backendAddress`, `workloadAddress`, `mount`, `authMount`, token và
`tlsCaPem` nếu có; không gửi checkbox/UI state hoặc thêm provider vào DTO.
Mount rỗng dùng defaults `kv`/`kubernetes`. Checkbox bật lấy backendAddress
làm workloadAddress trong payload; tắt gửi draft workload độc lập.

Frontend kiểm URL http(s) có host, không userinfo/query/fragment/whitespace, dấu gạch chéo ngược hoặc ký tự điều khiển;
mount là segment 1–64 ký tự chữ/số/gạch ngang/gạch dưới. Tên không rỗng, tối đa
100 ký tự Unicode, không control; token không rỗng, tối đa 4096 byte UTF-8,
không control; CA tối đa 64 KiB UTF-8, chỉ shape CERTIFICATE PEM (base64 blocks,
không nội dung thừa). Tổng JSON envelope tối đa 256 KiB. Backend vẫn là final
guard, parse X.509 và verify thật; FE shape check không chứng minh trust.

GET dùng safe `verification.verified`, `kvVersion`, `capabilities`, `probe`
và `kubernetesAuth` khi có; không suy auth từ READY. Text lỗi trang dùng
hướng dẫn Việt cố định theo HTTP status, không dịch regex/raw error; stable
code mapping toàn Console thuộc T20. Không mở rộng Developer choices DTO.
