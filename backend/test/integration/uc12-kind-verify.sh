#!/usr/bin/env bash
# UC-12/16 desired configuration, Preview -> Deploy, and Vault Agent on kind.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
WORK="$(mktemp -d)"
RUN_ID="uc12-$(date -u +%Y%m%d%H%M%S)-$RANDOM"
CONTEXT="kind-idp-internal"
TOKEN_FILE="${VAULT_BACKEND_TOKEN_FILE:-/home/thanhnt1/.local/share/poc-vault/vault-uc12-backend-token}"
NAMESPACE=""
DEPLOY_RUN_ID=""
API_PID=""
VAULT_PID=""

cleanup() {
  local status=$?
  [[ -z "${API_PID}" ]] || kill "${API_PID}" 2>/dev/null || true
  [[ -z "${VAULT_PID}" ]] || kill "${VAULT_PID}" 2>/dev/null || true
  if [[ -n "${NAMESPACE}" ]] && kubectl --context "${CONTEXT}" get namespace "${NAMESPACE}" >/dev/null 2>&1; then
    local actual
    actual="$(kubectl --context "${CONTEXT}" get namespace "${NAMESPACE}" -o jsonpath='{.metadata.labels.orchestrator\.io/run-id}' 2>/dev/null || true)"
    if [[ -n "${DEPLOY_RUN_ID}" && "${actual}" == "${DEPLOY_RUN_ID}" ]]; then
      kubectl --context "${CONTEXT}" delete namespace "${NAMESPACE}" --wait=true >/dev/null
    else
      echo "namespace ${NAMESPACE} was not deleted: run-id label did not match" >&2
      status=1
    fi
  fi
  if [[ -n "${DEPLOY_RUN_ID}" ]]; then
    kubectl --context "${CONTEXT}" get all,pvc,secret,namespace -A -l "orchestrator.io/run-id=${DEPLOY_RUN_ID}" > "${WORK}/cleanup-check.txt" || true
  fi
  echo "run-id=${RUN_ID} evidence=${WORK} status=${status}"
  exit "${status}"
}
trap cleanup EXIT

[[ "$(kubectl config current-context)" == "${CONTEXT}" ]]
[[ -s "${TOKEN_FILE}" ]]
kubectl --context "${CONTEXT}" -n vault wait --for=condition=Ready pod/vault-uc12-0 --timeout=60s >/dev/null
kubectl --context "${CONTEXT}" -n vault port-forward svc/vault-uc12 18211:8200 > "${WORK}/vault-port-forward.log" 2>&1 &
VAULT_PID=$!
for _ in $(seq 1 40); do
  curl -fsS http://127.0.0.1:18211/v1/sys/seal-status > "${WORK}/seal-status.json" && break
  sleep 0.5
done
[[ "$(jq -r .sealed "${WORK}/seal-status.json")" == "false" ]]

cd "${ROOT}"
go build -o "${WORK}/orchestrator" ./cmd/orchestrator
"${WORK}/orchestrator" -addr 127.0.0.1:0 -addr-file "${WORK}/api-addr" -state "${WORK}/state.json" \
  -adapters kubernetes -kube-context "${CONTEXT}" -cluster idp-internal -run-id "${RUN_ID}" \
  -vault-address http://127.0.0.1:18211 -vault-token-file "${TOKEN_FILE}" \
  -vault-agent-address http://vault-uc12.vault.svc:8200 \
  -ui-dir "${ROOT}/../frontend/dist" > "${WORK}/orchestrator.log" 2>&1 &
API_PID=$!
for _ in $(seq 1 80); do
  [[ -s "${WORK}/api-addr" ]] && break
  sleep 0.5
done
API="http://$(<"${WORK}/api-addr")/api/v1"
for _ in $(seq 1 40); do
  curl -fsS "${API}/healthz" >/dev/null && break
  sleep 0.5
done

curl -fsS -c "${WORK}/cookies" -H 'Content-Type: application/json' \
  --data '{"username":"developer","password":"test-password"}' "${API}/auth/sign-in" >/dev/null
APP_ID="$(curl -fsS -b "${WORK}/cookies" -H 'Content-Type: application/json' \
  --data "{\"name\":\"UC12 ${RUN_ID}\",\"subdomain\":\"${RUN_ID}\"}" "${API}/applications" | jq -r '.application.key')"
[[ -n "${APP_ID}" && "${APP_ID}" != "null" ]]
NAMESPACE="app-${APP_ID}-staging"
BASE="${API}/applications/${APP_ID}/environments/staging"

TEST_VALUE="kind-test-${RUN_ID}"
curl -fsS -b "${WORK}/cookies" -X PUT -H 'Content-Type: application/json' \
  --data "{\"kind\":\"SECRET\",\"value\":\"${TEST_VALUE}\",\"version\":0}" \
  "${BASE}/configuration/keys/API_TOKEN" > "${WORK}/configuration.json"
if rg -q "${TEST_VALUE}" "${WORK}/configuration.json"; then echo "secret leaked in API response" >&2; exit 1; fi

jq -n '{apiVersion:"score.dev/v1b1",metadata:{name:"probe"},containers:{main:{image:"busybox:1.37",command:["/bin/sh","-c"],args:["set -eu; . \"$ORCHESTRATOR_CONFIG_FILE\"; test -n \"$API_TOKEN\"; sleep 600"],variables:{API_TOKEN:"${resources.env.API_TOKEN}"}}},resources:{env:{type:"environment"}}}' \
  | jq '{score:.,version:0}' \
  | curl -fsS -b "${WORK}/cookies" -X PUT -H 'Content-Type: application/json' --data-binary @- "${BASE}/workloads/probe" > "${WORK}/draft.json"

PREVIEW_STATUS="$(curl -sS -w '%{http_code}' -b "${WORK}/cookies" -X POST -H 'Content-Type: application/json' --data '{}' -o "${WORK}/preview.json" "${BASE}/preview")"
if [[ "${PREVIEW_STATUS}" != "200" ]]; then
  jq -r '.error // .message // "preview failed"' "${WORK}/preview.json" >&2
  exit 1
fi
jq -e '.changes | length == 1' "${WORK}/preview.json" >/dev/null
DEPLOY_RUN_ID="$(jq -r .runId "${WORK}/preview.json")"
jq '{token:.token}' "${WORK}/preview.json" | curl -fsS -b "${WORK}/cookies" -X POST -H 'Content-Type: application/json' --data-binary @- "${BASE}/deploy" > "${WORK}/deploy.json"
jq -e '.status == "SUCCEEDED"' "${WORK}/deploy.json" >/dev/null
kubectl --context "${CONTEXT}" -n "${NAMESPACE}" rollout status deployment/probe --timeout=180s
kubectl --context "${CONTEXT}" -n "${NAMESPACE}" get deployment probe -o json > "${WORK}/deployment.json"
jq -e '.spec.template.metadata.annotations["vault.hashicorp.com/agent-inject"] == "true" and .spec.template.spec.serviceAccountName != null' "${WORK}/deployment.json" >/dev/null
if kubectl --context "${CONTEXT}" -n "${NAMESPACE}" get secret probe-env >/dev/null 2>&1; then echo "UC-12 value was copied to Kubernetes Secret" >&2; exit 1; fi
if rg -q "${TEST_VALUE}" "${WORK}/state.json" "${WORK}/deployment.json"; then echo "secret leaked to state or Deployment" >&2; exit 1; fi

curl -fsS -b "${WORK}/cookies" -X PUT -H 'Content-Type: application/json' \
  --data "{\"kind\":\"SECRET\",\"value\":\"rotated-${TEST_VALUE}\",\"version\":1}" \
  "${BASE}/configuration/keys/API_TOKEN" > "${WORK}/rotation.json"
curl -fsS -b "${WORK}/cookies" -X POST -H 'Content-Type: application/json' --data '{}' "${BASE}/preview" > "${WORK}/rotation-preview.json"
jq -e '.changes | length == 1' "${WORK}/rotation-preview.json" >/dev/null
jq '{token:.token}' "${WORK}/rotation-preview.json" | curl -fsS -b "${WORK}/cookies" -X POST -H 'Content-Type: application/json' --data-binary @- "${BASE}/deploy" > "${WORK}/rotation-deploy.json"
jq -e '.status == "SUCCEEDED"' "${WORK}/rotation-deploy.json" >/dev/null
kubectl --context "${CONTEXT}" -n "${NAMESPACE}" rollout status deployment/probe --timeout=180s
curl -fsS -b "${WORK}/cookies" -X POST -H 'Content-Type: application/json' --data '{}' "${BASE}/preview" > "${WORK}/after.json"
jq -e '.changes | length == 0' "${WORK}/after.json" >/dev/null
curl -fsS -b "${WORK}/cookies" "${BASE}/workloads" > "${WORK}/workloads.json"
jq '{version:.draftVersion}' "${WORK}/workloads.json" | curl -fsS -b "${WORK}/cookies" -X DELETE -H 'Content-Type: application/json' --data-binary @- "${BASE}/workloads/probe" > "${WORK}/delete-draft.json"
curl -fsS -b "${WORK}/cookies" -X POST -H 'Content-Type: application/json' --data '{}' "${BASE}/preview" > "${WORK}/remove-preview.json"
jq -e '(.changes | length) == 1 and .changes[0].action == "REMOVE"' "${WORK}/remove-preview.json" >/dev/null
jq '{token:.token}' "${WORK}/remove-preview.json" | curl -fsS -b "${WORK}/cookies" -X POST -H 'Content-Type: application/json' --data-binary @- "${BASE}/deploy" > "${WORK}/remove-deploy.json"
jq -e '.status == "SUCCEEDED"' "${WORK}/remove-deploy.json" >/dev/null
if kubectl --context "${CONTEXT}" -n "${NAMESPACE}" get deployment probe >/dev/null 2>&1; then echo "removed workload still has a Deployment" >&2; exit 1; fi
echo "UC-12/16 kind verification passed"
