#!/usr/bin/env bash
# Command-contract tests with a simulated cluster. Not live Kubernetes acceptance.
set -Eeuo pipefail
root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
mkdir -p "$tmp/bin"
cat > "$tmp/bin/kubectl" <<'KUBE'
#!/usr/bin/env bash
set -euo pipefail
[[ $1 == --context && $2 == test-cluster && $3 == --request-timeout=30s ]] || exit 91
shift 3
if [[ -f $MOCK_ROOT/rbac-failure ]]; then echo 'PRIVATE-ERROR' >&2; exit 1; fi
case "$1:$2" in
 version:-o) echo '{"serverVersion":{"gitVersion":"v1.36.1"}}' ;;
 get:namespace)
   if [[ $3 == kube-system ]]; then jq -n --arg uid "$(cat "$MOCK_ROOT/cluster")" '{metadata:{uid:$uid}}'
   elif [[ -f $MOCK_ROOT/namespace.json ]]; then cat "$MOCK_ROOT/namespace.json"; fi ;;
 get:storageclass) echo storageclass/standard ;;
 get:deployments.apps,statefulsets.apps,daemonsets.apps) cat "$MOCK_ROOT/workloads.json" ;;
 get:crds) cat "$MOCK_ROOT/crds.json" ;;
 get:prometheuses.monitoring.coreos.com|get:alertmanagers.monitoring.coreos.com) cat "$MOCK_ROOT/controllers.json" ;;
 get:deployment|get:statefulset|get:daemonset) cat "$MOCK_ROOT/reused.json" ;;
 create:-f)
   cat >/dev/null
   echo namespace >> "$MOCK_ROOT/mutations"
   echo '{"metadata":{"uid":"namespace-1"}}' > "$MOCK_ROOT/namespace.json"
   cat "$MOCK_ROOT/namespace.json" ;;
 *) printf 'Unexpected kubectl invocation\n' >&2; exit 92 ;;
esac
KUBE
cat > "$tmp/bin/helm" <<'HELM'
#!/usr/bin/env bash
set -euo pipefail
[[ " $* " == *' --kube-context test-cluster '* || $1 == version ]] || exit 93
case "$1" in
 version) echo v3.19.4 ;;
 list)
   if [[ " $* " == *' --selector '* ]]; then jq '[.[]|select(.owned)]' "$MOCK_ROOT/releases.json"
   else cat "$MOCK_ROOT/releases.json"; fi ;;
 show) printf 'name: kube-prometheus-stack\nversion: 1.0.0\n' ;;
 template)
   if [[ $2 == values-projection ]]; then
     cat "$MOCK_ROOT/projected-values.json"
     exit 0
   fi
   printf '%s\n' "$@" > "$MOCK_ROOT/template-args"
   [[ ! -f $MOCK_ROOT/render-failure ]] || { echo 'PRIVATE-ERROR' >&2; exit 1; }
   echo 'apiVersion: v1' ;;
 get) cat "$MOCK_ROOT/live-values.json" ;;
 install|upgrade)
   verb=$1; name=$2
   if [[ $verb == install ]] && jq -e --arg name "$name" '.[]|select(.name==$name)' "$MOCK_ROOT/releases.json" >/dev/null; then exit 94; fi
   echo "$verb:$name" >> "$MOCK_ROOT/mutations"
   status=deployed; [[ ! -f $MOCK_ROOT/install-failure ]] || status=failed
   jq -n --arg name "$name" --arg status "$status" '[{name:$name,namespace:"monitoring",chart:"kube-prometheus-stack-1.0.0",status:$status,owned:true}]' > "$MOCK_ROOT/releases.json"
   [[ $status == deployed ]] || { echo 'PRIVATE-ERROR' >&2; exit 1; }
   jq -n --arg name "$name" '{items:[{kind:"Deployment",metadata:{name:"owned-operator",namespace:"monitoring",generation:1,uid:"owned-1",annotations:{"meta.helm.sh/release-name":$name}},spec:{replicas:1,template:{spec:{containers:[{image:"quay.io/prometheus-operator/prometheus-operator:v1"}]}}},status:{observedGeneration:1,readyReplicas:1,updatedReplicas:1}}]}' > "$MOCK_ROOT/workloads.json"
   jq '.items += [(.items[0] | .kind="StatefulSet" | .metadata.name="owned-prometheus" | .metadata.uid="prom-1" | .spec.template.spec.containers=[{image:"quay.io/prometheus/prometheus:v3"}])]' "$MOCK_ROOT/workloads.json" > "$MOCK_ROOT/next-workloads.json"
   mv "$MOCK_ROOT/next-workloads.json" "$MOCK_ROOT/workloads.json"
   echo 'PRIVATE-HELM-NOTES' ;;
 *) echo 'Unexpected Helm invocation' >&2; exit 95 ;;
esac
HELM
chmod +x "$tmp/bin/"*
export PATH="$tmp/bin:$PATH"
new_fixture() {
  MOCK_ROOT=$(mktemp -d "$tmp/case.XXXXXX"); export MOCK_ROOT
  mkdir "$MOCK_ROOT/state"
  touch "$MOCK_ROOT/state/.observability-installer"
  echo cluster-1 > "$MOCK_ROOT/cluster"
  echo '[]' > "$MOCK_ROOT/releases.json"
  echo '{"items":[]}' | tee "$MOCK_ROOT/workloads.json" "$MOCK_ROOT/crds.json" > "$MOCK_ROOT/controllers.json"
  echo '{"metadata":{"uid":"namespace-1"}}' > "$MOCK_ROOT/namespace.json"
  echo '{"example":"stable"}' > "$MOCK_ROOT/live-values.json"
  : > "$MOCK_ROOT/mutations"
  echo chart-fixture > "$MOCK_ROOT/chart.tgz"
  checksum=$(sha256sum "$MOCK_ROOT/chart.tgz" | cut -d' ' -f1)
  cp "$MOCK_ROOT/chart.tgz" "$MOCK_ROOT/state/metrics-$checksum.tgz"
  echo 'example: stable' > "$MOCK_ROOT/values.yaml"
  jq -n --arg checksum "$checksum" '{schemaVersion:1,installationId:"test-install",context:"test-cluster",namespace:"monitoring",createNamespace:true,storageClass:"standard",timeout:"1s",crds:"install",
    components:{operator:{mode:"install"},prometheus:{mode:"install"},alertmanager:{mode:"skip",reason:"test"},grafana:{mode:"skip",reason:"test"},kubeStateMetrics:{mode:"skip",reason:"test"},nodeExporter:{mode:"skip",reason:"test"},loki:{mode:"skip",reason:"test"},alloy:{mode:"skip",reason:"test"}},
    releases:[{id:"metrics",name:"obs-metrics",valuesFile:"values.yaml",chart:{name:"kube-prometheus-stack",version:"1.0.0",url:"https://example.invalid/chart.tgz",sha256:$checksum}}]}' > "$MOCK_ROOT/config.json"
}
invoke() { "$root/setup.sh" "$@" --config "$MOCK_ROOT/config.json" --state-dir "$MOCK_ROOT/state" > "$MOCK_ROOT/stdout" 2> "$MOCK_ROOT/stderr"; }
reject() {
  if invoke "$@"; then echo 'Expected rejection' >&2; exit 1; fi
  ! grep -q 'PRIVATE-' "$MOCK_ROOT/stdout" "$MOCK_ROOT/stderr" || { echo 'Private command output leaked' >&2; exit 1; }
}
edit() { jq "$@" "$MOCK_ROOT/config.json" > "$MOCK_ROOT/next"; mv "$MOCK_ROOT/next" "$MOCK_ROOT/config.json"; }
trap 'if [[ -n ${MOCK_ROOT:-} ]]; then cat "$MOCK_ROOT/stderr" >&2; fi' ERR
count=0
pass() { count=$((count+1)); printf 'PASS %s\n' "$1"; }
new_fixture
invoke plan
[[ ! -s $MOCK_ROOT/mutations && ! -f $MOCK_ROOT/state/state.json ]]
reject apply
[[ ! -s $MOCK_ROOT/mutations ]]
pass 'plan is read-only and apply requires physical UID'
invoke apply --cluster-uid cluster-1
[[ $(wc -l < "$MOCK_ROOT/mutations") == 1 ]]
invoke apply --cluster-uid cluster-1
invoke verify
[[ $(wc -l < "$MOCK_ROOT/mutations") == 1 ]]
grep -q NOOP "$MOCK_ROOT/stdout"
pass 'second identical apply and verify perform zero mutations'
echo 'example: changed' > "$MOCK_ROOT/values.yaml"
invoke apply --cluster-uid cluster-1
[[ $(tail -1 "$MOCK_ROOT/mutations") == upgrade:obs-metrics ]]
pass 'intentional values change updates only owned release'
echo '{"example":"external-change"}' > "$MOCK_ROOT/live-values.json"
reject apply --cluster-uid cluster-1
[[ $(wc -l < "$MOCK_ROOT/mutations") == 2 ]]
pass 'out-of-band Helm values drift is blocked'
new_fixture
printf '[{"name":"obs-metrics","namespace":"monitoring","status":"deployed","owned":false}]' > "$MOCK_ROOT/releases.json"
reject apply --cluster-uid cluster-1
[[ ! -s $MOCK_ROOT/mutations ]]
pass 'foreign release is never adopted'
new_fixture
edit '.components.grafana={mode:"install"}'
rm "$MOCK_ROOT/namespace.json"
echo '{"items":[{"metadata":{"name":"existing-grafana","namespace":"shared"},"spec":{"template":{"spec":{"containers":[{"image":"grafana/grafana:12"}]}}}}]}' > "$MOCK_ROOT/workloads.json"
reject apply --cluster-uid cluster-1
[[ ! -s $MOCK_ROOT/mutations ]]
pass 'existing component under another release blocks before namespace mutation'
edit '.components.grafana={mode:"reuse",resources:[{kind:"deployment",namespace:"shared",name:"existing-grafana"}]}'
echo '{"kind":"Deployment","metadata":{"uid":"external-1","generation":1},"spec":{"replicas":1},"status":{"observedGeneration":1,"readyReplicas":1,"updatedReplicas":1}}' > "$MOCK_ROOT/reused.json"
invoke apply --cluster-uid cluster-1
grep -q 'grafana.enabled=false' "$MOCK_ROOT/template-args"
[[ $(wc -l < "$MOCK_ROOT/mutations") == 2 ]]
jq '.metadata.uid="external-2"' "$MOCK_ROOT/reused.json" > "$MOCK_ROOT/next"; mv "$MOCK_ROOT/next" "$MOCK_ROOT/reused.json"
reject apply --cluster-uid cluster-1
[[ $(wc -l < "$MOCK_ROOT/mutations") == 2 ]]
pass 'partial stack reuse disables bundled Grafana and pins reused UID'
new_fixture
invoke apply --cluster-uid cluster-1
echo cluster-2 > "$MOCK_ROOT/cluster"
reject apply --cluster-uid cluster-2
[[ $(wc -l < "$MOCK_ROOT/mutations") == 1 ]]
pass 'copied receipt cannot mutate another cluster'
new_fixture
invoke apply --cluster-uid cluster-1
echo '{"metadata":{"uid":"namespace-replacement"}}' > "$MOCK_ROOT/namespace.json"
reject apply --cluster-uid cluster-1
pass 'replaced namespace is rejected'
new_fixture
touch "$MOCK_ROOT/install-failure"
reject apply --cluster-uid cluster-1
jq -e '.releases.metrics.phase=="intent"' "$MOCK_ROOT/state/state.json" >/dev/null
rm "$MOCK_ROOT/install-failure"
error_hash=$(sha256sum "$MOCK_ROOT/state/last-error.log" | cut -d' ' -f1)
reject apply --cluster-uid cluster-1
[[ $(sha256sum "$MOCK_ROOT/state/last-error.log" | cut -d' ' -f1) == "$error_hash" ]]
[[ $(wc -l < "$MOCK_ROOT/mutations") == 1 ]]
pass 'partial failure retains intent and blocks blind retry'
new_fixture
touch "$MOCK_ROOT/render-failure"
reject apply --cluster-uid cluster-1
[[ ! -s $MOCK_ROOT/mutations ]]
pass 'render errors are private and occur before mutation'
new_fixture
echo tampered > "$MOCK_ROOT/state/metrics-$checksum.tgz"
reject apply --cluster-uid cluster-1
[[ ! -s $MOCK_ROOT/mutations ]]
pass 'archive checksum mismatch blocks installation'
new_fixture
touch "$MOCK_ROOT/rbac-failure"
reject plan
[[ ! -s $MOCK_ROOT/mutations ]]
pass 'inventory permission errors never become missing resources'
new_fixture
echo '{"items":[{"metadata":{"name":"prometheuses.monitoring.coreos.com"},"spec":{"group":"monitoring.coreos.com"}}]}' > "$MOCK_ROOT/crds.json"
reject apply --cluster-uid cluster-1
[[ ! -s $MOCK_ROOT/mutations ]]
pass 'existing CRDs require explicit reuse and compatibility review'
new_fixture
invoke apply --cluster-uid cluster-1
# Operator creates StatefulSets with its own instance label: follow immutable owner UID.
echo '{"items":[{"metadata":{"uid":"prom-cr-1","namespace":"monitoring","annotations":{"meta.helm.sh/release-name":"obs-metrics"}}}]}' > "$MOCK_ROOT/controllers.json"
echo '{"items":[{"metadata":{"name":"prometheuses.monitoring.coreos.com"},"spec":{"group":"monitoring.coreos.com"}}]}' > "$MOCK_ROOT/crds.json"
jq '.items = [(.items[]|select(.metadata.name=="owned-operator")), {kind:"StatefulSet",metadata:{name:"operator-created",namespace:"monitoring",generation:1,ownerReferences:[{uid:"prom-cr-1"}]},spec:{replicas:1,template:{spec:{containers:[{image:"quay.io/prometheus/prometheus:v3"}]}}},status:{observedGeneration:1,readyReplicas:1,updatedReplicas:1}}]' "$MOCK_ROOT/workloads.json" > "$MOCK_ROOT/next-workloads.json"
mv "$MOCK_ROOT/next-workloads.json" "$MOCK_ROOT/workloads.json"
invoke apply --cluster-uid cluster-1
[[ $(wc -l < "$MOCK_ROOT/mutations") == 1 ]]
pass 'operator-owned workload identity does not trigger duplicate install'
new_fixture
invoke apply --cluster-uid cluster-1
edit '.components.prometheus={mode:"skip",reason:"remove"}'
reject apply --cluster-uid cluster-1
[[ $(wc -l < "$MOCK_ROOT/mutations") == 1 ]]
pass 'managed components cannot be removed by switching config to skip'
new_fixture
edit '.releases += [{id:"loki",name:"obs-loki",valuesFile:"values.yaml",chart:{name:"loki",version:"1.0.0",url:"https://example.invalid/chart.tgz",sha256:"0000000000000000000000000000000000000000000000000000000000000000"}}] | .components.loki={mode:"install"}'
echo corrupt > "$MOCK_ROOT/state/loki-0000000000000000000000000000000000000000000000000000000000000000.tgz"
reject apply --cluster-uid cluster-1
[[ ! -s $MOCK_ROOT/mutations ]]
pass 'a later invalid chart blocks all earlier release mutations'
new_fixture
edit '.components.grafana={mode:"reuse",resources:[{kind:"deployment",namespace:"shared",name:"grafana"}]}'
echo '{"kind":"Deployment","metadata":{"uid":"external-1","generation":2},"spec":{"replicas":1},"status":{"observedGeneration":1,"readyReplicas":1,"updatedReplicas":1}}' > "$MOCK_ROOT/reused.json"
reject apply --cluster-uid cluster-1
[[ ! -s $MOCK_ROOT/mutations ]]
pass 'stale/unready reused controllers block installation'
new_fixture
invoke apply --cluster-uid cluster-1
jq '.items|=map(select(.metadata.name!="owned-prometheus"))' "$MOCK_ROOT/workloads.json" > "$MOCK_ROOT/next-workloads.json"
mv "$MOCK_ROOT/next-workloads.json" "$MOCK_ROOT/workloads.json"
reject verify
pass 'operator Ready alone does not prove Prometheus exists'
new_fixture
cp "$MOCK_ROOT/values.yaml" "$MOCK_ROOT/values with spaces.yaml"
edit '.releases[0].valuesFile="values with spaces.yaml"'
invoke apply --cluster-uid cluster-1
invoke apply --cluster-uid cluster-1
[[ $(wc -l < "$MOCK_ROOT/mutations") == 1 ]]
pass 'values paths with spaces remain idempotent'
new_fixture
edit '.crds="reuse" | .components.operator={mode:"reuse",resources:[{kind:"deployment",namespace:"shared",name:"operator"}]}'
echo '{"kind":"Deployment","metadata":{"uid":"external-operator-1","generation":1},"spec":{"replicas":1},"status":{"observedGeneration":1,"readyReplicas":1,"updatedReplicas":1}}' > "$MOCK_ROOT/reused.json"
jq -n '{items:(["prometheuses","alertmanagers","servicemonitors","podmonitors","prometheusrules"]|map({metadata:{name:(.+".monitoring.coreos.com")},spec:{group:"monitoring.coreos.com",versions:[{name:"v1",served:true}]}}))}' > "$MOCK_ROOT/crds.json"
invoke apply --cluster-uid cluster-1
grep -q 'prometheusOperator.enabled=false' "$MOCK_ROOT/template-args"
grep -q 'crds.enabled=false' "$MOCK_ROOT/template-args"
pass 'reuse operator and served CRDs disables their installation'
new_fixture
rm "$MOCK_ROOT/state/.observability-installer"
chmod 755 "$MOCK_ROOT/state"
reject plan
[[ $(stat -c %a "$MOCK_ROOT/state") == 755 && -f $MOCK_ROOT/state/metrics-$checksum.tgz ]]
pass 'unrelated nonempty state directory is not chmodded or overwritten'
new_fixture
invoke apply --cluster-uid cluster-1
echo 'example: changed' > "$MOCK_ROOT/values.yaml"
touch "$MOCK_ROOT/install-failure"
reject apply --cluster-uid cluster-1
jq -e '.releases.metrics.phase=="intent" and .releases.metrics.previous.phase=="done" and (.releases.metrics.previous.valuesHash|length>0)' "$MOCK_ROOT/state/state.json" >/dev/null
pass 'failed upgrade retains the previous completed receipt for reconciliation'
new_fixture
invoke apply --cluster-uid cluster-1
echo 'example: changed' > "$MOCK_ROOT/values.yaml"
touch "$MOCK_ROOT/install-failure"
reject apply --cluster-uid cluster-1
rm "$MOCK_ROOT/install-failure"
jq 'map(.status="deployed")' "$MOCK_ROOT/releases.json" > "$MOCK_ROOT/next"; mv "$MOCK_ROOT/next" "$MOCK_ROOT/releases.json"
echo '{"example":"changed"}' > "$MOCK_ROOT/live-values.json"
echo '{"example":"changed"}' > "$MOCK_ROOT/projected-values.json"
reject reconcile
invoke reconcile --cluster-uid cluster-1
jq -e '.releases.metrics.phase=="done"' "$MOCK_ROOT/state/state.json" >/dev/null
[[ $(wc -l < "$MOCK_ROOT/mutations") == 2 ]]
invoke apply --cluster-uid cluster-1
[[ $(wc -l < "$MOCK_ROOT/mutations") == 2 ]]
pass 'reviewed repair reconciles only matching saved intent and never mutates releases'
echo 'example: different' > "$MOCK_ROOT/values.yaml"
touch "$MOCK_ROOT/install-failure"
reject apply --cluster-uid cluster-1
rm "$MOCK_ROOT/install-failure"
jq 'map(.status="deployed")' "$MOCK_ROOT/releases.json" > "$MOCK_ROOT/next"; mv "$MOCK_ROOT/next" "$MOCK_ROOT/releases.json"
echo '{"example":"foreign"}' > "$MOCK_ROOT/live-values.json"
reject reconcile --cluster-uid cluster-1
jq -e '.releases.metrics.phase=="intent"' "$MOCK_ROOT/state/state.json" >/dev/null
pass 'reconciliation refuses repaired values differing from intent'
printf '%s contract cases passed (simulated cluster, not live acceptance).\n' "$count"
