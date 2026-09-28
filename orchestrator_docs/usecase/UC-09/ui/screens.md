---
id: UC-09-UI-SCREENS
artifact: use-case-ui-screens
status: current
last_reviewed: 2026-09-28
related: UC-09
---

# UC-09 UI screens

Application home shows recent deployments for the selected staging or
production Environment. Each row has status, action, workload and time; a
`View all` action opens Environment deployment history.

History lists deployments newest first, with a status filter and a link to each
detail. Detail displays persisted Deployment status and failure reason, actor,
timestamps, workload/resource status, graph matches and provision batches.
Secret outputs remain redacted by the backend; the UI does not request cluster
logs or perform runtime refresh.
