---
id: UC-07-UI-API
artifact: use-case-ui-api-mapping
status: current
last_reviewed: 2026-10-13
related: UC-07, UC-16, OC-09, OC-17
---

# UC-07 UI API mapping

All operations below use `/api/v1/applications/{applicationId}/environments/{environment}`
and require UC-00 session-derived Organization scope. Missing session `401`,
foreign/missing scope `404`, invalid input `400`, stale version/token `409`.
Mutation bodies contain exactly one bounded JSON object with known fields.
Versions are required nonnegative integers, not omitted/null defaults.

| UI operation | Method/path suffix | Request / result |
|---|---|---|
| Load workload editor/list | `GET /workloads` | Current/pending Score, workload state, draft version. |
| Save edit | `PUT /workloads/{workloadId}` | `{score,version}`; pending upsert, no runtime change. |
| Confirm pending deletion | `DELETE /workloads/{workloadId}` | `{version}`; pending deletion for deployed workload. New undeployed draft is simply discarded. |
| Undo pending deletion | `POST /workloads/{workloadId}/undo` | `{version}`; restore prior pending edit if present, otherwise current deployed workload. |
| Preview changes | `POST /preview` | `{}`; pinned pending token, update/remove actions and resource changes. |
| Deploy preview | `POST /deploy` | `{token}`; per-workload `SUCCEEDED`/`FAILED`/`SKIPPED` and batch summary. |

On conflict reload current list/version and ask the user to preview again;
never silently retry mutation with a fresh version. A partial deployment keeps
successful workload commits and leaves unfinished drafts; retry requires a new
Preview. Route-only retry remains the UC-06 contract and does not redeploy
workloads. Safe errors must not disclose credential/resolved secret values.
History/detail uses the separate scoped UC-09 reads.
New failed Deployment records store a safe failure summary, not raw runtime
adapter errors that may contain credentials. Internal error kinds remain
available for stale/conflict control flow. Safe summaries also appear in the
scoped UC-09 history/detail used to review update/remove results.
Unknown historical raw failure reasons are replaced in the query projection
with generic safe guidance; existing persisted history is not rewritten.

## Lỗi theo code — T20

Áp dụng [public HTTP error contract](../../../architecture/contracts/operation-contracts.md#public-http-error-contract--t20--vi-03)
và [shared error presentation](../../../architecture/ui/README.md#trình-bày-lỗi-theo-code--t20--vi-03).
Known code chọn hướng dẫn Việt cụ thể; unknown/missing code dùng fallback Việt
an toàn, không hiển thị/parse raw `error`, submitted values hoặc provider cause.
Status/scoping và guards hiện hành giữ nguyên. Giữ draft/confirmation theo flow;
BUSY không mất edits, stale cần reload/review hoặc Preview mới. Retry tường minh
không tự replay mutation; lỗi tải lại không phủ nhận kết quả đã commit.

## API Xóa tài nguyên — OC-19

Các path sau nằm dưới prefix Application/Environment ở trên. Root ID là
resource instance thuộc scope; Organization/role lấy từ session.

| UI operation | Method/path suffix | Request / result |
|---|---|---|
| Review phạm vi xóa | `POST /resources/{resourceId}/deletion-preview` | `{}`; 200 với token, confirmationName, root + dependency candidates, consumer blockers, versions/expiry. Chỉ đọc metadata; không provider/secret call. |
| Xác nhận xóa | `POST /resources/{resourceId}/deletions` | `{token,confirmationName,selectedResourceIds}`; 202 với operationId và REQUESTED sau atomic admission. Danh sách IDs bắt buộc có root, thuộc candidates trong token; server kiểm lại toàn bộ tập, không tự thêm mục hoặc nhận quyền run từ caller. |
| Theo dõi | `GET /resource-cleanups/{operationId}` | 200 safe persisted stages, confirmed IDs, per-instance verification/remaining IDs. |

Preview/execute chỉ PE/Admin; 401 thiếu session, 403 Developer, 404 foreign scope,
400 payload/tên xác nhận sai, 409 stale/busy/ineligible, 503 thiếu state/module/
credential service. Token pin root/scope/candidates/versions; server kiểm exact
confirmationName từ descriptor, không echo tên sai trong error. Execute lặp
cùng token của cùng actor trả operation đã nhận, không tạo destroy thứ hai.
Nếu graph/consumer/version đổi thì review lại trước mọi provider call.
Các read query được phân quyền theo UC-09; token không phải credential state backend.
