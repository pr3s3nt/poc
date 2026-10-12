---
id: UC-09-UI-API
artifact: use-case-ui-api-mapping
status: current
last_reviewed: 2026-10-13
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

## Terraform v2 contract extension

Existing UI routes and role/org/version guards remain. Required payload/metadata
extensions follow [module contract](../../../architecture/contracts/terraform-modules.md),
[state contract](../../../architecture/contracts/terraform-state.md) and
[operation contracts](../../../architecture/contracts/operation-contracts.md#terraform-v2-scoped-supersession-adr-014).
These are planned v2 fields, not claims the current HTTP handlers accept them.
Workload params/class stay Score-backed references; PE source/revision/mapping
fields are Definition metadata. AWS onboarding belongs to dedicated concealed
Connection write. Public queries return only scoped safe projections; no internal
state endpoint/token exposed. No API bypass of acceptance UI steps is allowed.
