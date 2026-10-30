#!/usr/bin/env bash
set -euo pipefail
run_dir=${ORCH_RECON_RUN_DIR:?Set ORCH_RECON_RUN_DIR to the original run directory}
cluster=$(cat "$run_dir/cluster-name")
[[ "$cluster" =~ ^orch-reconciliation-[a-z0-9-]+$ ]] || exit 1
expected=$(cat "$run_dir/container-id")
actual=$(docker inspect --format '{{.Id}}' "$cluster-control-plane" 2>/dev/null || true)
if [[ -n "$actual" ]]; then
 [[ "$actual" = "$expected" ]] || { echo 'Cluster incarnation differs; refusing cleanup' >&2; exit 1; }
 uid=$(kubectl --kubeconfig "$run_dir/kubeconfig" get namespace kube-system -o jsonpath='{.metadata.uid}')
 [[ "$uid" = "$(cat "$run_dir/kube-system-uid")" ]] || { echo 'Cluster identity differs; refusing cleanup' >&2; exit 1; }
 kind delete cluster --name "$cluster"
fi
if docker inspect "$cluster-control-plane" >/dev/null 2>&1; then echo 'Cluster container still exists' >&2; exit 1; fi
printf 'Verified task cluster %s absent; private evidence remains in run directory.\n' "$cluster"
