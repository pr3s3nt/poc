#!/usr/bin/env bash
# Run-scoped UC-16 -> GitRepo -> Fleet -> Harbor image verification on kind.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
WORK="$(mktemp -d)"
GITOPS_REPO="${FLEET_GITOPS_REPO:-/home/thanhnt1/idp_lab/read_code/poc-fleet-gitops}"
DOCKER_CONFIG_FILE="${HARBOR_DOCKERCONFIG_FILE:-/home/thanhnt1/.local/share/poc-harbor/dockerconfig.json}"
KUBE_CONTEXT="kind-idp-internal"
REGISTRY="10.96.91.170:80"
IMAGE="${REGISTRY}/library/busybox:1.37-poc"
RUN_ID="fleet-$(date -u +%Y%m%d%H%M%S)-$RANDOM"
NAMESPACE=""
DEPLOY_RUN_ID=""
APP_ID=""
API_PID=""

cleanup() {
  local status=$?
  [[ -z "${API_PID}" ]] || kill "${API_PID}" 2>/dev/null || true
  if [[ -n "${NAMESPACE}" ]] && kubectl --context "${KUBE_CONTEXT}" get namespace "${NAMESPACE}" >/dev/null 2>&1; then
    local actual
    actual="$(kubectl --context "${KUBE_CONTEXT}" get namespace "${NAMESPACE}" -o jsonpath='{.metadata.labels.orchestrator\.io/run-id}' 2>/dev/null || true)"
    if [[ -n "${DEPLOY_RUN_ID}" && "${actual}" == "${DEPLOY_RUN_ID}" && ! -d "${GITOPS_REPO}/applications/${APP_ID}/staging/probe" && ! -d "${GITOPS_REPO}/applications/${APP_ID}/staging/_routes" ]]; then
      kubectl --context "${KUBE_CONTEXT}" delete namespace "${NAMESPACE}" --wait=true >/dev/null
    else
      echo "scoped namespace retained for diagnosis: ${NAMESPACE}" >&2
      status=1
    fi
  fi
  echo "run-id=${RUN_ID} evidence=${WORK} status=${status}"
  exit "${status}"
}
trap cleanup EXIT

[[ "$(kubectl config current-context)" == "${KUBE_CONTEXT}" ]]
[[ -s "${DOCKER_CONFIG_FILE}" ]]
[[ "$(git -C "${GITOPS_REPO}" status --porcelain)" == "" ]]
kubectl --context "${KUBE_CONTEXT}" -n fleet-local get gitrepo poc-workloads >/dev/null
cd "${ROOT}"
go build -o "${WORK}/orchestrator" ./cmd/orchestrator
"${WORK}/orchestrator" -addr 127.0.0.1:0 -addr-file "${WORK}/api-addr" -state "${WORK}/state.json" \
  -adapters kubernetes -kube-context "${KUBE_CONTEXT}" -cluster idp-internal -run-id "${RUN_ID}" \
  -workload-delivery fleet-gitrepo -gitops-repo-dir "${GITOPS_REPO}" -gitops-branch main \
  -fleet-gitrepo-name poc-workloads -harbor-registry "${REGISTRY}" \
  -harbor-dockerconfig-file "${DOCKER_CONFIG_FILE}" > "${WORK}/orchestrator.log" 2>&1 &
API_PID=$!
for _ in $(seq 1 80); do [[ -s "${WORK}/api-addr" ]] && break; sleep 0.5; done
API="http://$(<"${WORK}/api-addr")/api/v1"
for _ in $(seq 1 40); do curl -fsS "${API}/healthz" >/dev/null 2>&1 && break; sleep 0.5; done
curl -fsS -c "${WORK}/cookies" -H 'Content-Type: application/json' \
  --data '{"username":"developer","password":"test-password"}' "${API}/auth/sign-in" >/dev/null
APP_ID="$(curl -fsS -b "${WORK}/cookies" -H 'Content-Type: application/json' \
  --data "{\"name\":\"Fleet ${RUN_ID}\",\"subdomain\":\"${RUN_ID}\"}" "${API}/applications" | jq -r '.application.key')"
[[ -n "${APP_ID}" && "${APP_ID}" != "null" ]]
NAMESPACE="app-${APP_ID}-staging"
BASE="${API}/applications/${APP_ID}/environments/staging"
jq -n --arg image "${IMAGE}" '{apiVersion:"score.dev/v1b1",metadata:{name:"probe"},containers:{main:{image:$image,command:["/bin/sh","-c"],args:["mkdir -p /tmp/www; printf fleet-ok >/tmp/www/index.html; exec httpd -f -p 8080 -h /tmp/www"]}},service:{ports:{http:{port:8080,targetPort:8080}},publicRoutes:[{path:"/",port:"http"}]}}' \
  | jq '{score:.,version:0}' \
  | curl -fsS -b "${WORK}/cookies" -X PUT -H 'Content-Type: application/json' --data-binary @- "${BASE}/workloads/probe" > "${WORK}/draft.json"
curl -fsS -b "${WORK}/cookies" -X POST -H 'Content-Type: application/json' --data '{}' "${BASE}/preview" > "${WORK}/preview.json"
jq -e '(.changes | length) == 1 and .changes[0].action == "DEPLOY"' "${WORK}/preview.json" >/dev/null
DEPLOY_RUN_ID="$(jq -r .runId "${WORK}/preview.json")"
jq '{token:.token}' "${WORK}/preview.json" | curl -fsS -b "${WORK}/cookies" -X POST -H 'Content-Type: application/json' --data-binary @- "${BASE}/deploy" > "${WORK}/deploy.json"
jq -e '.status == "SUCCEEDED"' "${WORK}/deploy.json" >/dev/null
kubectl --context "${KUBE_CONTEXT}" -n "${NAMESPACE}" rollout status deployment/probe --timeout=180s
kubectl --context "${KUBE_CONTEXT}" -n "${NAMESPACE}" get deployment probe -o json > "${WORK}/deployment.json"
jq -e --arg image "${IMAGE}" '.spec.template.spec.containers[0].image == $image and .spec.template.spec.imagePullSecrets[0].name == "harbor-pull"' "${WORK}/deployment.json" >/dev/null
kubectl --context "${KUBE_CONTEXT}" -n "${NAMESPACE}" get secret harbor-pull -o json | jq -e '.type == "kubernetes.io/dockerconfigjson"' >/dev/null
test -f "${GITOPS_REPO}/applications/${APP_ID}/staging/probe/deployment-probe.json"
test -f "${GITOPS_REPO}/applications/${APP_ID}/staging/_routes/ingress-orch-public.json"
kubectl --context "${KUBE_CONTEXT}" -n "${NAMESPACE}" get ingress orch-public -o json > "${WORK}/ingress.json"
jq -e --arg host "staging.${RUN_ID}.example.com" '.spec.rules[0].host == $host and .spec.rules[0].http.paths[0].backend.service.name == "probe" and (.metadata.labels["orchestrator.io/route-hash"] | length) > 0 and (.metadata.annotations["objectset.rio.cattle.io/id"] | length) > 0' "${WORK}/ingress.json" >/dev/null
if rg -q '"(auth|password|secret)"[[:space:]]*:' "${GITOPS_REPO}/applications/${APP_ID}/staging/probe" "${GITOPS_REPO}/applications/${APP_ID}/staging/_routes"; then echo "credential key found in GitOps manifest" >&2; exit 1; fi

curl -fsS -b "${WORK}/cookies" "${BASE}/workloads" > "${WORK}/workloads.json"
jq '{version:.draftVersion}' "${WORK}/workloads.json" | curl -fsS -b "${WORK}/cookies" -X DELETE -H 'Content-Type: application/json' --data-binary @- "${BASE}/workloads/probe" > "${WORK}/delete-draft.json"
curl -fsS -b "${WORK}/cookies" -X POST -H 'Content-Type: application/json' --data '{}' "${BASE}/preview" > "${WORK}/remove-preview.json"
jq -e '(.changes | length) == 1 and .changes[0].action == "REMOVE"' "${WORK}/remove-preview.json" >/dev/null
jq '{token:.token}' "${WORK}/remove-preview.json" | curl -fsS -b "${WORK}/cookies" -X POST -H 'Content-Type: application/json' --data-binary @- "${BASE}/deploy" > "${WORK}/remove-deploy.json"
jq -e '.status == "SUCCEEDED"' "${WORK}/remove-deploy.json" >/dev/null
if kubectl --context "${KUBE_CONTEXT}" -n "${NAMESPACE}" get deployment probe >/dev/null 2>&1; then echo "Fleet did not remove Deployment" >&2; exit 1; fi
if [[ -d "${GITOPS_REPO}/applications/${APP_ID}/staging/_routes" ]] || kubectl --context "${KUBE_CONTEXT}" -n "${NAMESPACE}" get ingress orch-public >/dev/null 2>&1; then echo "Fleet did not prune public route" >&2; exit 1; fi
echo "Fleet GitRepo + Harbor kind verification passed"
