---
id: UC-02-UI-API
artifact: use-case-api-mapping
status: current
last_reviewed: 2026-10-02
related: UC-02
---

# UC-02 HTTP mapping

`GET /api/v1/resource-types` lists the session Organization's catalog.
`POST /api/v1/resource-types` registers one contract, Platform Engineer/Admin
only. Body contains `key`, `inputs` and `outputs` in the supported MVP schema;
one bounded JSON object, unknown fields/trailing/non-object bodies rejected.
Success is 201, validation 400, duplicate 409, missing session 401, wrong role
403, oversize 413. Unknown store errors return generic 500 without raw cause.
Registration never updates an existing contract, even with concurrent requests.
New contracts are available to Score validation without restart. Runtime matching
still requires a supported Definition; registration does not add a new executor.

## Registration policy

New IDs follow specification BR-05; `environment` and `service` are reserved (BR-06). Invalid IDs return 400 without normalization; form retains user input and explains lowercase letters, digits and hyphens.
