---
id: UC-04-UI-API
artifact: use-case-api-mapping
status: current
last_reviewed: 2026-10-17
---

# UC-04 Connection HTTP mapping

All operations require a session and Platform Engineer/Admin in the session's
Organization. Clients cannot select another Organization in request data.

| Method/path | Body/result | Mutation |
|---|---|---|
| `GET /api/v1/connections` | Public Connection DTO list | None |
| `POST /api/v1/connections/kubernetes/inspect` | `{kubeconfig}` -> `{contexts:[{name,cluster,endpoint}]}` | None; no cluster access |
| `POST /api/v1/connections/kubernetes` | `{name,kubeconfig,context}` -> public Connection DTO | Verify then secret write + insert READY |
| `POST /api/v1/connections/aws` | `{name,accessKeyId,secretAccessKey,allowedRegions}` -> safe account/principal/region metadata | Verify identity/region/capabilities, lưu Vault ref, insert READY |

The form and API use only the upload contract. The old
`{key,clusterId,kubeContext}` body and mixed documents return 400 without
verification, credential writes or Connection insertion.
The public DTO includes key/name/kind/authenticationType/config/status and safe
verification metadata. It excludes secretRef, credentials, kubeconfig and raw
provider results. UI chỉ hiển thị safe READY metadata; AWS key không xuất hiện trong read DTO.
AWS registration không nhận Terraform backend config; executor không lấy host key.

Known-field JSON object; document max 1 MiB and envelope bounded to accommodate
JSON escaping (8 MiB ceiling). Unknown fields, mixed shapes and trailing JSON
are rejected. Inspect does not trust file extension and register does not trust
prior inspection or client-supplied metadata. Context is explicit on register;
UI auto-selects when only one is present.

Success GET/inspect 200, register 201; invalid 400,
verification rejection 422, missing session 401, wrong role 403, oversized 413,
unavailable credential store 503, unexpected storage failure generic 500.
Generated key collisions are resolved without overwriting prior connections.
Safe fixed context/API/permission guidance only; no raw parser/kubectl/Vault
errors. Non-READY records and failures do not disclose sensitive fields.

See [shared credential design](../../../architecture/connection-credentials.md)
for store/executor scope and fail-closed authentication.

## T06 — Projection trang Kết nối (không đổi API)

Upload/paste auto-inspect dùng POST `/connections/kubernetes/inspect` ở bảng
trên với `{kubeconfig}`; revision/request guard chỉ thuộc frontend. Inspect
không verify hoặc ghi credential. Register vẫn chỉ gửi `{name,kubeconfig,context}`
khi người dùng bấm “Kiểm tra và lưu”; server tự parse/validate/verify lại.
Không gửi UI source, timer, suggested-name ownership hay client summary.

Gợi ý tên từ safe context metadata chỉ là draft, không thay key generation
hoặc Organization defaults. Các giới hạn document/name hiện hành giữ nguyên.
Trang dùng hướng dẫn tiếng Việt cố định theo status/operation và fallback an
toàn, không hiển thị raw English/provider error; stable code mapping theo contract lỗi T20.
Khi inspect response mất hoặc đến muộn không tự register. Với register network
failure, thông báo chưa biết đã lưu hay chưa và reload list trước retry.

## Secret Store Connection API

PE/Admin GET/POST `/api/v1/secret-stores`, strict bounded register payload:
name/provider/backendAddress/workloadAddress/mount/authMount/TLS/token. Only
VAULT_KV_V2/token is supported initially. Create verifies attempt-owned probe and
capabilities, cleans probe, stores credential privately and insert-only metadata;
201 only after all required steps. Errors remain safe as above.
Developer GET `/api/v1/secret-store-choices` returns scoped READY metadata only.
No token, credential ref, legacy flag/verification marker, arbitrary secret-read
or delete endpoint is exposed (T23). Ordinary READY store choices keep the same
keys, provider, status and explicit Environment selection contract.

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
code mapping toàn Console theo contract lỗi T20. Không mở rộng Developer choices DTO.

## Lỗi theo code — T20

Áp dụng [public HTTP error contract](../../../architecture/contracts/operation-contracts.md#public-http-error-contract--t20--vi-03)
và [shared error presentation](../../../architecture/ui/README.md#trình-bày-lỗi-theo-code--t20--vi-03).
Known code chọn hướng dẫn Việt cụ thể; unknown/missing code dùng fallback Việt
an toàn, không hiển thị/parse raw `error`, submitted values hoặc provider cause.
Status/scoping và guards hiện hành giữ nguyên. Giữ draft/confirmation theo flow;
BUSY không mất edits, stale cần reload/review hoặc Preview mới. Retry tường minh
không tự replay mutation; lỗi tải lại không phủ nhận kết quả đã commit.

## AWS request bounds và verification adapter — I04-06

POST AWS tối đa 16 KiB, exact-case known fields, không duplicate/null/mixed shape
hoặc trailing JSON. Name theo giới hạn Connection hiện hành; initial access key
là AKIA 20 ký tự, secret 40 ký tự; allowedRegions 1–10 vùng khác nhau, bounded
region syntax. Initial verifier chỉ hỗ trợ long-term IAM user, không root hoặc
session-token input. Root/temporary/capability-unverifiable trả safe 422, không READY.
Identity, từng region read probe và IAM policy simulation đều phải đạt; key cần
`iam:SimulatePrincipalPolicy` trên principal của mình. IAM global context dùng
us-east-1, service regional dùng từng vùng đích. Vùng đầu tiên là default, full
allowed list được giữ trong read metadata và checked khi binding. Registration
không tạo hạ tầng; execution preflight kiểm lại trước apply theo runbook.


## V01 infrastructure account API

All routes use authenticated session Organization and PE/Admin gates. A foreign
key returns scoped 404; clients cannot send Organization, secretRef, profile or
applicability. Strict bounded known-field JSON and existing credential limits
apply; reject duplicate/null/mixed/unknown fields and trailing documents. New
account type is `AWS`/`INTERNAL`, separate from authenticationType.

| Method/path | Request/result | Effect |
|---|---|---|
| `GET /api/v1/driver-accounts` | `{accounts:[Account]}` | Safe Organization catalog |
| `GET /api/v1/driver-accounts/{key}` | `Account` | Safe detail/version |
| `POST /api/v1/driver-accounts/internal/inspect` | `{kubeconfig}` -> safe contexts | Parse only |
| `POST /api/v1/driver-accounts/internal` | `{name,kubeconfig,context}` -> `Account` | Verify, immutable secret, create version 1 |
| `POST /api/v1/driver-accounts/aws` | `{name,accessKeyId,secretAccessKey,verificationRegions}` -> `Account` | Read-only verify, immutable secret, create version 1 |
| `PUT /api/v1/driver-accounts/{key}` | `{name,version}` -> `Account` | Rename via CAS |
| `POST /api/v1/driver-accounts/{key}/verify` | `{version,verificationRegions?}` -> `Account` | Recheck stored secret; CAS on success |
| `PUT /api/v1/driver-accounts/{key}/credentials` | Internal `{version,kubeconfig,context}` or AWS `{version,accessKeyId,secretAccessKey,verificationRegions}` -> `Account` | Same-target verified replacement via CAS |

`Account` includes id/key/name/type/authenticationType/status/version and safe
config/verification. It excludes private ref and credential; infrastructure type
cannot be changed by update. AWS `config.verificationRegions` and safe account/
principal metadata describe checks; do not expose `allowedRegions`/default-region
fields as new account policy. Internal config whitelists selected cluster/context/
endpoint and safe TLS metadata (`config.tls.mode`, optional `serverName`);
`verification.verifiedAt` records the last successful check. Seed markers without
a stored credential remain visible but cannot be verified until registered. Legacy `/connections` DTO is unchanged except
optional version for shared backing records; it retains legacy kind/region fields.

AWS verify may omit verificationRegions to recheck the stored evidence list;
Internal verify must reject this AWS-only field. Credential replacement is
kind-specific and rejects mixed forms. Same-target checks prevent account-ID or
Internal target/context/endpoint/trust redirection; require a new account instead.
Verify failures do not change committed status/ref/version; UI must report the
fresh failure separately. No claim of live health solely from READY.

GET/inspect/edit/verify/replace success 200; create 201. Invalid 400, auth 401/403,
scoped missing 404, stale 409 with stable stale code, body limit 413, verification
or incompatible target 422 (`ACCOUNT_TARGET_MISMATCH` for target changes,
`CONNECTION_UNAVAILABLE` for missing stored credentials), unavailable credential
store 503, uncertain database commit 503 `COMMIT_UNCERTAIN`, unexpected storage
500. Use existing public error contract with safe stable codes/fields, no raw
provider details. `COMMIT_UNCERTAIN` requires GET recovery and explicit user
review before another mutation; retain any potentially committed credential.
Replacement AWS keys are also checked in every retained legacy allowed region
before publication. No account delete/read-secret API. Consumers/driver selection
and deployment region move to the new model in V02–V03, not V01.

## EC2 role API [EC2-01..05]

`POST /api/v1/driver-accounts/aws/ec2-role` strictly accepts
`{name,roleArn,verificationRegions}` and returns the safe Account DTO (201).
`authenticationType=AWS_EC2_ROLE`; config adds `roleArn` and `roleId`.
Existing GET/rename/verify use the same role/org/version gates and error mapping.
`PUT .../{key}/credentials` rejects EC2 role accounts (ACCOUNT_TARGET_MISMATCH).
No metadata endpoint, account ID, credential or secret reference can be submitted.

## Diagnostic HTTP — DIAG-01..14

Account list/detail use the safe latest-diagnostic projection. Start/list/read/
cancel use `/api/v1/driver-accounts/{key}/diagnostics` and its report subroutes
from the [canonical contract](../../../architecture/contracts/account-diagnostics.md#api).
All operations retain PE/Admin and session Organization boundaries. Scope includes
account version/purpose/mode/region/use-case/context pins, never credentials or
network commands. Browser export serializes only the safe report response.

UI polls backend state and persisted progress; no synthetic run timer. Error
mapping follows the existing safe error envelope and VI/EN catalog patterns.
Unknown start outcome reconciles GET rather than resubmitting. Account switches
and stale response guards apply equally to report, history and scope loaders.
