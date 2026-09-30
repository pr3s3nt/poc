---
id: UC-09-UI-API
artifact: use-case-ui-api-mapping
status: current
last_reviewed: 2026-09-30
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
