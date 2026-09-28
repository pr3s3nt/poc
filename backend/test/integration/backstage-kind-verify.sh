#!/usr/bin/env bash
# Run-scoped Backstage image -> UC-16 Preview/Deploy -> PostgreSQL -> Ingress check.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
WORK="$(mktemp -d)"
RUN_ID="backstage-$(date -u +%Y%m%d%H%M%S)-$RANDOM"
CONTEXT="kind-idp-internal"
IMAGE="ghcr.io/backstage/backstage:1.53.1"
TOKEN_FILE="${VAULT_BACKEND_TOKEN_FILE:-/home/thanhnt1/.local/share/poc-vault/vault-uc12-backend-token}"
NAMESPACE=""
DEPLOY_RUN_ID=""
API_PID=""
PROXY_PID=""
VAULT_PID=""

cleanup() {
  local status=$?
  [[ -z "${API_PID}" ]] || kill "${API_PID}" 2>/dev/null || true
  [[ -z "${PROXY_PID}" ]] || kill "${PROXY_PID}" 2>/dev/null || true
  [[ -z "${VAULT_PID}" ]] || kill "${VAULT_PID}" 2>/dev/null || true
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
docker exec idp-internal-control-plane ctr --namespace=k8s.io images list --quiet | rg -Fx "${IMAGE}" >/dev/null
[[ -s "${TOKEN_FILE}" ]]
kubectl --context "${CONTEXT}" -n vault wait --for=condition=Ready pod/vault-uc12-0 --timeout=60s >/dev/null
kubectl --context "${CONTEXT}" -n vault-secrets-operator-system rollout status deployment/vault-secrets-operator-controller-manager --timeout=60s >/dev/null
kubectl --context "${CONTEXT}" -n traefik rollout status deployment/traefik --timeout=60s >/dev/null
kubectl --context "${CONTEXT}" -n vault port-forward svc/vault-uc12 18212:8200 > "${WORK}/vault-port-forward.log" 2>&1 &
VAULT_PID=$!
for _ in $(seq 1 40); do
  if curl -fsS http://127.0.0.1:18212/v1/sys/seal-status > "${WORK}/seal-status.json" 2>/dev/null; then break; fi
  sleep 0.5
done
[[ "$(jq -r .sealed "${WORK}/seal-status.json")" == "false" ]]
cd "${ROOT}"
go build -o "${WORK}/orchestrator" ./cmd/orchestrator
"${WORK}/orchestrator" -addr 127.0.0.1:0 -addr-file "${WORK}/api-addr" -state "${WORK}/state.json" \
  -adapters kubernetes -kube-context "${CONTEXT}" -cluster idp-internal -run-id "${RUN_ID}" \
  -vault-address http://127.0.0.1:18212 -vault-token-file "${TOKEN_FILE}" \
  -vault-agent-address http://vault-uc12.vault.svc:8200 -vault-delivery vso \
  -ui-dir "${ROOT}/../frontend/dist" > "${WORK}/orchestrator.log" 2>&1 &
API_PID=$!
for _ in $(seq 1 80); do [[ -s "${WORK}/api-addr" ]] && break; sleep 0.5; done
API="http://$(<"${WORK}/api-addr")/api/v1"
for _ in $(seq 1 40); do curl -fsS "${API}/healthz" >/dev/null 2>&1 && break; sleep 0.5; done
curl -fsS -c "${WORK}/cookies" -H 'Content-Type: application/json' \
  --data '{"username":"developer","password":"test-password"}' "${API}/auth/sign-in" >/dev/null
APP_ID="$(curl -fsS -b "${WORK}/cookies" -H 'Content-Type: application/json' \
  --data "{\"name\":\"Backstage ${RUN_ID}\",\"subdomain\":\"${RUN_ID}\"}" "${API}/applications" | jq -r '.application.key')"
[[ -n "${APP_ID}" && "${APP_ID}" != "null" ]]
NAMESPACE="app-${APP_ID}-staging"
BASE="${API}/applications/${APP_ID}/environments/staging"
HOST="staging.${RUN_ID}.example.com"
PUBLIC_URL="http://${HOST}:18381"

for entry in "APP_CONFIG_app_baseUrl:${PUBLIC_URL}" "APP_CONFIG_backend_baseUrl:${PUBLIC_URL}" "APP_CONFIG_auth_providers_guest_dangerouslyAllowOutsideDevelopment:true"; do
  key="${entry%%:*}"
  value="${entry#*:}"
  version="$(curl -fsS -b "${WORK}/cookies" "${BASE}/configuration" | jq -r '.version')"
  jq -n --arg value "${value}" --argjson version "${version}" '{kind:"VARIABLE",value:$value,version:$version}' |
    curl -fsS -b "${WORK}/cookies" -X PUT -H 'Content-Type: application/json' --data-binary @- "${BASE}/configuration/keys/${key}" > "${WORK}/config-${key}.json"
done

jq -n --arg image "${IMAGE}" '
  {apiVersion:"score.dev/v1b1", metadata:{name:"backstage"},
   containers:{main:{image:$image,
     variables:{
       POSTGRES_HOST:"${resources.db.host}",
       POSTGRES_PORT:"${resources.db.port}",
       POSTGRES_USER:"${resources.db.username}",
       POSTGRES_PASSWORD:"${resources.db.password}",
       APP_CONFIG_app_baseUrl:"${resources.env.APP_CONFIG_app_baseUrl}",
       APP_CONFIG_backend_baseUrl:"${resources.env.APP_CONFIG_backend_baseUrl}",
       APP_CONFIG_auth_providers_guest_dangerouslyAllowOutsideDevelopment:"${resources.env.APP_CONFIG_auth_providers_guest_dangerouslyAllowOutsideDevelopment}"
     },
     readinessProbe:{path:"/.backstage/health/v1/readiness",port:7007},
     livenessProbe:{path:"/.backstage/health/v1/liveness",port:7007},
     resources:{requests:{cpu:"100m",memory:"256Mi"},limits:{cpu:"1000m",memory:"1Gi"}}
   }},
   service:{ports:{http:{port:7007,targetPort:7007}},publicRoutes:[{path:"/",port:"http"}]},
   resources:{db:{type:"postgres",class:"default",id:"backstage-db",params:{database:"backstage",username:"backstage"}},env:{type:"environment"}}}
' > "${WORK}/score.json"
draft_status="$(jq '{score:.,version:0}' "${WORK}/score.json" |
  curl -sS -b "${WORK}/cookies" -X PUT -H 'Content-Type: application/json' --data-binary @- -o "${WORK}/draft.json" -w '%{http_code}' "${BASE}/workloads/backstage")"
if [[ "${draft_status}" != "200" ]]; then jq -r '.error' "${WORK}/draft.json" >&2; exit 1; fi
preview_status="$(curl -sS -b "${WORK}/cookies" -X POST -H 'Content-Type: application/json' --data '{}' -o "${WORK}/preview.json" -w '%{http_code}' "${BASE}/preview")"
if [[ "${preview_status}" != "200" ]]; then jq -r '.error' "${WORK}/preview.json" >&2; exit 1; fi
jq -e '(.changes | length) == 1 and .changes[0].action == "DEPLOY"' "${WORK}/preview.json" >/dev/null
DEPLOY_RUN_ID="$(jq -r .runId "${WORK}/preview.json")"
jq '{token:.token}' "${WORK}/preview.json" |
  curl -fsS -b "${WORK}/cookies" -X POST -H 'Content-Type: application/json' --data-binary @- "${BASE}/deploy" > "${WORK}/deploy.json"
jq -e '.status == "SUCCEEDED"' "${WORK}/deploy.json" >/dev/null
kubectl --context "${CONTEXT}" -n "${NAMESPACE}" rollout status deployment/backstage --timeout=300s >/dev/null
kubectl --context "${CONTEXT}" -n "${NAMESPACE}" get ingress orch-public -o json > "${WORK}/ingress.json"
jq -e --arg host "${HOST}" '.spec.rules[0].host == $host and .spec.rules[0].http.paths[0].backend.service.name == "backstage"' "${WORK}/ingress.json" >/dev/null

kubectl --context "${CONTEXT}" -n traefik port-forward svc/traefik 18381:80 > "${WORK}/traefik-port-forward.log" 2>&1 &
PROXY_PID=$!
for _ in $(seq 1 60); do
  if curl -fsS -H "Host: ${HOST}" http://127.0.0.1:18381/ > "${WORK}/index.html" 2>/dev/null; then break; fi
  sleep 0.5
done
grep -qi 'backstage' "${WORK}/index.html"
curl -fsS -H "Host: ${HOST}" "http://127.0.0.1:18381/.backstage/health/v1/readiness" > "${WORK}/readiness.json"
curl -fsS -H "Host: ${HOST}" "http://127.0.0.1:18381/api/auth/guest/refresh" |
  jq -e '.backstageIdentity.token | type == "string" and length > 0' >/dev/null
DEPLOYMENT_ID="$(jq -r '.results[0].deploymentId' "${WORK}/deploy.json")"
curl -fsS -b "${WORK}/cookies" "${API}/deployments/${DEPLOYMENT_ID}" > "${WORK}/deployment-view.json"
jq -e '.deployment.status == "SUCCEEDED" and (.resources | any(.resourceType == "postgres"))' "${WORK}/deployment-view.json" >/dev/null
echo "Backstage kind verification passed: ${PUBLIC_URL}"
