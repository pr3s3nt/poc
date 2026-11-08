---
id: ADR-041
artifact: architecture-decision
status: current
last_reviewed: 2026-11-06
related: [ADR-023, ADR-036, UC-04, UC-09]
---

# ADR-041 — External Kubernetes monitoring and alerts

Initially accepted by the user on 2026-11-05 for Slice 1 of issue #35. Product
implementation remains WIP; capability-based installation tooling was subsequently
approved on 2026-11-06 as recorded below.
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


## Capability stages and portable installation — decision 2026-11-06

Issue #35 now proceeds by capability: installation, connections/bindings,
collection/configuration, investigation, alerts, lifecycle and overall acceptance.
The user requested a portable, idempotent operator installer for another cluster
including partial existing stacks. This extends operator tooling, not Console
provisioning or permission to mutate an unspecified cluster.

Use a standalone Bash + Helm installer with jq configuration (no custom Go/Node
runtime). The installer must require an explicit context and physical cluster UID on apply.
It plans all releases before mutation; archives are version/checksum pinned.
Operators explicitly choose install, reuse or skip for each signal component.
Reuse is read-only and identity-pinned; existing Helm releases/resources are never
adopted. Only installer-owned releases may be updated. Existing shared CRDs are
reused explicitly, never force-replaced. Unchanged deployed releases are no-ops;
failed/pending or ambiguous ownership requires reconciliation rather than reset.
Runtime receipts live outside Git and survive partial failure. No automatic
uninstall, namespace/PVC deletion or data reset is included. The
[installation runbook](../../operations/observability-installation.md) defines
configuration, validation and recovery. Installation readiness does not establish
telemetry coverage, notification delivery or application acceptance.

## Capability collection and profiles — execution 2026-11-07

The approved capability stages use typed, versioned per-workload collection
profiles, PodMonitor delivery through the existing Git/Fleet units, and trusted
collector identity labels. PromQL/LogQL remain server-built. Local Prometheus
samples must carry a cluster label; external labels alone are insufficient.
Connection defaults and whole-workload binding overrides support heterogeneous
applications and exporters without a generic policy language. See the contract
for bounds and precedence. A dedicated owned kind cluster is authorized for live
checks; actual MCP instrumentation remains dependent on Gateway/Admin source.
