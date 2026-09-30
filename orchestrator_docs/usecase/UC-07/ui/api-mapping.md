---
id: UC-07-UI-API
artifact: use-case-ui-api-mapping
status: current
last_reviewed: 2026-09-30
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
