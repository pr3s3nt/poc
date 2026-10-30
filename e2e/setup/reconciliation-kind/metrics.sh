#!/usr/bin/env bash
# Platform prerequisite for the real HPA happy path, only on this owned kind.
set -euo pipefail
run=${ORCH_RECON_RUN_DIR:?private run directory required}
cluster=$(cat "$run/cluster-name")
[[ $cluster == orch-reconciliation-* ]]
[[ $(docker inspect --format '{{.Id}}' "$cluster-control-plane") == "$(cat "$run/container-id")" ]]
kube=(kubectl --kubeconfig "$run/kubeconfig")
[[ $("${kube[@]}" get namespace kube-system -o jsonpath='{.metadata.uid}') == "$(cat "$run/kube-system-uid")" ]]
umask 077
[[ ! -e "$run/metrics-created.json" ]]
curl --fail --silent --show-error --location https://github.com/kubernetes-sigs/metrics-server/releases/download/v0.8.1/components.yaml -o "$run/metrics-v0.8.1.yaml"
"${kube[@]}" create --dry-run=client -f "$run/metrics-v0.8.1.yaml" -o json > "$run/metrics-input.json"
# kubectl emits one JSON document per source YAML document; wrap the stream.
python3 - "$run/metrics-input.json" <<'PYJSON'
import json,sys
path=sys.argv[1];raw=open(path).read();items=[];decoder=json.JSONDecoder()
while raw.strip():
 value,n=decoder.raw_decode(raw.lstrip());items.append(value);raw=raw.lstrip()[n:]
with open(path,'w') as f:json.dump({'apiVersion':'v1','kind':'List','items':items},f)
PYJSON
node --input-type=module - "$run" <<'JS'
import {readFileSync,writeFileSync} from 'node:fs';import {join} from 'node:path';import {execFileSync} from 'node:child_process';import assert from 'node:assert/strict';
const run=process.argv[2],list=JSON.parse(readFileSync(join(run,'metrics-input.json'),'utf8'));
for(const o of list.items){const args=['--kubeconfig',join(run,'kubeconfig'),'get',o.kind,o.metadata.name,'--ignore-not-found','-o','name'];if(o.metadata.namespace)args.push('-n',o.metadata.namespace);assert.equal(execFileSync('kubectl',args,{encoding:'utf8',stdio:['ignore','pipe','pipe']}).trim(),'','Refusing preexisting metrics object');o.metadata.labels??={};o.metadata.labels['orchestrator.test/run']='reconciliation-20261029';if(o.kind==='Deployment')o.spec.template.spec.containers[0].args.push('--kubelet-insecure-tls');}
writeFileSync(join(run,'metrics-install.json'),JSON.stringify(list),{mode:0o600});
JS
# kind's disposable self-signed kubelet certificate is the only TLS exception.
# Product Kubernetes/Git credential verification remains unchanged.
docker pull registry.k8s.io/metrics-server/metrics-server:v0.8.1 > "$run/metrics-image.log" 2>&1
# Select this node's platform: kind's --all-platforms import can request
# manifests Docker did not download from the multi-architecture image index.
[[ $(docker exec "$cluster-control-plane" uname -m) == x86_64 ]]
docker save registry.k8s.io/metrics-server/metrics-server:v0.8.1 -o "$run/metrics-image.tar"
docker exec -i "$cluster-control-plane" ctr --namespace=k8s.io images import --platform linux/amd64 --digests - < "$run/metrics-image.tar"
"${kube[@]}" create -f "$run/metrics-install.json" -o json > "$run/metrics-created.json"
"${kube[@]}" rollout status -n kube-system deployment/metrics-server --timeout=180s
"${kube[@]}" wait --for=condition=Available apiservice/v1beta1.metrics.k8s.io --timeout=180s
"${kube[@]}" top nodes
