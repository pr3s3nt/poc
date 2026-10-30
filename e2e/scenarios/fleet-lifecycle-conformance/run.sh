#!/usr/bin/env bash
set -euo pipefail
umask 077
run_dir=${ORCH_RECON_RUN_DIR:?Original owned-kind run directory required}
cluster=$(cat "$run_dir/cluster-name")
[[ "$cluster" =~ ^orch-reconciliation-[a-z0-9-]+$ ]] || exit 1
[[ "$(docker inspect --format '{{.Id}}' "$cluster-control-plane")" = "$(cat "$run_dir/container-id")" ]] || exit 1
kubeconfig="$run_dir/kubeconfig"
uid=$(kubectl --kubeconfig "$kubeconfig" get namespace kube-system -o jsonpath='{.metadata.uid}')
[[ "$uid" = "$(cat "$run_dir/kube-system-uid")" ]] || exit 1
fixture="$run_dir/lifecycle"
[[ ! -e "$fixture" ]] || { echo 'Use a fresh conformance run; existing fixture kept for investigation' >&2; exit 1; }
mkdir -m 700 "$fixture"
git init --bare --initial-branch=main "$fixture/repo.git" >/dev/null
git clone "$fixture/repo.git" "$fixture/writer" >/dev/null 2>&1
mkdir "$fixture/writer/unit"
cat > "$fixture/writer/unit/fleet.yaml" <<'YAML'
namespace: orch-fleet-conformance
correctDrift:
  enabled: false
  force: false
deleteNamespace: false
helm:
  releaseName: orch-lifecycle-conformance
  force: false
  takeOwnership: false
YAML
cat > "$fixture/writer/unit/configmap.yaml" <<'YAML'
apiVersion: v1
kind: ConfigMap
metadata:
  name: retained-fixture
  namespace: orch-fleet-conformance
  annotations:
    helm.sh/resource-policy: keep
  labels:
    orchestrator.io/conformance: owned
    orchestrator.io/managed-by: orchestrator
data:
  value: original
YAML
git -C "$fixture/writer" add unit
git -C "$fixture/writer" -c user.name=Orchestrator -c user.email=orchestrator@localhost commit -m 'Fleet lifecycle conformance' >/dev/null
git -C "$fixture/writer" push origin main >/dev/null 2>&1
commit=$(git -C "$fixture/writer" rev-parse HEAD)
# Read-only task-owned Git fixture. This transport tests controller semantics;
# production adapter HTTPS/authentication is a separate acceptance obligation.
git daemon --reuseaddr --export-all --base-path="$fixture" --listen=172.18.0.1 --port=19418 "$fixture/repo.git" > "$fixture/git-daemon.log" 2>&1 &
git_pid=$!
trap 'kill "$git_pid" 2>/dev/null || true' EXIT
kubectl --kubeconfig "$kubeconfig" create namespace orch-fleet-conformance >/dev/null
kubectl --kubeconfig "$kubeconfig" create -f - <<YAML
apiVersion: fleet.cattle.io/v1alpha1
kind: GitRepo
metadata:
  name: orch-lifecycle-conformance
  namespace: fleet-local
  labels:
    orchestrator.io/conformance: owned
spec:
  repo: git://172.18.0.1:19418/repo.git
  revision: $commit
  paths: [unit]
  targets:
    - clusterName: local
YAML
ready=0
for ((attempt=0;attempt<90;attempt++)); do
 if kubectl --kubeconfig "$kubeconfig" -n orch-fleet-conformance get configmap retained-fixture -o json > "$fixture/configmap-applied.json" 2>/dev/null; then ready=1; break; fi
 sleep 2
done
[[ "$ready" = 1 ]] || { echo 'Fleet did not apply fixture within deadline' >&2; exit 1; }
kubectl --kubeconfig "$kubeconfig" -n fleet-local get gitrepo orch-lifecycle-conformance -o json > "$fixture/gitrepo-applied.json"
kubectl --kubeconfig "$kubeconfig" -n orch-fleet-conformance get configmap retained-fixture -o jsonpath='{.metadata.uid}' > "$fixture/object-uid"
kubectl --kubeconfig "$kubeconfig" -n fleet-local get gitrepo orch-lifecycle-conformance -o jsonpath='{.metadata.uid}' > "$fixture/gitrepo-uid"
# Everything in this fixture was created in the verified disposable cluster.
# Observe native retained uninstall before the product implements guarded deletion.
kubectl --kubeconfig "$kubeconfig" -n fleet-local delete gitrepo orch-lifecycle-conformance --wait=true --timeout=120s
kubectl --kubeconfig "$kubeconfig" -n orch-fleet-conformance get configmap retained-fixture -o json > "$fixture/configmap-retained.json"
[[ "$(kubectl --kubeconfig "$kubeconfig" -n orch-fleet-conformance get configmap retained-fixture -o jsonpath='{.metadata.uid}')" = "$(cat "$fixture/object-uid")" ]]
helm --kubeconfig "$kubeconfig" -n orch-fleet-conformance list --all --output json > "$fixture/releases-after-withdrawal.json"
[[ "$(cat "$fixture/releases-after-withdrawal.json")" = '[]' ]] || { echo 'Owned Helm release remains after withdrawal' >&2; exit 1; }
printf 'Fleet retained-withdrawal fixture passed; product E2E remains separate.\n'
