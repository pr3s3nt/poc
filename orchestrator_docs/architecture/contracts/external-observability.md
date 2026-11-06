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


## Capability stages 1–5 — approved execution 2026-11-07

The user authorized continuing through lifecycle stage 5 and creating a dedicated
kind test cluster. Stop before stage 6 and do not merge main. MCP-specific live
acceptance still requires the actual Gateway/Admin sources; a generic workload
proves reusable mechanisms only.

Connections may also configure a Loki URL and separate opaque credential references
for prometheus, alertmanager and loki. Public projections expose configured flags,
never references/tokens. Existing shared-token connections remain readable until
explicitly updated. Verification facts are conditional on the configuration
revision and do not increment that revision, invalidate bound scopes, or claim
Grafana browser authorization/config application from connectivity alone.

Binding is valid before deployment. It pins verified source cluster identity and
Environment generation; absent targets mean pending target verification. Every
subsequent deployment independently verifies its credential-backed physical target
before publishing app-owned rules/scrapes. A desired manifest does not need an
existing controller UID to generate configuration. Reads still require actual
owned current-generation targets and UID checks. A configuration revision change
requires explicit binding refresh; retesting unchanged config does not.

PE/Admin owns connection, binding/profile updates and unbinding. Developers read
only authorized app signals. Unbind is rejected while a live workload still has app-owned configuration.
Select native sources and redeploy to withdraw monitoring artifacts through normal
Git/Fleet pruning, then unbind after absence is verified; workload removal through
existing delivery cleanup is another supported path. Connection deletion rejects any referencing binding. This preserves
Git/Fleet ownership and avoids leaving live unowned rules after unbinding.

Logs source is explicit per binding: native by default, Loki when selected and
configured. Loki errors do not fall back. Bound queries use backend-owned namespace
and controller/pod inventory, bounded time/search/paging, and identity checks.
Keep native retained history accessible as an explicit source. Parsing detects
levels only from source facts; unknown remains unknown. Source/config/data/rule
and notification states remain independent.

Profiles use a bounded typed versioned configuration rather than a generic policy
engine. Connection defaults are inherited by the binding; explicit PE/Admin
binding overrides win. Bindings pin connection/profile versions and generation.
Changes must be reviewed/rebound and applied through ordinary delivery; a DB save
is not evidence of Fleet/Prometheus/Loki synchronization. No second rule evaluator
or product-owned notification sender is introduced.

### Typed collection/profile contract

A connection may publish `profile` with `schemaVersion: 1`, a nonempty immutable
version string, and optional workload collection defaults. Binding
`workloadProfiles` supplies PE/Admin overrides keyed by workload ID; an explicit
entry replaces the complete defaults for that workload, rather than deep-merging
ambiguous fields. Binding pins the profile version from its connection revision.
Rules retain their explicitly configured development provenance.

A workload profile contains a named Pod port, `/metrics` path (no query/fragment),
30–300 second scrape interval, sample limit, and at most eight typed metric
observations. Each metric supplies an identifier, source metric name, unit,
aggregation (`gauge`, `rate`, or histogram `p95`), optional bounded selector
labels, and optional threshold/duration/severity. Metric names/labels are validated
identifiers, never PromQL. Rates have a fixed five minute window. Rates and
histograms need actual instrumentation; a missing family remains missing. Database,
ingress and dependency exporters use the same mechanism and their actual metric
names; there are no assumed MCP-specific names or business semantics.

Git/Fleet delivers the generated PodMonitor and rules beside each owned controller.
The monitor selects that workload's managed Pods and attaches trusted cluster,
organization, application, environment, generation and workload target labels.
`honorLabels: false` prevents an application from overriding those labels.
Pod template labels supplied by the platform carry the same identities. API log
collection relabels these Kubernetes identities; log JSON cannot override them.
Collector namespace RBAC remains an explicit operator allowlist. High cardinality
pod UID stays in structured metadata, not an indexed Loki label. Request/trace IDs
remain log fields. Plain text is retained; unrecognized level stays UNKNOWN.

Kubernetes metrics require an operator-configured `cluster_uid` metric label on
kube-state-metrics and kubelet/cAdvisor samples, including local Prometheus reads.
External labels alone do not provide local-query isolation. Every generated query
filters this cluster label and namespace, and read verification still checks live
owned controller identity. Operators reusing a stack must configure the equivalent
relabel rules before the source is marked ready.

Loki selectors include the trusted cluster/org/app/environment/generation labels
and optional workload/container. Backend validates the owned environment before
network access. This retains logs for replaced Pods within the bound generation;
a current Pod UID is not used to erase retained history. A generation change does
not silently expose an earlier generation's logs. Native history remains available
through explicit `source=native`; source changes never delete it. Loki paging is
bounded and reports truncation when timestamp ties exceed the provider page bound.
