---
id: VER-OBSERVABILITY-CAPABILITIES-20261107
artifact: verification
status: current
last_reviewed: 2026-11-07
related: [ADR-040, UC-09]
---

# Issue #35 capability execution checkpoints

User authorized continuing through stage 5, stopping before stage 6, and creating
a dedicated kind cluster. No merge into main is authorized. This record separates
implemented mechanisms from completed live gates.

## Installation core checkpoint

A fresh owned two-node kind cluster runs the pinned metrics and logging charts.
Prometheus Operator, Prometheus, Alertmanager, Grafana, kube-state-metrics,
node-exporter, Loki and Alloy reached controller readiness. Metrics were installed
first; the logging installation explicitly reused the six metrics components and
existing monitoring CRDs. Reused controller UIDs and pod templates were unchanged.
Both unchanged repeated applies reported NOOP for every owned release.

Real queries returned cAdvisor CPU/RAM, kube-state-metrics ownership/node data,
and Loki streams carrying the physical cluster UID. These prove source collection,
not complete application acceptance or production compliance. PostgreSQL exporter,
blackbox/ingress coverage, datasource/dashboard checks, notifications and restart
persistence remain separate gates.

The first live run found a Helm defaults merge defect: overriding the Alertmanager
receiver left default child routes pointing at an absent receiver. The values now
set an explicit empty child-route list. Rendered configuration passed the real
Alertmanager `amtool check-config`; the owned release update reconciled both
Prometheus and Alertmanager readiness. No shared cluster was changed or reset.

Local-query isolation needs `cluster_uid` metric relabeling on kube-state-metrics
and kubelet/cAdvisor; the example values require the operator to replace that UID.
External labels alone do not scope local Prometheus queries.

## Connection and binding checkpoint

The isolated feature Docker stack initialized a fresh schema-11 database with
separate owner/runtime identities. Console was opened through the actual Tailscale
feature URL. Its existing shell/platform forms were the visual reference before UI
changes. Browser creation and testing of a real connection returned successful
Prometheus/Alertmanager/Loki connectivity, cluster identity and Kubernetes/log
coverage, with config sync explicitly UNVERIFIED.

Focused Go tests cover source-specific credentials even with identical endpoint
URLs, revision-stable retesting, pre-deployment binding, foreign target rejection,
reserved labels and organization boundaries. Real PostgreSQL focused tests cover
verification CAS, referenced deletion, binding deletion fences and stale writes.
Upgrade/rollback tests preserve populated schema-8/schema-10 identities through
schema 11 and keep State runtime privileges separate. No released migration or
schema reference was changed. The inaccessible historical monitoring-only schema-9
installation has not been opened, reset or declared upgrade-compatible.

Focused existing frontend observability tests passed (8 assertions across two
files). TypeScript checks passed; lint has one pre-existing warning in
PlatformSettingsPage. Documentation checker passed; PlantUML was unavailable.

## Work in progress

Typed profiles/PodMonitor generation, retained Loki queries, live artifact
comparison and native-source withdrawal are being implemented and validated.
These are not yet completed stages 2–5. Actual MCP Gateway/Admin sources are still
missing; generic fixtures must not be counted as MCP-specific acceptance.

Private runtime state, credentials, kubeconfig and screenshots are outside Git.
The isolated kind and feature stack are retained for the remaining checks. Private
network addresses/feature links are deliberately omitted from this public record.
