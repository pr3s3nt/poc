---
id: UC-05-UI-API
artifact: use-case-ui-api-mapping
status: current
last_reviewed: 2026-10-13
related: UC-05, OC-06
---

# UC-05 UI API mapping

| UI action | HTTP contract | Result |
|---|---|---|
| Preview Score | `POST /api/v1/applications/{applicationId}/environments/{environment}/score-preview` | `200` read-only planning artifacts; no persisted preview/token. |

UC-00 session required. Organization comes from session; foreign/missing
Application or Environment returns `404`; missing/expired session `401`.
Invalid JSON/unknown fields/trailing document, invalid action/input combination,
Score validation or planning failure returns `400` with a safe actionable
message. Other service errors are retryable; never include credentials.
An oversized request returns `413`. Public planning errors use safe categories
and guidance, not arbitrary adapter/inspector/catalog error strings. Logs must
not echo raw errors containing credential material.

Strict JSON request: `workloadId` (nonempty string), `action` (`deploy`,
`update`, `remove`), `runId` (nonempty string), `scoreBefore` and `scoreAfter`
(Score objects, omitted when absent). `deploy` means add: after only;
`update` needs both; `remove` needs before only. Request size is bounded.
No organization, connection, base Set, active resources or version override
is accepted from the client. Scope/input validation precedes planner use.

Explicit response fields: `applicationKey`, `environmentKey`, `baseSetId`,
`baseVersion`, `runId`, `workloadId`, `action`, `planHash`, `delta`,
`candidateSet`, `graph`, `matches`, `batches`, `classification`.
Collections use stable empty collections rather than inaccessible null data.
Graph edges remain consumer-to-provider; batches list providers first.
No raw connection, resolved inputs, Terraform credentials or runtime outputs.

The public view redacts literal container variable values belonging to other
workloads using the existing redacted marker, and excludes graph parameter
values that may come from Definition defaults/driver inputs. Caller-supplied
Score placeholders stay symbolic. This projection never changes the internal
Candidate/Delta or plan hash: invariant/parity tests compare the unredacted
planning artifacts, not a redacted rendering. UI labels the response as a
sanitized view rather than an executable Deployment Set.

Existing `POST .../preview` remains UC-16 pending changes; this endpoint
does not save drafts or confer permission to Deploy these changes.

## Implicit cluster binding projection

Each `matches` row keeps `descriptor`, `definitionKey`, `driverType` and
`specificity`. The implicit internal cluster adds `binding` with value
`environment-connection` and safe `connectionKey`; its system Definition key
is `builtin-existing-cluster` and specificity `-1` denotes bypassed matching.
Normal catalog matches omit these optional binding fields. This is internal
product metadata, not a Humanitec matching contract. Credential/reference bytes
and resolved inputs remain excluded. Preview never admits the system record.

## Lỗi theo code — T20

Áp dụng [public HTTP error contract](../../../architecture/contracts/operation-contracts.md#public-http-error-contract--t20--vi-03)
và [shared error presentation](../../../architecture/ui/README.md#trình-bày-lỗi-theo-code--t20--vi-03).
Known code chọn hướng dẫn Việt cụ thể; unknown/missing code dùng fallback Việt
an toàn, không hiển thị/parse raw `error`, submitted values hoặc provider cause.
Status/scoping và guards hiện hành giữ nguyên. Giữ draft/confirmation theo flow;
BUSY không mất edits, stale cần reload/review hoặc Preview mới. Retry tường minh
không tự replay mutation; lỗi tải lại không phủ nhận kết quả đã commit.
