# Slice 1 development observability stack

For portable operator installation and partial existing stacks, use the
[Bash/Helm installer](../../../scripts/observability/README.md) and
[runbook](../../../orchestrator_docs/operations/observability-installation.md).
This folder retains the historical dev acceptance fixture/receiver; do not copy
its kind-specific values/identity generator as a general cluster installer.

Operator setup for [ADR-040](../../../orchestrator_docs/architecture/decisions/ADR-040-external-observability.md)
and [UC-09 OBS](../../../orchestrator_docs/usecase/UC-09/specification.md#external-kubernetes-monitoring-and-alerts--obs).
Installation is not product acceptance. Requires explicit cluster mutation permission,
Helm 3, kubectl, kind `k8s-4f`, working standard local-path storage and capacity.
The 2026-11-05 user authorization covers this dev setup and isolated test receiver.

Pinned kube-prometheus-stack 92.18.0 (operator v0.94.2) comes from
https://prometheus-community.github.io/helm-charts. `values.yaml` pins dev retention,
requests/limits and 10Gi total PVC requests (Prometheus 8Gi, Grafana/Alertmanager 1Gi
apiece). This is development configuration, not company production policy.
Component images inherit the immutable chart version defaults. Record downloaded
chart checksum and rendered image inventory outside Git before installation.

Preflight: explicitly select context, inspect namespace/release/CRD absence,
node capacity/allocations and disk space. Never adopt a preexisting release or
namespace without proving ownership. The shared CRDs are retained on cleanup.
Namespace `orch-observability` must carry `orchestrator.dev/setup=observability-s1`.

From repository root, after preflight and recording ownership:

```sh
helm install orch-observability kube-prometheus-stack \
  --repo https://prometheus-community.github.io/helm-charts --version 92.18.0 \
  --kube-context kind-k8s-4f --namespace orch-observability \
  --values e2e/setup/observability-stack/values.yaml --timeout 10m
```

Use explicit context for every read/mutation. Poll readiness separately; if install
fails, inspect resources before retry. Never blindly reinstall or reset PVCs.
By default cluster notifications go to discard; only alerts explicitly marked
`orchestrator_test=true` route to the in-namespace receiver Service. No production
channel is configured. The receiver is a separate acceptance prerequisite.
Prometheus/Alertmanager stay ClusterIP; backend connectivity uses authorized scoped
networking/forwards. Grafana browser access requires authentication and a checked
Tailscale route. Never print generated Grafana credentials or embed them in links.

Keep this shared stack after successful acceptance. Cleanup requires explicit
operator intent: prove original namespace UID and release ownership, check no
other app depends on it, uninstall only this release, then remove only owned PVCs
and namespace with UID preconditions. Keep cluster-scoped CRDs/RBAC that may have
other consumers; do not delete all monitoring resources by API group. Local-path
PVC data is deleted if its PVC is removed. Record native absence checks and retained
objects. Test-app cleanup never removes this shared stack.

Read-back checks: StatefulSets/Deployments/DaemonSet ready, PVCs Bound, healthy
scrape targets, real kube-state-metrics/cAdvisor/node-exporter data, loaded app
rules, Alertmanager receiver firing/resolved evidence. No mock proves these.
Private manifests, generated secrets, logs, charts, screenshots and receipts remain
outside Git under the run-specific state directory.

`identity.sh` is a read-only generator: it reads the actual kube-system namespace
UID and emits a setup-owned identity recording rule. Save/review its output outside
Git, verify setup namespace UID/label, then create it once. On rerun, compare the
existing owner and exact spec before reuse. This trusted operator assertion links
the configured Prometheus to its physical cluster; product binding independently
reads the same UID using the owned deployment target credential. Endpoints and
namespace names alone never establish cluster identity.

`receiver.go` is a Go standard-library dev acceptance receiver. Build it outside
Git with `CGO_ENABLED=0 go build -o /private/receiver receiver.go`, put it in a
scratch image and load that exact image into the authorized kind cluster. Run
only a receipt-owned Deployment/ClusterIP Service named `observability-test-receiver`
in the setup namespace, with 16Mi/10m requests and 64Mi/100m limits. It accepts
only alerts with `orchestrator_test=true`, keeps at most 1000 timestamped receipts
in process memory, and exposes GET `/notifications` for private evidence reads.
A restart loses receipts; preserve them outside Git before cleanup. It does not
simulate notifications. Only actual Alertmanager requests create receipts.
Record Pod/Deployment/Service UIDs and use UID checks before deletion.
