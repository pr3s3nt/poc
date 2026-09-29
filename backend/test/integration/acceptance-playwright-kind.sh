#!/usr/bin/env bash
# Browser-driven deployment of the diagnostic acceptance app on existing kind.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
WORK="$(mktemp -d)"
RUN_ID="acceptance-$(date -u +%Y%m%d%H%M%S)-$RANDOM"
CONTEXT="kind-idp-internal"
TOKEN_FILE="${VAULT_BACKEND_TOKEN_FILE:-/home/thanhnt1/.local/share/poc-vault/vault-uc12-backend-token}"
API_PID=""
VAULT_PID=""

cleanup() {
  local status=$?
  [[ -z "${API_PID}" ]] || kill "${API_PID}" 2>/dev/null || true
  [[ -z "${VAULT_PID}" ]] || kill "${VAULT_PID}" 2>/dev/null || true
  if [[ -s "${WORK}/namespace" ]]; then
    local namespace app_id actual
    namespace="$(<"${WORK}/namespace")"
    if [[ "${namespace}" =~ ^app-([a-z0-9-]+)-staging$ ]]; then
      app_id="${BASH_REMATCH[1]}"
      actual="$(kubectl --context "${CONTEXT}" get namespace "${namespace}" -o jsonpath='{.metadata.labels.orchestrator\.io/application}' 2>/dev/null || true)"
      if [[ "${actual}" == "${app_id}" ]]; then
        kubectl --context "${CONTEXT}" delete namespace "${namespace}" --wait=true >/dev/null || status=1
      elif kubectl --context "${CONTEXT}" get namespace "${namespace}" >/dev/null 2>&1; then
        echo "namespace ${namespace} retained: application label mismatch" >&2
        status=1
      fi
    fi
  fi
  if [[ -s "${WORK}/acceptance-full.webm" ]]; then
    echo "video=${WORK}/acceptance-full.webm"
  fi
  echo "run-id=${RUN_ID} evidence=${WORK} status=${status}"
  exit "${status}"
}
trap cleanup EXIT

[[ "$(kubectl config current-context)" == "${CONTEXT}" ]]
[[ -s "${TOKEN_FILE}" ]]
[[ -d "${ROOT}/../frontend/dist" ]]
[[ -d "${ROOT}/../frontend/node_modules/@playwright/test" ]]
kubectl --context "${CONTEXT}" -n vault wait --for=condition=Ready pod/vault-uc12-0 --timeout=60s >/dev/null
kubectl --context "${CONTEXT}" -n vault-secrets-operator-system rollout status deployment/vault-secrets-operator-controller-manager --timeout=60s >/dev/null
kubectl --context "${CONTEXT}" -n vault port-forward svc/vault-uc12 18213:8200 > "${WORK}/vault-port-forward.log" 2>&1 &
VAULT_PID=$!
for _ in $(seq 1 40); do
  if curl -fsS http://127.0.0.1:18213/v1/sys/seal-status > "${WORK}/seal-status.json" 2>/dev/null; then break; fi
  sleep 0.5
done
[[ "$(jq -r .sealed "${WORK}/seal-status.json")" == "false" ]]

cd "${ROOT}"
"${ROOT}/test/integration/build-images.sh" "${RUN_ID}" "${WORK}/bin"
for workload in frontend backend; do
  kind load docker-image "acceptance-${workload}:${RUN_ID}" --name idp-internal
done
go build -o "${WORK}/orchestrator" ./cmd/orchestrator
"${WORK}/orchestrator" -addr 127.0.0.1:0 -addr-file "${WORK}/api-addr" -state "${WORK}/state.json" \
  -adapters kubernetes -kube-context "${CONTEXT}" -cluster idp-internal -run-id "${RUN_ID}" \
  -vault-address http://127.0.0.1:18213 -vault-token-file "${TOKEN_FILE}" \
  -vault-agent-address http://vault-uc12.vault.svc:8200 -vault-delivery vso \
  -ui-dir "${ROOT}/../frontend/dist" > "${WORK}/orchestrator.log" 2>&1 &
API_PID=$!
for _ in $(seq 1 80); do [[ -s "${WORK}/api-addr" ]] && break; sleep 0.5; done
[[ -s "${WORK}/api-addr" ]]
API="http://$(<"${WORK}/api-addr")"
for _ in $(seq 1 40); do curl -fsS "${API}/api/v1/healthz" >/dev/null 2>&1 && break; sleep 0.5; done
curl -fsS "${API}/api/v1/healthz" >/dev/null

ORCH_E2E_URL="${API}" ORCH_E2E_RUN_ID="${RUN_ID}" ORCH_E2E_KUBE_CONTEXT="${CONTEXT}" \
  ORCH_E2E_NAMESPACE_FILE="${WORK}/namespace" ORCH_E2E_EVIDENCE_DIR="${WORK}" \
  node "${ROOT}/../frontend/test/e2e/acceptance-kind.mjs" 2>&1 | tee "${WORK}/playwright.log"
