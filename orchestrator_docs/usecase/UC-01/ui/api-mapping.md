---
id: UC-01-UI-API
artifact: use-case-ui-api-mapping
status: current
last_reviewed: 2026-10-07
related: UC-01, OC-01
---

# UC-01 UI API mapping

The UI submits only Name, Subdomain and selected connectionKey. Backend derives
Organization identity from UC-00 session and profile/region from the scoped
Connection through OC-01. UC-04 management endpoints remain platform-only.

| UI action | Proposed HTTP contract | Success handling | Failure handling |
|---|---|---|---|
| Load Applications home | `GET /api/v1/applications` | Render Applications belonging to session Organization. | `401` redirects Sign in; other failures show retryable list error. |
| Load connection choices | `GET /api/v1/application-connections` | `{connections: [{key, name, kind, status}], defaultConnectionKey}`; READY Kubernetes/AWS choices (AWS requires nonempty region); default key is empty when ineligible, no config, verification, SecretRef or credential. Authenticated Organization scope. | `401` signs in; retryable loading errors and empty state block submit. |
| Submit create form | `POST /api/v1/applications` with `name`, `subdomain`, `connectionKey` (old callers may omit key); unknown fields are rejected | Navigate to created Application home; selected tab is `staging`. | `400` maps validation to the returned `field`; `409` identifies duplicate Name/Subdomain; `422` with field `connectionKey` reports an unavailable selected/default target; explicit blank/null key returns `400`; all failures retain form input. |
| Load Application home | `GET /api/v1/applications/{applicationId}` | Render Application, two Environment summaries, Workloads and recent deployments. | `404` shows scoped not-found state; `401` redirects Sign in. |

Workload actions intentionally have no M00-a HTTP mapping. UC-16 owns the
configuration contracts; UC-05 owns preview and UC-06/UC-07 own runtime
application. Those contracts must be designed before the buttons become active.

Create/list/get Application views include safe persisted `connectionKey` and
derived profile; no credentials. Application home, standalone Score Preview and
pending Preview/Deploy show this binding for both Environments.
