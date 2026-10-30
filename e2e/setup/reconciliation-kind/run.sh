#!/usr/bin/env bash
set -euo pipefail
umask 077
# Creates only a fresh task-owned cluster; never installs Fleet into an existing one.
run_dir=${ORCH_RECON_RUN_DIR:?Set ORCH_RECON_RUN_DIR to a new absolute directory outside Git}
cluster=${ORCH_RECON_CLUSTER:-orch-reconciliation-20261028}
[[ "$run_dir" = /* && ! -e "$run_dir" && "$cluster" =~ ^orch-reconciliation-[a-z0-9-]+$ ]] || { echo 'Invalid or existing run directory/cluster name' >&2; exit 1; }
for cmd in kind kubectl helm curl sha256sum docker; do command -v "$cmd" >/dev/null; done
parent=$(dirname "$run_dir")
[[ -d "$parent" ]] || exit 1
if git -C "$parent" rev-parse --show-toplevel >/dev/null 2>&1; then echo 'Run directory must be outside Git' >&2; exit 1; fi
if kind get clusters | rg -x "$cluster" >/dev/null; then echo 'Refusing existing cluster' >&2; exit 1; fi
mkdir -m 700 "$run_dir"
owned=0
on_failure() {
 status=$?
 if [[ "$status" != 0 && "$owned" = 1 ]]; then kind delete cluster --name "$cluster"; fi
 exit "$status"
}
trap on_failure EXIT
printf '%s\n' "$cluster" > "$run_dir/cluster-name"
owned=1
kind create cluster --name "$cluster" --image kindest/node:v1.35.0@sha256:4613778f3cfcd10e615029370f5786704559103cf27bef934597ba562b269661 --kubeconfig "$run_dir/kubeconfig" --wait 180s
# Bound Go runtime parallelism for kindnet's 100m default budget on large hosts.
# Keep policy enforcement active; raise its budget rather than disabling CNI guards.
kubectl --kubeconfig "$run_dir/kubeconfig" -n kube-system set resources daemonset/kindnet --limits=cpu=500m,memory=128Mi --requests=cpu=100m,memory=50Mi
kubectl --kubeconfig "$run_dir/kubeconfig" -n kube-system set env daemonset/kindnet GOMAXPROCS=1
kubectl --kubeconfig "$run_dir/kubeconfig" -n kube-system rollout status daemonset/kindnet --timeout=60s
# Record exact incarnation before installing anything.
docker inspect --format '{{.Id}}' "$cluster-control-plane" > "$run_dir/container-id"
kubectl --kubeconfig "$run_dir/kubeconfig" get namespace kube-system -o jsonpath='{.metadata.uid}' > "$run_dir/kube-system-uid"
for chart in fleet-crd fleet; do
 curl --fail --silent --show-error --location --max-time 120 "https://github.com/rancher/fleet/releases/download/v0.15.2/$chart-0.15.2.tgz" -o "$run_dir/$chart-0.15.2.tgz"
done
(cd "$run_dir" && sha256sum --check <<'SUMS'
d41960617204d0172a2ae2f336bf6c6c6611c364ef6f463fb4f996b681ed30a2  fleet-crd-0.15.2.tgz
602f7da0519e434ad1e0ee1b906143132ff21f8475a4ceb77ad7d490dff6954b  fleet-0.15.2.tgz
SUMS
)
helm --kubeconfig "$run_dir/kubeconfig" upgrade --install fleet-crd "$run_dir/fleet-crd-0.15.2.tgz" --namespace cattle-fleet-system --create-namespace --wait --timeout 300s
helm --kubeconfig "$run_dir/kubeconfig" upgrade --install fleet "$run_dir/fleet-0.15.2.tgz" --namespace cattle-fleet-system --set agentImage.repository=rancher/fleet-agent --set agentImage.tag=v0.15.2 --wait --timeout 300s
kubectl --kubeconfig "$run_dir/kubeconfig" -n cattle-fleet-system get deployments -o json > "$run_dir/controller-deployments.json"
printf 'Fleet v0.15.2 setup ready in %s. This is setup only, not application acceptance.\n' "$cluster"
