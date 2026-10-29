---
id: UC-18-UI-API
artifact: use-case-ui-api-mapping
status: current
last_reviewed: 2026-10-27
related: UC-18, ADR-025
---

# UC-18 UI API mapping

Paths use `/api/v1/applications/{applicationId}` and UC-00 session scope.
Missing session `401`, foreign/missing/`DELETED` Application `404`, invalid body
`400`, stale token/version or a running deletion `409`. A blocked preview is a
`200` with `blocked: true`, per-Environment blockers and no token; a confirmation
that meets new blockers is `409 APPLICATION_DELETION_BLOCKED` with blocker codes.

| UI operation | Method/path suffix | Request / result |
|---|---|---|
| Load preview | `POST /deletion-preview` | `{}` → `{token, environments:[{key, workloads, resources, blockers}], kept, warnings, confirmationName}` |
| Confirm / Retry | `POST /deletions` | `{token, confirmationName}` → `202 {operationId, status}`; same token repeats return the same operation |
| Follow progress | `GET /deletions/{operationId}` | Per Environment stage/status, per-instance results, remaining IDs, safe error codes |

While the Application is `DELETING`, every existing mutation endpoint under
the Application returns `409` with code `APPLICATION_DELETING`. Reads continue.
Responses never contain secret values, Terraform state or credentials.
