# Online Boutique observability acceptance through Console UI — issue #35 stage 6

Goal: prove [UC-09 external observability](../../../orchestrator_docs/usecase/UC-09/specification.md)
end to end on a real application: connect the shared stack, bind the Environment
before deployment, deploy the eleven Online Boutique workloads, observe applied
configuration, metrics and Loki logs in Console, trigger a controlled failure
through the workload form, receive the real Alertmanager notification, recover,
then withdraw to native sources. Contract:
[external observability](../../../orchestrator_docs/architecture/contracts/external-observability.md),
architecture [ADR-041](../../../orchestrator_docs/architecture/decisions/ADR-041-external-observability.md).

Business writes use browser controls only (connection, binding, workload edits,
Preview, Deploy, unbind). API GETs and response observation are technical
assertions. No Score import, API mutation or fake adapter is used. Boutique has
no Prometheus metrics endpoint, so the collection profile is left empty: the
product delivers Kubernetes availability/restart/OOM rules only. MCP Gateway/Admin
coverage is not claimed.

## Prerequisites

- The Boutique Console run from [online-boutique](../online-boutique/README.md):
  `platform.mjs`, `definitions.mjs` and `workloads.mjs` completed, same
  `ORCH_TEST_RUN_DIR`, shared Console variables and `ORCH_CLUSTER_KUBECONFIG`
  (host-side kubeconfig used for read-only checks and the receiver read).
- An installed, verified observability stack in the target cluster (see the
  [installation runbook](../../../orchestrator_docs/operations/observability-installation.md)):
  Prometheus with `cluster_uid` relabeling, Alertmanager routing
  `orchestrator_test=true` to `observability-test-receiver`, Loki/Alloy collecting
  the application namespace, Grafana with the Loki datasource and dashboard.
- Backend-reachable query endpoints and a Tailscale-reachable Grafana origin.
- `observability-input.json` (0600) in the run directory with `name`,
  `prometheusUrl`, `lokiUrl`, `alertmanagerUrl`, `grafanaUrl`, `clusterUid`,
  `dashboardUid`, `expectedWorkloads`, `cartMemoryLimit`, `cartMemoryRequest`
  and `cartOomLimit`. It contains no credential.
- The Fleet namespace RBAC rules must allow `monitoring.coreos.com`
  `prometheusrules`/`podmonitors`; pass them with `ORCH_NAMESPACE_ACCESS_RULES_FILE`
  to the online-boutique `deploy.mjs`.

## Run from repository root

```sh
node e2e/scenarios/observability-boutique/run.mjs connect   # create + test connection
node e2e/scenarios/observability-boutique/run.mjs bind      # bind staging before deploy
ORCH_DEPLOY=1 node e2e/scenarios/online-boutique/deploy.mjs # one batch Deploy
# Operator step: add the product namespace to the Alloy allowlist and re-apply.
node e2e/scenarios/observability-boutique/run.mjs observe
node e2e/scenarios/observability-boutique/run.mjs oom       # cart memory limit too low
node e2e/scenarios/observability-boutique/run.mjs receipts  # firing notifications
node e2e/scenarios/observability-boutique/run.mjs restore   # original limit, Deploy
node e2e/scenarios/observability-boutique/run.mjs receipts  # resolved notifications
node e2e/scenarios/observability-boutique/run.mjs native    # withdraw app-owned config
node e2e/scenarios/observability-boutique/run.mjs unbind
```

## Assertions

- Connection test reports connectivity and cluster identity; rule selector labels
  are the shared stack's `release=orch-observability`.
- Binding exists before the first Deploy; every Boutique workload reports
  configuration `APPLIED` and the Console monitoring and log views open.
- The controlled OOM produces real `OOMKilled` restarts; the configured rules fire
  and the isolated receiver records actual firing and later resolved
  notifications. Missing data is never treated as recovery.
- Native withdrawal leaves no app-owned PrometheusRule/PodMonitor; unbind succeeds.

## Side effects and cleanup

Creates a monitoring connection/binding in the feature Console and app-owned
PrometheusRule objects in the Boutique namespace through Git/Fleet. The OOM step
deliberately makes `cart` unavailable until `restore`. Screenshots and
`observability.json` stay in the private run directory. Cleanup uses `native`
then `unbind`; shared stack releases, receiver, history and Alloy configuration
remain and are removed only by explicit operator work.
