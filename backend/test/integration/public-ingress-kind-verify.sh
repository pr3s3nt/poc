#!/usr/bin/env bash
# Run-scoped UC-16 -> Preview -> Deploy -> Traefik HTTP route on kind.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
WORK="$(mktemp -d)"
RUN_ID="route-$(date -u +%Y%m%d%H%M%S)-$RANDOM"
CONTEXT="kind-idp-internal"
NAMESPACE=""
DEPLOY_RUN_ID=""
API_PID=""
PROXY_PID=""

cleanup() {
  local status=$?
  [[ -z "${API_PID}" ]] || kill "${API_PID}" 2>/dev/null || true
  [[ -z "${PROXY_PID}" ]] || kill "${PROXY_PID}" 2>/dev/null || true
  if [[ -n "${NAMESPACE}" ]] && kubectl --context "${CONTEXT}" get namespace "${NAMESPACE}" >/dev/null 2>&1; then
    local actual
    actual="$(kubectl --context "${CONTEXT}" get namespace "${NAMESPACE}" -o jsonpath='{.metadata.labels.orchestrator\.io/run-id}' 2>/dev/null || true)"
    if [[ -n "${DEPLOY_RUN_ID}" && "${actual}" == "${DEPLOY_RUN_ID}" ]]; then
      kubectl --context "${CONTEXT}" delete namespace "${NAMESPACE}" --wait=true >/dev/null
    else
      echo "namespace ${NAMESPACE} retained: run-id label mismatch" >&2
      status=1
    fi
  fi
  echo "run-id=${RUN_ID} evidence=${WORK} status=${status}"
  exit "${status}"
}
trap cleanup EXIT

[[ "$(kubectl config current-context)" == "${CONTEXT}" ]]
kubectl --context "${CONTEXT}" -n traefik rollout status deployment/traefik --timeout=60s >/dev/null
cd "${ROOT}"
go build -o "${WORK}/orchestrator" ./cmd/orchestrator
"${WORK}/orchestrator" -addr 127.0.0.1:0 -addr-file "${WORK}/api-addr" -state "${WORK}/state.json" \
  -adapters kubernetes -kube-context "${CONTEXT}" -cluster idp-internal -run-id "${RUN_ID}" \
  -ui-dir "${ROOT}/../frontend/dist" > "${WORK}/orchestrator.log" 2>&1 &
API_PID=$!
for _ in $(seq 1 80); do [[ -s "${WORK}/api-addr" ]] && break; sleep 0.5; done
API="http://$(<"${WORK}/api-addr")/api/v1"
for _ in $(seq 1 40); do curl -fsS "${API}/healthz" >/dev/null 2>&1 && break; sleep 0.5; done
curl -fsS -c "${WORK}/cookies" -H 'Content-Type: application/json' \
  --data '{"username":"developer","password":"test-password"}' "${API}/auth/sign-in" >/dev/null
APP_ID="$(curl -fsS -b "${WORK}/cookies" -H 'Content-Type: application/json' \
  --data "{\"name\":\"Route ${RUN_ID}\",\"subdomain\":\"${RUN_ID}\"}" "${API}/applications" | jq -r '.application.key')"
[[ -n "${APP_ID}" && "${APP_ID}" != "null" ]]
NAMESPACE="app-${APP_ID}-staging"
BASE="${API}/applications/${APP_ID}/environments/staging"
HOST="staging.${RUN_ID}.example.com"

jq -n '{apiVersion:"score.dev/v1b1",metadata:{name:"probe"},containers:{main:{image:"busybox:1.37",command:["/bin/sh","-c"],args:["printf ingress-ok >/tmp/index.html; exec httpd -f -p 8080 -h /tmp"],readinessProbe:{path:"/",port:8080}}},service:{ports:{http:{port:8080,targetPort:8080}},publicPort:"http"}}' \
  | jq '{score:.,version:0}' \
  | curl -fsS -b "${WORK}/cookies" -X PUT -H 'Content-Type: application/json' --data-binary @- "${BASE}/workloads/probe" > "${WORK}/draft.json"
curl -fsS -b "${WORK}/cookies" -X POST -H 'Content-Type: application/json' --data '{}' "${BASE}/preview" > "${WORK}/preview.json"
jq -e '(.changes | length) == 1 and .changes[0].action == "DEPLOY"' "${WORK}/preview.json" >/dev/null
DEPLOY_RUN_ID="$(jq -r .runId "${WORK}/preview.json")"
jq '{token:.token}' "${WORK}/preview.json" | curl -fsS -b "${WORK}/cookies" -X POST -H 'Content-Type: application/json' --data-binary @- "${BASE}/deploy" > "${WORK}/deploy.json"
jq -e '.status == "SUCCEEDED"' "${WORK}/deploy.json" >/dev/null
kubectl --context "${CONTEXT}" -n "${NAMESPACE}" rollout status deployment/probe --timeout=180s >/dev/null
kubectl --context "${CONTEXT}" -n "${NAMESPACE}" get ingress orch-public -o json > "${WORK}/ingress.json"
jq -e --arg host "${HOST}" '.spec.ingressClassName == "traefik" and .spec.rules[0].host == $host and .spec.rules[0].http.paths[0].backend.service.name == "probe" and .spec.rules[0].http.paths[0].backend.service.port.name == "http"' "${WORK}/ingress.json" >/dev/null

kubectl --context "${CONTEXT}" -n traefik port-forward svc/traefik 18380:80 > "${WORK}/traefik-port-forward.log" 2>&1 &
PROXY_PID=$!
for _ in $(seq 1 40); do
  if curl -fsS -H "Host: ${HOST}" http://127.0.0.1:18380/ > "${WORK}/http-response.txt" 2>/dev/null; then break; fi
  sleep 0.5
done
[[ "$(<"${WORK}/http-response.txt")" == "ingress-ok" ]]

# A second workload owns /api on the same host; the frontend remains at /.
curl -fsS -b "${WORK}/cookies" "${BASE}/workloads" > "${WORK}/workloads.json"
jq -n '{apiVersion:"score.dev/v1b1",metadata:{name:"api"},containers:{main:{image:"busybox:1.37",command:["/bin/sh","-c"],args:["printf api-ok >/tmp/api; exec httpd -f -p 8080 -h /tmp"]}},service:{ports:{http:{port:8080,targetPort:8080}},publicRoutes:[{path:"/api",port:"http"}]}}' > "${WORK}/api-score.json"
jq -n --slurpfile score "${WORK}/api-score.json" --slurpfile state "${WORK}/workloads.json" '{score:$score[0],version:$state[0].draftVersion}' | curl -fsS -b "${WORK}/cookies" -X PUT -H 'Content-Type: application/json' --data-binary @- "${BASE}/workloads/api" > "${WORK}/api-draft.json"
curl -fsS -b "${WORK}/cookies" -X POST -H 'Content-Type: application/json' --data '{}' "${BASE}/preview" > "${WORK}/api-preview.json"
jq '{token:.token}' "${WORK}/api-preview.json" | curl -fsS -b "${WORK}/cookies" -X POST -H 'Content-Type: application/json' --data-binary @- "${BASE}/deploy" > "${WORK}/api-deploy.json"
jq -e '.status == "SUCCEEDED"' "${WORK}/api-deploy.json" >/dev/null
kubectl --context "${CONTEXT}" -n "${NAMESPACE}" get ingress orch-public -o json > "${WORK}/ingress-two-paths.json"
jq -e '[.spec.rules[0].http.paths[] | [.path,.backend.service.name]] | sort == [["/","probe"],["/api","api"]]' "${WORK}/ingress-two-paths.json" >/dev/null
for _ in $(seq 1 40); do
  if curl -fsS -H "Host: ${HOST}" http://127.0.0.1:18380/api > "${WORK}/api-http-response.txt" 2>/dev/null; then break; fi
  sleep 0.5
done
[[ "$(<"${WORK}/api-http-response.txt")" == "api-ok" ]]

curl -fsS -b "${WORK}/cookies" "${BASE}/workloads" > "${WORK}/workloads.json"
jq '{score:(.workloads[] | select(.id == "probe") | .score),version:.draftVersion}' "${WORK}/workloads.json" | curl -fsS -b "${WORK}/cookies" -X PUT -H 'Content-Type: application/json' --data-binary @- "${BASE}/workloads/probe" > "${WORK}/noop-draft.json"
kubectl --context "${CONTEXT}" -n "${NAMESPACE}" get pods -o jsonpath='{.items[0].metadata.uid}' > "${WORK}/pod-uid-before"
curl -fsS -b "${WORK}/cookies" -X POST -H 'Content-Type: application/json' --data '{}' "${BASE}/preview" > "${WORK}/noop-preview.json"
jq -e '(.changes | length) == 0' "${WORK}/noop-preview.json" >/dev/null
[[ "$(<"${WORK}/pod-uid-before")" == "$(kubectl --context "${CONTEXT}" -n "${NAMESPACE}" get pods -o jsonpath='{.items[0].metadata.uid}')" ]]
curl -fsS -b "${WORK}/cookies" "${BASE}/workloads" > "${WORK}/workloads.json"
jq '{version:.draftVersion}' "${WORK}/workloads.json" | curl -fsS -b "${WORK}/cookies" -X DELETE -H 'Content-Type: application/json' --data-binary @- "${BASE}/workloads/probe" > "${WORK}/delete-draft.json"
curl -fsS -b "${WORK}/cookies" -X POST -H 'Content-Type: application/json' --data '{}' "${BASE}/preview" > "${WORK}/remove-preview.json"
jq '{token:.token}' "${WORK}/remove-preview.json" | curl -fsS -b "${WORK}/cookies" -X POST -H 'Content-Type: application/json' --data-binary @- "${BASE}/deploy" > "${WORK}/remove-deploy.json"
jq -e '.status == "SUCCEEDED"' "${WORK}/remove-deploy.json" >/dev/null
kubectl --context "${CONTEXT}" -n "${NAMESPACE}" get ingress orch-public -o json | jq -e '[.spec.rules[0].http.paths[] | [.path,.backend.service.name]] == [["/api","api"]]' >/dev/null
curl -fsS -b "${WORK}/cookies" "${BASE}/workloads" > "${WORK}/workloads.json"
jq '{version:.draftVersion}' "${WORK}/workloads.json" | curl -fsS -b "${WORK}/cookies" -X DELETE -H 'Content-Type: application/json' --data-binary @- "${BASE}/workloads/api" > "${WORK}/delete-api-draft.json"
curl -fsS -b "${WORK}/cookies" -X POST -H 'Content-Type: application/json' --data '{}' "${BASE}/preview" > "${WORK}/remove-api-preview.json"
jq '{token:.token}' "${WORK}/remove-api-preview.json" | curl -fsS -b "${WORK}/cookies" -X POST -H 'Content-Type: application/json' --data-binary @- "${BASE}/deploy" > "${WORK}/remove-api-deploy.json"
jq -e '.status == "SUCCEEDED"' "${WORK}/remove-api-deploy.json" >/dev/null
if kubectl --context "${CONTEXT}" -n "${NAMESPACE}" get ingress orch-public >/dev/null 2>&1; then echo "public route was not removed" >&2; exit 1; fi
echo "Public Ingress kind verification passed"
