#!/usr/bin/env bash
# Operator tooling. No eval, implicit context, takeover, reset or automatic deletion.
set -Eeuo pipefail
umask 077
root=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
fail() { printf 'ERROR: %s\n' "$*" >&2; exit 1; }
usage() {
  cat <<'HELP'
Usage: setup.sh plan|apply|verify --config FILE --state-dir DIR [--cluster-uid UID]
Requires Bash 4.4+, Helm 3.19.x, kubectl, jq, curl, sha256sum, flock and realpath (Linux).
plan: read cluster and prepare private local artifacts; no cluster writes.
apply: preflight all releases, then install/update owned releases; UID required.
verify: read-only workload readiness; does not prove telemetry/notification delivery.
See README.md for reuse, partial failure and CRD/upgrade boundaries.
HELP
}
(( BASH_VERSINFO[0]>4 || (BASH_VERSINFO[0]==4 && BASH_VERSINFO[1]>=4) )) || fail 'Bash 4.4+ required'
[[ $# -gt 0 ]] || { usage; exit 1; }
action=$1; shift
case "$action" in plan|apply|verify) ;; -h|--help) usage; exit 0 ;; *) fail 'Unknown command' ;; esac
config='' state_dir='' expected_cluster=''
while (($#)); do
  (($# >= 2)) || fail 'Option requires a value'
  case "$1" in
    --config) config=$2 ;; --state-dir) state_dir=$2 ;; --cluster-uid) expected_cluster=$2 ;;
    *) fail "Unknown option: $1" ;;
  esac
  shift 2
done
[[ -n $config && -n $state_dir ]] || fail '--config and --state-dir required'
for tool in helm kubectl jq curl sha256sum flock realpath; do command -v "$tool" >/dev/null || fail "Missing dependency: $tool"; done
config=$(realpath -e -- "$config")
config_dir=$(dirname -- "$config")
state_dir=$(realpath -m -- "$state_dir")
# Receipts, rendered Secrets and command logs must never enter the source tree.
source_tree=$(git -C "$root" rev-parse --show-toplevel 2>/dev/null || printf '%s' "$root")
[[ $state_dir != "$source_tree" && $state_dir != "$source_tree/"* && $state_dir != / ]] || fail 'State directory must be outside the source/bundle tree'
if [[ -d $state_dir && ! -f $state_dir/.observability-installer ]]; then
  [[ -z $(find "$state_dir" -mindepth 1 -maxdepth 1 -print -quit) ]] || fail 'Refusing a nonempty unrelated state directory'
fi
mkdir -p -- "$state_dir"
[[ -O $state_dir && ! -L $state_dir ]] || fail 'State directory must belong to current user'
chmod 700 "$state_dir"
printf 'observability-installer-v1\n' > "$state_dir/.observability-installer"
exec 9>"$state_dir/lock"
flock -n 9 || fail 'Another installer is using this state directory'
work=$(mktemp -d "$state_dir/work.XXXXXX")
trap 'rm -rf -- "$work"' EXIT
# Capture errors privately: Helm output/notes may contain credentials.
run() {
  if ! "$@" 2>"$work/command-error.log"; then
    error_receipt=$(mktemp "$state_dir/error.XXXXXX.log")
    cp "$work/command-error.log" "$error_receipt"
    cp "$work/command-error.log" "$state_dir/last-error.log"
    fail "Command failed ($1); inspect private $error_receipt and reconcile before retry"
  fi
}
version=$(run helm version --template '{{.Version}}')
[[ $version =~ ^v3\.19\.[0-9]+([+-].*)?$ ]] || fail 'This installer is validated with Helm 3.19.x; use that pinned minor version'
jq -e '
 def name: type=="string" and test("^[a-z0-9]([a-z0-9-]*[a-z0-9])?$") and length<=53;
 .schemaVersion==1 and (.installationId|name) and (.context|type=="string" and length>0 and (startswith("CHANGE_")|not))
 and (.namespace|name) and (.createNamespace|type=="boolean")
 and (.storageClass|name) and (.timeout|type=="string" and test("^[1-9][0-9]*(s|m)$"))
 and (.crds=="install" or .crds=="reuse")
 and (.components|type=="object" and length>0)
 and ((["operator","prometheus","alertmanager","grafana","kubeStateMetrics","nodeExporter","loki","alloy"] - (.components|keys))|length==0)
 and ([.components[]|.mode=="install" or .mode=="reuse" or .mode=="skip"]|all)
 and ([.components|to_entries[]|.key|IN("operator","prometheus","alertmanager","grafana","kubeStateMetrics","nodeExporter","loki","alloy","postgresExporter","blackboxExporter")]|all)
 and ([.components[]|select(.mode=="skip")|.reason|type=="string" and length>0]|all)
 and ([.components[]|select(.mode=="reuse")|.resources|type=="array" and length>0]|all)
 and ([.components[]|select(.mode=="reuse")|.resources[]|
   (.kind|IN("deployment","statefulset","daemonset")) and (.name|name) and (.namespace|name)]|all)
 and (.releases|type=="array")
 and ([.releases[].id]|length== (unique|length))
 and ([.releases[].name]|length== (unique|length))
 and ([.releases[]|(.id|IN("metrics","loki","alloy","postgresExporter","blackboxExporter"))
   and (.name|name) and (.chart.name|name)
   and (.chart.version|type=="string" and test("^[0-9]+\\.[0-9]+\\.[0-9]+$"))
   and (.chart.url|type=="string" and test("^https://[^/@]+/[^?#]+\\.tgz$"))
   and (.chart.sha256|type=="string" and test("^[a-f0-9]{64}$"))
   and (.valuesFile|type=="string" and length>0)]|all)
' "$config" >/dev/null || fail 'Invalid configuration; see examples (placeholders must be replaced)'
installation=$(jq -r .installationId "$config")
context=$(jq -r .context "$config")
namespace=$(jq -r .namespace "$config")
timeout=$(jq -r .timeout "$config")
storage=$(jq -r .storageClass "$config")
crds=$(jq -r .crds "$config")
k() { run kubectl --context "$context" --request-timeout=30s "$@"; }
h() { run helm "$@" --kube-context "$context"; }
state="$state_dir/state.json"
kube_version=$(k version -o json | jq -er '.serverVersion.gitVersion|select(length>0)')
cluster=$(k get namespace kube-system -o json | jq -er '.metadata.uid|select(length>0)')
[[ -z $expected_cluster || $cluster == "$expected_cluster" ]] || fail 'Physical cluster UID does not match'
[[ $action != apply || $expected_cluster == "$cluster" ]] || fail 'apply requires --cluster-uid from plan'
if [[ -f $state ]]; then
  jq -e --arg id "$installation" --arg cluster "$cluster" --arg ns "$namespace" '
    .schemaVersion==1 and .installationId==$id and .clusterUID==$cluster and .namespace==$ns
  ' "$state" >/dev/null || fail 'Receipt belongs to another installation/cluster/namespace; do not reset it'
  cp "$state" "$work/state.json"
else
  jq -n --arg id "$installation" --arg c "$cluster" --arg ns "$namespace" \
    '{schemaVersion:1,installationId:$id,clusterUID:$c,namespace:$ns,releases:{},reused:{}}' > "$work/state.json"
fi
save_state() { cp "$work/state.json" "$state_dir/state.tmp"; chmod 600 "$state_dir/state.tmp"; mv "$state_dir/state.tmp" "$state"; }
mutate_state() { jq "$@" "$work/state.json" > "$work/next.json"; mv "$work/next.json" "$work/state.json"; }
ns=$(k get namespace "$namespace" --ignore-not-found -o json)
[[ -n $ns ]] || ns='{}'
ns_uid=$(jq -r '.metadata.uid // ""' <<<"$ns")
prior_ns=$(jq -r '.namespaceUID // ""' "$work/state.json")
[[ -z $prior_ns || $prior_ns == "$ns_uid" ]] || fail 'Namespace was removed/replaced; reconcile identity'
[[ -n $ns_uid || $(jq -r .createNamespace "$config") == true ]] || fail 'Namespace absent and createNamespace=false'
k get storageclass "$storage" -o name >/dev/null
k get deployments.apps,statefulsets.apps,daemonsets.apps -A -o json > "$work/workloads.json"
k get crds -o json > "$work/crds.json"
printf '{"items":[]}' > "$work/controllers.json"
for resource in prometheuses alertmanagers; do
  if jq -e --arg name "$resource.monitoring.coreos.com" '.items[]|select(.metadata.name==$name)' "$work/crds.json" >/dev/null; then
    k get "$resource.monitoring.coreos.com" -A -o json > "$work/controller-part.json"
    jq -s '{items:([.[].items[]])}' "$work/controllers.json" "$work/controller-part.json" > "$work/controller-next.json"
    mv "$work/controller-next.json" "$work/controllers.json"
  fi
done
h list -A --all --max 10000 -o json > "$work/releases.json"
h list -A --all --max 10000 --selector "orchestrator.dev/installer=$installation" -o json > "$work/owned.json"
[[ $(jq length "$work/releases.json") -lt 10000 ]] || fail 'Release inventory truncated'
# Never interpret an inventory/RBAC/network error as an absent component.
mode() { jq -r --arg key "$1" '.components[$key].mode // "skip"' "$config"; }
group() {
  case "$1" in operator|prometheus|alertmanager|grafana|kubeStateMetrics|nodeExporter) printf metrics ;; *) printf '%s' "$1" ;; esac
}
image_pattern() {
  case "$1" in
    operator) printf '(^|/)prometheus-operator(:|@)' ;; prometheus) printf '(^|/)prometheus(:|@)' ;;
    alertmanager) printf '(^|/)alertmanager(:|@)' ;; grafana) printf '(^|/)grafana(:|@)' ;;
    kubeStateMetrics) printf '(^|/)kube-state-metrics(:|@)' ;; nodeExporter) printf '(^|/)node-exporter(:|@)' ;;
    loki) printf '(^|/)loki(:|@)' ;; alloy) printf '(^|/)alloy(:|@)' ;;
    postgresExporter) printf '(^|/)postgres-exporter(:|@)' ;; blackboxExporter) printf '(^|/)blackbox-exporter(:|@)' ;;
  esac
}
# Every install mode must have its corresponding chart; every chart must be needed.
for component in operator prometheus alertmanager grafana kubeStateMetrics nodeExporter loki alloy postgresExporter blackboxExporter; do
  [[ $(mode "$component") == install ]] || continue
  id=$(group "$component")
  jq -e --arg id "$id" '.releases[]|select(.id==$id)' "$config" >/dev/null || fail "Missing chart for $component"
  name=$(jq -r --arg id "$id" '.releases[]|select(.id==$id)|.name' "$config")
  # Known upstream image fingerprints detect duplicates under different names/releases.
  # Unrecognized custom images require operator inventory; never claim exhaustive discovery.
  conflicts=$(jq -r --slurpfile controllers "$work/controllers.json" --arg pattern "$(image_pattern "$component")" --arg name "$name" --arg ns "$namespace" '
    .items[] | select(any(.spec.template.spec.containers[]?; .image|test($pattern))) |
    select((.metadata.annotations["meta.helm.sh/release-name"] // .metadata.labels["app.kubernetes.io/instance"] // "")!=$name or .metadata.namespace!=$ns) |
    . as $workload | select(any($controllers[0].items[]; .metadata.namespace==$ns and .metadata.annotations["meta.helm.sh/release-name"]==$name and
      (.metadata.uid as $uid | any($workload.metadata.ownerReferences[]?; .uid==$uid))) | not) |
    .metadata.namespace+"/"+.metadata.name' "$work/workloads.json")
  [[ -z $conflicts ]] || fail "$component already exists ($conflicts); configure reuse or resolve duplicate explicitly"
done
# Reused controllers must exist and be ready. Identity is pinned on first apply.
ready() {
  jq -e '
   (.status.observedGeneration // 0) >= (.metadata.generation // 1) and
   (if .kind=="DaemonSet" then (.status.desiredNumberScheduled // 0)>0 and (.status.numberReady // 0)==.status.desiredNumberScheduled and (.status.updatedNumberScheduled // 0)==.status.desiredNumberScheduled
    else (.spec.replicas // 1)>0 and (.status.readyReplicas // 0)==(.spec.replicas // 1) and (.status.updatedReplicas // 0)==(.spec.replicas // 1) end)
  ' "$1" >/dev/null
}
while IFS= read -r ref; do
  kind=$(jq -r .kind <<<"$ref"); name=$(jq -r .name <<<"$ref"); ref_ns=$(jq -r .namespace <<<"$ref")
  key="$ref_ns/$kind/$name"
  k get "$kind" "$name" -n "$ref_ns" -o json > "$work/reused.json"
  ready "$work/reused.json" || fail "Reused controller not ready: $key"
  uid=$(jq -er .metadata.uid "$work/reused.json")
  old=$(jq -r --arg key "$key" '.reused[$key] // ""' "$work/state.json")
  expected=$(jq -r '.expectedUID // ""' <<<"$ref")
  [[ -z $old || $old == "$uid" ]] || fail "Reused controller replaced: $key; reconcile before retry"
  [[ -z $expected || $expected == "$uid" ]] || fail "Unexpected reused controller UID: $key"
  # shellcheck disable=SC2016 # These are jq variables, not shell variables.
  mutate_state --arg key "$key" --arg uid "$uid" '.reused[$key]=$uid'
done < <(jq -c '.components[]|select(.mode=="reuse")|.resources[]' "$config")
# Orphaning/removing a managed release from config is not an uninstall operation.
while IFS= read -r id; do
  jq -e --arg id "$id" '.releases[]|select(.id==$id)' "$config" >/dev/null || fail "Managed release $id removed from config; reconcile explicitly"
done < <(jq -r '.releases|keys[]' "$work/state.json")
mapfile -t ids < <(jq -r '.releases[].id' "$config")
(( ${#ids[@]} > 0 )) || [[ $action != apply ]] || fail 'Nothing to install; use verify for reuse-only configuration'
printf 'Cluster UID: %s\n' "$cluster"
printf 'ACTION\tRELEASE\tNAMESPACE\n'
if [[ $(mode prometheus) == install || $(mode alertmanager) == install ]]; then
  [[ $(mode operator) != skip ]] || fail 'Prometheus/Alertmanager installation requires install or reuse of an operator'
fi
for id in "${ids[@]}"; do
  entry=$(jq -c --arg id "$id" '.releases[]|select(.id==$id)' "$config")
  name=$(jq -r .name <<<"$entry")
  enabled_components='[]'
  count=0
  for component in operator prometheus alertmanager grafana kubeStateMetrics nodeExporter loki alloy postgresExporter blackboxExporter; do
    if [[ $(group "$component") == "$id" && $(mode "$component") == install ]]; then
      count=$((count+1))
      enabled_components=$(jq -c --arg component "$component" '.+[$component]' <<<"$enabled_components")
    fi
  done
  ((count>0)) || fail "Chart $id has no install components; remove it from releases for reuse/skip"
  existing=$(jq -c --arg name "$name" --arg ns "$namespace" '[.[]|select(.name==$name and .namespace==$ns)][0] // null' "$work/releases.json")
  receipt=$(jq -c --arg id "$id" '.releases[$id] // null' "$work/state.json")
  operation=INSTALL
  if [[ $existing != null ]]; then
    [[ $receipt != null ]] || fail "Release $name exists without this installer's receipt; configure reuse, never adopt"
    jq -e --arg name "$name" --arg ns "$namespace" '.[]|select(.name==$name and .namespace==$ns)' "$work/owned.json" >/dev/null || fail "Release ownership mismatch: $name"
    [[ $(jq -r .name <<<"$receipt") == "$name" ]] || fail "Release name changed for $id"
    [[ $(jq -r .status <<<"$existing") == deployed ]] || fail "Release $name is not deployed; reconcile failed/pending release before retry"
    jq -e --argjson desired "$enabled_components" '(.components-$desired)|length==0' <<<"$receipt" >/dev/null || fail "Removing managed components requires explicit lifecycle migration: $id"
    operation=UPDATE
  elif [[ $receipt != null ]]; then
    fail "Previously attempted release $name is absent; reconcile uncertain outcome before retry"
  fi
  chart_version=$(jq -r .chart.version <<<"$entry")
  chart_name=$(jq -r .chart.name <<<"$entry")
  case "$id:$chart_name" in metrics:kube-prometheus-stack|loki:loki|alloy:alloy|postgresExporter:prometheus-postgres-exporter|blackboxExporter:prometheus-blackbox-exporter) ;; *) fail "Unexpected chart for $id" ;; esac
  archive="$state_dir/$id-$(jq -r .chart.sha256 <<<"$entry").tgz"
  checksum=$(jq -r .chart.sha256 <<<"$entry")
  if [[ ! -f $archive ]]; then
    run curl --fail --silent --show-error --location --proto '=https' --proto-redir '=https' --max-time 120 --max-filesize 134217728 \
      "$(jq -r .chart.url <<<"$entry")" -o "$work/chart.tgz"
    [[ $(sha256sum "$work/chart.tgz" | cut -d' ' -f1) == "$checksum" ]] || fail "Chart checksum mismatch: $id"
    mv "$work/chart.tgz" "$archive"
  fi
  [[ $(sha256sum "$archive" | cut -d' ' -f1) == "$checksum" ]] || fail "Cached chart checksum mismatch: $id"
  # Exact version/name checked independently from archive filename and config.
  h show chart "$archive" > "$work/chart.yaml"
  grep -Fxq "name: $chart_name" "$work/chart.yaml" || fail "Chart name mismatch: $id"
  grep -Fxq "version: $chart_version" "$work/chart.yaml" || fail "Chart version mismatch: $id"
  values=$(jq -r .valuesFile <<<"$entry")
  [[ $values == /* ]] || values="$config_dir/$values"
  values=$(realpath -e -- "$values")
  grep -Eq 'CHANGE_ME_[A-Z0-9_]+' "$values" && fail "Edit required placeholders in values for $id"
  cp "$values" "$work/$id.user-values.yaml"
  set_args=()
  if [[ $id == metrics ]]; then
    for pair in operator:prometheusOperator.enabled prometheus:prometheus.enabled alertmanager:alertmanager.enabled grafana:grafana.enabled kubeStateMetrics:kubeStateMetrics.enabled nodeExporter:nodeExporter.enabled; do
      component=${pair%%:*}; field=${pair#*:}; enabled=false
      [[ $(mode "$component") != install ]] || enabled=true
      set_args+=(--set "$field=$enabled")
    done
    if [[ $(mode prometheus) != install ]]; then
      set_args+=(--set defaultRules.create=false --set grafana.sidecar.datasources.defaultDatasourceEnabled=false)
    fi
    if [[ $crds == reuse ]]; then
      set_args+=(--set crds.enabled=false)
      for crd in prometheuses alertmanagers servicemonitors podmonitors prometheusrules; do
        jq -e --arg name "$crd.monitoring.coreos.com" '.items[]|select(.metadata.name==$name)|.spec.versions[]|select(.name=="v1" and .served==true)' "$work/crds.json" >/dev/null || fail "Required existing CRD unavailable: $crd"
      done
    elif [[ $receipt == null ]] && jq -e '.items[]|select(.spec.group=="monitoring.coreos.com")' "$work/crds.json" >/dev/null; then
      fail 'Monitoring CRDs already exist; choose crds=reuse after checking compatibility'
    fi
    set_args+=(--set crds.upgradeJob.enabled=false
      --set-string "prometheus.prometheusSpec.storageSpec.volumeClaimTemplate.spec.storageClassName=$storage"
      --set-string "alertmanager.alertmanagerSpec.storage.volumeClaimTemplate.spec.storageClassName=$storage"
      --set-string "grafana.persistence.storageClassName=$storage"
      --set-string "prometheus.prometheusSpec.externalLabels.cluster_uid=$cluster")
    if [[ $(mode prometheus) == install ]]; then
      identity_rule=$(jq -cn --arg uid "$cluster" '{groups:[{name:"orchestrator-cluster-identity",rules:[{record:"orchestrator_cluster_identity",expr:"vector(1)",labels:{cluster_uid:$uid}}]}]}')
      set_args+=(--set-json "additionalPrometheusRulesMap.installer-identity=$identity_rule")
    fi
  elif [[ $id == loki ]]; then
    set_args+=(--set-string "singleBinary.persistence.storageClass=$storage")
  fi
  # Save exact override argv privately, preserving spaces without eval/word splitting.
  printf '%s\0' "${set_args[@]}" > "$work/$id.args"
  [[ ${#set_args[@]} -gt 0 ]] || : > "$work/$id.args"
  digest=$({ printf '%s\n' "$checksum" "$namespace" "$name" "$crds"; cat "$work/$id.user-values.yaml" "$work/$id.args"; } | sha256sum | cut -d' ' -f1)
  if [[ $existing != null ]]; then
    # Compare live supplied Helm values as well as receipt; do not silently repair drift.
    h get values "$name" -n "$namespace" -o json > "$work/$id.live-values.json"
    live_hash=$(jq -Sc . "$work/$id.live-values.json" | sha256sum | cut -d' ' -f1)
    recorded_hash=$(jq -r '.valuesHash // ""' <<<"$receipt")
    [[ -n $recorded_hash && $recorded_hash == "$live_hash" ]] || fail "Live Helm values changed or prior attempt incomplete: $name; reconcile first"
    [[ $(jq -r .chart <<<"$existing") == "$(jq -r .chart <<<"$receipt")" ]] || fail "Live chart changed outside installer: $name"
    if [[ $(jq -r .digest <<<"$receipt") == "$digest" ]]; then operation=NOOP; fi
  fi
  if [[ $id == metrics && $receipt != null && $operation == UPDATE && $crds == install && $(jq -r .chart <<<"$receipt") != "$chart_name-$chart_version" ]]; then
    fail 'Operator/CRD version upgrade requires separate compatibility review; automatic CRD upgrades disabled'
  fi
  if [[ $action == verify && $operation != NOOP ]]; then fail "Configuration not applied: $name ($operation)"; fi
  h template "$name" "$archive" -n "$namespace" --include-crds --kube-version "$kube_version" -f "$work/$id.user-values.yaml" "${set_args[@]}" > "$work/$id.rendered.yaml"
  jq -n --arg operation "$operation" --arg id "$id" --arg name "$name" --arg digest "$digest" --arg archive "$archive" --arg chart "$chart_name-$chart_version" --argjson components "$enabled_components" \
    '{operation:$operation,id:$id,name:$name,digest:$digest,archive:$archive,chart:$chart,components:$components}' > "$work/$id.plan.json"
  printf '%s\t%s\t%s\n' "$operation" "$name" "$namespace"
done
# All inventory, ownership, archive and render checks above finish before any writes.
if [[ $action == plan ]]; then
  mkdir -p "$state_dir/rendered"
  chmod 700 "$state_dir/rendered"
  for id in "${ids[@]}"; do cp "$work/$id.rendered.yaml" "$state_dir/rendered/$id.yaml"; done
  jq -s --arg cluster "$cluster" --arg ns "$namespace" '{clusterUID:$cluster,namespace:$ns,releases:.}' "$work/"*.plan.json > "$state_dir/plan.json" 2>/dev/null || \
    jq -n --arg cluster "$cluster" --arg ns "$namespace" '{clusterUID:$cluster,namespace:$ns,releases:[]}' > "$state_dir/plan.json"
  printf 'Plan only. Private plan: %s/plan.json\n' "$state_dir"
  exit 0
fi
if [[ $action == apply ]]; then
  # Recheck target immediately before mutation; no current-context switching.
  [[ $(k get namespace kube-system -o json | jq -r .metadata.uid) == "$cluster" ]] || fail 'Cluster changed during preflight'
  current_ns=$(k get namespace "$namespace" --ignore-not-found -o json)
  [[ -n $current_ns ]] || current_ns='{}'
  [[ $(jq -r '.metadata.uid // ""' <<<"$current_ns") == "$ns_uid" ]] || fail 'Namespace changed during preflight'
  if [[ -z $ns_uid ]]; then
    jq -n --arg ns "$namespace" --arg id "$installation" '{apiVersion:"v1",kind:"Namespace",metadata:{name:$ns,labels:{"orchestrator.dev/installer":$id}}}' | k create -f - >/dev/null
    ns_uid=$(k get namespace "$namespace" -o json | jq -er .metadata.uid)
  fi
  # shellcheck disable=SC2016 # These are jq variables, not shell variables.
  mutate_state --arg uid "$ns_uid" '.namespaceUID=$uid'
  save_state
  for id in "${ids[@]}"; do
    plan="$work/$id.plan.json"; operation=$(jq -r .operation "$plan"); name=$(jq -r .name "$plan")
    [[ $operation != NOOP ]] || continue
    # Durable intent exists before Helm, including failed/uncertain outcomes.
    # shellcheck disable=SC2016 # These are jq variables, not shell variables.
    mutate_state --arg id "$id" --slurpfile plan "$plan" '.releases[$id]=($plan[0]+{phase:"intent",previous:(.releases[$id]|del(.previous))})'
    save_state
    mapfile -d '' -t set_args < "$work/$id.args"
    # Install never becomes an upgrade of a release that appeared after preflight.
    # Upgrade never becomes a reinstall of a deleted release.
    if [[ $operation == INSTALL ]]; then
      helm_args=(install "$name" "$(jq -r .archive "$plan")")
    else
      h list -n "$namespace" --all --selector "orchestrator.dev/installer=$installation" -o json > "$work/recheck.json"
      jq -e --arg name "$name" '.[]|select(.name==$name and .status=="deployed")' "$work/recheck.json" >/dev/null || fail 'Release changed during preflight'
      helm_args=(upgrade "$name" "$(jq -r .archive "$plan")" --reset-values)
    fi
    helm_args+=(-n "$namespace"
      --labels "orchestrator.dev/installer=$installation" --description "observability installer $installation"
      --wait --wait-for-jobs --timeout "$timeout" -f "$work/$id.user-values.yaml")
    [[ $crds != reuse ]] || helm_args+=(--skip-crds)
    h "${helm_args[@]}" "${set_args[@]}" > "$state_dir/last-helm.log"
    h get values "$name" -n "$namespace" -o json > "$work/$id.applied-values.json"
    applied_hash=$(jq -Sc . "$work/$id.applied-values.json" | sha256sum | cut -d' ' -f1)
    # shellcheck disable=SC2016 # These are jq variables, not shell variables.
    mutate_state --arg id "$id" --arg hash "$applied_hash" '.releases[$id].phase="done" | .releases[$id].valuesHash=$hash'
    save_state
  done
fi
# Read actual owned release workloads. Operators may create them after Helm returns.
verify_owned_once() {
  local verify_id=$1 verify_name=$2
  if [[ $verify_id == metrics ]]; then
    printf '{"items":[]}' > "$work/controllers.json"
    for resource in prometheuses alertmanagers; do
      k get "$resource.monitoring.coreos.com" -A -o json > "$work/controller-part.json"
      jq -s '{items:([.[].items[]])}' "$work/controllers.json" "$work/controller-part.json" > "$work/controller-next.json"
      mv "$work/controller-next.json" "$work/controllers.json"
    done
  fi
  k get deployments.apps,statefulsets.apps,daemonsets.apps -n "$namespace" -o json > "$work/verify.json"
  jq --slurpfile controllers "$work/controllers.json" --arg name "$verify_name" --arg ns "$namespace" '[.items[]| . as $workload |
    select((.metadata.annotations["meta.helm.sh/release-name"] // .metadata.labels["app.kubernetes.io/instance"] // "")==$name or
      any($controllers[0].items[]; .metadata.namespace==$ns and .metadata.annotations["meta.helm.sh/release-name"]==$name and
        (.metadata.uid as $uid | any($workload.metadata.ownerReferences[]?; .uid==$uid))))]' "$work/verify.json" > "$work/owned-workloads.json"
  for component in operator prometheus alertmanager grafana kubeStateMetrics nodeExporter loki alloy postgresExporter blackboxExporter; do
    [[ $(mode "$component") == install && $(group "$component") == "$verify_id" ]] || continue
    if ! jq -e --arg pattern "$(image_pattern "$component")" 'any(.[]; any(.spec.template.spec.containers[]?; .image|test($pattern)))' "$work/owned-workloads.json" >/dev/null; then
      verify_reason="Missing owned controller for $component"
      return 1
    fi
  done
  while IFS= read -r object; do
    printf '%s\n' "$object" > "$work/object.json"
    if ! ready "$work/object.json"; then
      verify_reason="Owned workload not ready: $(jq -r .metadata.name "$work/object.json")"
      return 1
    fi
  done < <(jq -c '.[]' "$work/owned-workloads.json")
}
verify_seconds=${timeout%?}
[[ $timeout != *m ]] || verify_seconds=$((verify_seconds*60))
for id in "${ids[@]}"; do
  name=$(jq -r .name "$work/$id.plan.json")
  verify_deadline=$((SECONDS+verify_seconds))
  until verify_owned_once "$id" "$name"; do
    (( SECONDS < verify_deadline )) || fail "$verify_reason; inspect status and rerun verify (no reset)"
    sleep 2
  done
done
printf 'Workload readiness verified. Telemetry coverage, retention and notifications require separate live checks.\n'
