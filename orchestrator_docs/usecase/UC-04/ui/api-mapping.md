---
id: UC-04-UI-API
artifact: use-case-api-mapping
status: current
last_reviewed: 2026-09-30
related: UC-04
---

# UC-04 local Kubernetes HTTP mapping

`GET /api/v1/connections` lists the session Organization's Connections for
Platform Engineer/Admin. The Console shows READY records with ID, kind, cluster
ID, kube context and status. `POST /api/v1/connections/kubernetes` takes only
`key`, `clusterId`, `kubeContext`: one bounded known-field JSON object; no
credential upload. Existing host context verification is read-only.
Success 201, invalid document 400, duplicate 409, verification rejection 422,
missing session 401, wrong role 403, oversize 413. Verification failures use fixed
safe guidance (context/API/permissions), never raw kubectl/provider stderr.
Unknown store causes return generic 500. Concurrent duplicates cannot replace
existing metadata/reference. A new READY Connection can be referenced by a
Definition without restart. Host migration still requires configuring the same
context; this does not implement durable credential storage or an AWS endpoint.
