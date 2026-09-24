---
id: UC-01-UI-API
artifact: use-case-ui-api-mapping
status: current
last_reviewed: 2026-09-23
related: UC-01, OC-01
---

# UC-01 UI API mapping

The UI never submits Organization ID, role, profile, Connection or credential.
Backend derives Organization identity from UC-00 session and resolves platform
defaults through OC-01.

| UI action | Proposed HTTP contract | Success handling | Failure handling |
|---|---|---|---|
| Load Applications home | `GET /api/v1/applications` | Render Applications belonging to session Organization. | `401` redirects Sign in; other failures show retryable list error. |
| Submit create form | `POST /api/v1/applications` with Name/Subdomain | Navigate to created Application home; selected tab is `staging`. | `400` maps field errors; `409` identifies duplicate Name/Subdomain; others retain form and show API error. |
| Load Application home | `GET /api/v1/applications/{applicationId}` | Render Application, two Environment summaries, Workloads and recent deployments. | `404` shows scoped not-found state; `401` redirects Sign in. |

Workload actions intentionally have no M00-a HTTP mapping. UC-16 owns the
configuration contracts; UC-05 owns preview and UC-06/UC-07 own runtime
application. Those contracts must be designed before the buttons become active.
