---
id: EXTERNAL-OBSERVABILITY-CONTRACT
artifact: operation-contract
status: current
last_reviewed: 2026-11-05
related: [ADR-040, UC-04, UC-09]
---

# External observability — Slice 1

[ADR-040](../decisions/ADR-040-external-observability.md) selects the architecture.

## Connection and binding

Organization-scoped PE/Admin CRUD/test stores display name, Prometheus backend
endpoint, Alertmanager backend endpoint, Grafana browser URL/dashboard UID,
verified cluster identity, TLS/CA and opaque credential reference. Updates use
optimistic revisions; deletion refuses referenced connections. Public responses
exclude credential bytes and provider errors. Test reports connectivity/auth,
query capability, expected Kubernetes metric coverage and rule synchronization
separately. Successful HTTP alone proves none of the latter three.

Environment binding pins connection revision and physical cluster UID from the
existing authorized deployment target. Binding does not replace placement or CD
selection. A changed cluster requires re-verification. Binding cannot import a
foreign organization or bypass Environment operation fencing.

## Bounded reads

Use Prometheus `/api/v1/query`, `/api/v1/query_range`, `/api/v1/rules` and
Alertmanager `/api/v2/alerts` through a private adapter. Request deadline 10s,
response maximum 4 MiB, 2000 series and 240 samples per series; historical range
at most 15 days, step at least 30s and derived to preserve the sample bound.
Reject malformed, nonfinite, oversized, redirected and partial provider responses
with safe codes. Query values use structured URL encoding and quoted server-owned
selector values. No request accepts raw PromQL or arbitrary tenant selectors.
Only explicitly configured credentials are resolved; no ambient host credentials.

Every data response identifies source, selected interval, actual observation
timestamps and coverage. Empty, not configured, not deployed, waiting, missing
metrics, stale, partial and failed states are distinct. Stale threshold is 90s
for the dev 30s scrape interval. Null chart gaps remain gaps. CPU is mCPU; RAM is
bytes; limits are nullable and only used when coverage is complete. Restart counts
use increase over the selected window. OOMKilled is a last-termination observation,
not a complete event history. Readiness is separate from collection health.

## Alerts and links

List/detail include stable rule identity, owned scope/workload, severity,
pending/firing, active time, evaluated value and safe annotations. Authorize and
filter every returned rule/alert by server-owned application/environment identity.
Rule status, Alertmanager state and receiver delivery evidence are separate.
No active alert or absent series does not establish resolved/recovered state.
Drill-down retains Environment/workload/time; native log links identify their
source and retention. Grafana links contain only scope/time/dashboard parameters;
Grafana authorization is independent. Deployment proximity is evidence, not a
causal claim.

## HTTP route inventory

PE/Admin routes: GET/POST `/api/v1/monitoring/connections`,
PUT/DELETE `/api/v1/monitoring/connections/{connection}`,
POST `/api/v1/monitoring/connections/{connection}/test`.
Environment selection: GET/PUT
`/api/v1/applications/{id}/environments/{env}/monitoring/binding`.
Application-scoped observation: GET
`/api/v1/applications/{id}/environments/{env}/monitoring` with from/to/workload.
Mutation JSON carries expected revision; public Connection projects only nonsecret
configuration, credentialConfigured boolean and verification state. Credentials
are write-only token input; an opaque reference is never accepted from the client.
