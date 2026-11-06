---
id: ADR-040
artifact: architecture-decision
status: current
last_reviewed: 2026-11-05
related: [ADR-023, ADR-036, UC-04, UC-09]
---

# ADR-040 — External Kubernetes monitoring and alerts

Accepted by the user on 2026-11-05 for Slice 1 of issue #35. Implement only this
slice; later application, database and dependency instrumentation remain deferred.
The user authorized the shared dev stack on kind `k8s-4f`, an isolated test
receiver, and controlled failure/recovery on owned test workloads. Shared apps,
production receivers and retained platform installations are outside that mutation scope.

## Sources and ownership

Prometheus collects and evaluates rules; Alertmanager routes notifications;
Grafana supplies deep analysis. Stack installation is operator setup, never a
side effect of registering a product connection. Platform PostgreSQL stores
connection/binding metadata, not replicated external metric history.

An Environment with a verified binding uses external metrics explicitly. Errors
never silently fall back to native metrics. Unbound Environments retain ADR-023.
Native logs and retained native metric history remain readable with their source
identified; binding changes never delete historical rows. Native collection may
continue independently while the selected current metric source is external.

PE/Admin manages organization-scoped connections using private credential
references. Endpoint TLS verification is mandatory; optional private CA is explicit.
Private network endpoints are necessary; redirects, URL credentials, query/fragment,
link-local/metadata destinations and arbitrary client-supplied PromQL are rejected.
Grafana browser URL is distinct from backend endpoints and never contains secrets.

## Query identity

Authorization precedes network reads. Backend restores owned current-generation
workload targets and immutable controller identities, verifies their physical
cluster against the binding, and builds namespace/workload/pod selectors itself.
Namespace/name equality alone is insufficient proof across clusters or generations.
Pod UID and timestamp distinguish replacements. Unknown/missing/partial/stale data
remain explicit; missing values are never zero or evidence of recovery.

## Configuration lifecycle

Git/Fleet delivers app-owned PrometheusRule alongside the app's existing delivery
units, using stable ownership and names. Prometheus query API is read-only for this
purpose. Rule selectors and namespace selectors must be verified against the
actual stack. Existing cluster rules remain intact. Redeploy is idempotent and
withdrawal removes only app-owned configuration, preserving shared stack/history.
Configuration applied and metrics received are independent statuses.

Representative development rules cover unavailable desired replicas with rollout
allowance and scale-to-zero, restart increase over a configurable window, and
OOMKilled observations with documented source limits. Thresholds/durations/severity
are configuration with dev provenance, not company production standards. Prometheus
is the only rule evaluator; Alertmanager is the only notification sender.

## Acceptance

Console connection/test/binding, overview/workloads/charts, scoped alerts and
metrics/logs/deployment drill-down must work with real data. Test failure -> firing
-> actual receiver notification -> recovery -> resolved notification. Notification
delivery is unknown unless sender/receiver evidence proves it. Verify missing and
stale signals, cross-app/environment isolation, redeploy idempotency, and owned
cleanup. Browser handoff uses a checked Tailscale URL. Installation alone does not
complete Slice 1.

[External contract](../contracts/external-observability.md) owns bounds and API.
[UC-09](../../usecase/UC-09/specification.md) owns required product behavior.
