---
id: VER-OBSERVABILITY-BOUTIQUE-20261108
artifact: verification
status: current
last_reviewed: 2026-11-08
related: [ADR-041, UC-09]
---

# Issue #35 stage 6 — Online Boutique observability through Console UI

User decision 2026-11-08: stage 6 is accepted through the Console UI (no Score
file import, no API mutation, no fake adapter) using the Online Boutique example
(<https://docs.score.dev/docs/examples/microservices/>), on the shared
`kind-k8s-4f` cluster. The task-owned stage-0–5 cluster had been deleted; its
evidence stays in [capability checkpoints](2026-11-07-observability-capabilities.md).
Scenario: [observability-boutique](../../e2e/scenarios/observability-boutique/README.md)
together with [online-boutique](../../e2e/scenarios/online-boutique/README.md).
Private URLs, credentials, screenshots and receipts stay outside Git.

## Environment changes on kind-k8s-4f

- The issue-35 Slice-1 `orch-observability` kube-prometheus-stack release was
  upgraded with `cluster_uid` relabeling for kubelet/cAdvisor and
  kube-state-metrics only. Rendered diff contained only those additions; PVC,
  controller UIDs and the Grafana secret were unchanged. Real samples carry the
  label afterwards.
- Loki and Alloy were installed with the portable installer, reusing all six
  metrics components (receipts private). Re-verify returned NOOP. Alloy collects
  only `orch-observability` and the Boutique namespace; adding that namespace was
  an owned Alloy upgrade with Loki NOOP. Real streams carry the cluster UID.
- Two owned Grafana sidecar ConfigMaps add the Loki datasource and the workload
  dashboard. Other namespaces/releases on the cluster were not modified.
- The feature Console stack was rebuilt empty (`down -v`) from the merged branch;
  its Vault gained Kubernetes auth for the cluster and a scoped secret-store token.

## UI flow and results

| Step (all writes by browser controls) | Result |
|---|---|
| Git, cluster account, secret store, Fleet delivery, Definitions, Application, 11 workloads + Redis | PASS (online-boutique runners) |
| Monitoring connection created and tested | connectivity and cluster identity verified; rules selector `release=orch-observability` |
| Binding staging to Prometheus/Loki before first Deploy | PASS |
| One batch Preview/Deploy of 11 workloads | SUCCEEDED; 11 app-owned PrometheusRules |
| Console monitoring/logs | 11 workloads `APPLIED`; real Loki logs of `cart`; state PARTIAL (no OOM observation yet) |
| Renderer repin then redeploy of all workloads | SUCCEEDED; still 11 rules, all `APPLIED` (no duplicates) |
| Controlled failure: `cart` memory limit 16Mi via workload form, Preview/Deploy | real `OOMKilled` restarts; OOM and Restarts firing, Unavailable firing after 5 minutes |
| Isolated receiver | actual firing notifications for OOM, Restarts, Unavailable of `cart` |
| Investigation | Console showed 0/1 ready, restarts, OOM=1 and the three alerts; logs and the contextual Grafana dashboard showed the incident window |
| Recovery: original 90Mi limit via form, Preview/Deploy | SUCCEEDED; pod Ready; actual resolved notifications for all three alerts |

The controlled-OOM Deploy reported SUCCEEDED although the new `cart` Pod then
crash-looped: deployment success does not prove runtime health, which is the gap
these signals cover. The development restart rule (increase ≥ 3 in 5 minutes)
resolved while CrashLoopBackOff lengthened intervals; Unavailable covered the
remaining outage. These are development rule values, not company thresholds.

## Defects found and fixed during acceptance

- Monitoring read returned 503 for every request with 11 workloads: live target
  verification and per-workload observation ran sequentially inside one
  10-second budget (measured 10.0 s → 503). They now run with bounded
  concurrency (6), every target must still pass and order is preserved; the same
  read takes about 3 s. Tests fail when forced sequential.
- The installer metrics values replaced the chart's kubelet bucket drop rule when
  adding `cluster_uid`; the default rule is now kept.
- The workload dashboard variable used `kube_deployment_labels`, which the pinned
  kube-state-metrics does not expose, so every panel was empty; it now uses
  `kube_deployment_status_replicas_available` with an explicit All option, and the
  environment-wide Console link selects `$__all`.

## Not covered

MCP Gateway/Admin sources (not supplied), Boutique application metrics (no
`/metrics` endpoint; profile left empty), Redis exporter/dependency observations,
production receivers and thresholds. Native withdrawal/unbind and application
cleanup are kept for after user acceptance so the environment stays inspectable;
they were verified on the stage-5 cluster.
