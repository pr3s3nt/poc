---
id: UC-09-UI-API
artifact: use-case-ui-api-mapping
status: current
last_reviewed: 2026-10-23
related: UC-09, OC-11
---

# UC-09 UI API mapping

Both read operations require the UC-00 session. Organization is derived from
that session; the client supplies only the selected Application/Environment
path. A Deployment outside that complete scope returns `404`, not tenant
existence information.

| UI action | HTTP contract | Success | Failure |
|---|---|---|---|
| Load recent/history | `GET /api/v1/applications/{applicationId}/environments/{environment}/deployments?status={optional}` | `200`; deployments newest first, optionally filtered by one lifecycle status. | `400` invalid status; `401` expired/missing session; `404` Application/Environment outside scope; other errors are retryable. |
| Load detail | `GET /api/v1/applications/{applicationId}/environments/{environment}/deployments/{deploymentId}` | `200`; persisted metadata, plan artifacts, deployment resource outputs and workload snapshots for that Deployment. | `401` expired/missing session; `404` any scope mismatch/missing Deployment; other errors are retryable. |

Detail never triggers runtime refresh. It omits resource resolved inputs and
returns secret outputs only as the canonical redacted marker. Planning failure
may legitimately return no Delta, graph, matches, batches or workload rows.
History and detail project safe failure summaries, not unknown raw legacy
adapter error text; the read operation never rewrites persisted history.

## Lỗi theo code — T20

Áp dụng [public HTTP error contract](../../../architecture/contracts/operation-contracts.md#public-http-error-contract--t20--vi-03)
và [shared error presentation](../../../architecture/ui/README.md#trình-bày-lỗi-theo-code--t20--vi-03).
Known code chọn hướng dẫn Việt cụ thể; unknown/missing code dùng fallback Việt
an toàn, không hiển thị/parse raw `error`, submitted values hoặc provider cause.
Status/scoping và guards hiện hành giữ nguyên. Giữ draft/confirmation theo flow;
BUSY không mất edits, stale cần reload/review hoặc Preview mới. Retry tường minh
không tự replay mutation; lỗi tải lại không phủ nhận kết quả đã commit.


## Resource detail và cleanup query

| UI action | HTTP contract dưới Application/Environment prefix | Kết quả |
|---|---|---|
| Mở tài nguyên của Environment | `GET /resources` | 200 `resources`: safe IDs/descriptor/type/class/status/scope/generation/module để mở detail; gồm UNREFERENCED và lịch sử instance, không chỉ graph hiện tại. 404 ngoài scope; lỗi đọc không biến thành danh sách rỗng. |
| Đọc chi tiết tài nguyên | `GET /resources/{resourceId}` | 200 safe descriptor/status/module/target/owner/consumers/eligibility; 404 ngoài scope. |
| Đọc tiến trình xóa | `GET /resource-cleanups/{operationId}` | 200 confirmed scope, stages/timestamps/actor, verification/remaining IDs; không state bytes. |

Query cần quyền đọc Environment, lấy Organization từ session. Developer chỉ
đọc safe metadata; PE/Admin mutation ở [UC-07 API](../../UC-07/ui/api-mapping.md).
Eligibility chỉ là snapshot, không token cấp quyền hoặc bảo đảm Execute;
Operation/result scope cố định, không lấy cleanup của Environment khác.

I04-08 hiện thực đường từ Environment và lịch sử Deployment sang resource
detail. Deployment progress có thể mang `resourceInstanceId` đã kiểm cùng
Environment để mở detail; bản ghi lịch sử không có association không được đoán
ID hoặc thay bằng instance ở generation mới. List/detail/progress chỉ đọc
metadata từ Console; không provider refresh, secret resolution hoặc state blob.
Thiếu dịch vụ dùng cho mutation thì eligibility bị khóa với hướng dẫn an toàn.

## OBS review/progress additions

Existing deployment list accepts optional `reviewRequestId` exact-match filter,
validated as a bounded identifier and always combined with full session scope.
Existing detail adds safe `review`, `nodeProgress` and manifest review association
from its own persisted records under the shared contract. Missing old artifact
is UNAVAILABLE, never reconstructed from current catalog or mutable instances.
Before admission, a filtered empty list means no execution records yet. Poll only
exact returned IDs; do not fall back to latest deployment. Response errors use
existing safe error handling. No provider refresh or new mutation endpoint.

## Application telemetry

Overview, logs and metrics use the authenticated scoped GET endpoints in
[the telemetry contract](../../../architecture/contracts/observability.md#read-api).
Polling reads stored observations, never invokes the collector. Page cursors and
ISO time bounds are opaque/validated; no client-supplied organization or target.
