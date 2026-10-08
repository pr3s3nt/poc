#!/usr/bin/env bash
# Recorded self-hosting check on the existing kind cluster: a host Orchestrator
# deploys the Orchestrator images through the Web Console, then that in-cluster
# Orchestrator deploys the diagnostic acceptance app. The in-cluster
# Orchestrator is kept running; only the acceptance app namespace is removed.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
REPO="$(cd "${ROOT}/.." && pwd)"
WORK="$(mktemp -d)"
chmod 0700 "${WORK}"
RUN_ID="selfhost-$(date -u +%Y%m%d%H%M%S)-$RANDOM"
CONTEXT="kind-idp-internal"
CLUSTER="idp-internal"
SUBDOMAIN="${ORCH_SELF_SUBDOMAIN:-orchestrator}"
SELF_HOST="staging.${SUBDOMAIN}.example.com"
IN_CLUSTER_VAULT="http://vault-uc12.vault.svc:8200"
TOKEN_FILE="${VAULT_BACKEND_TOKEN_FILE:-/home/thanhnt1/.local/share/poc-vault/vault-uc12-backend-token}"
SCREEN="1280x900"
PIDS=()
# shellcheck source=video-lib.sh
source "${ROOT}/test/integration/video-lib.sh"

delete_owned_namespace() {
  local file="$1" namespace app_id actual
  [[ -s "${file}" ]] || return 0
  namespace="$(<"${file}")"
  [[ "${namespace}" =~ ^app-([a-z0-9-]+)-staging$ ]] || return 1
  app_id="${BASH_REMATCH[1]}"
  actual="$(kubectl --context "${CONTEXT}" get namespace "${namespace}" -o jsonpath='{.metadata.labels.orchestrator\.io/application}' 2>/dev/null || true)"
  if [[ "${actual}" == "${app_id}" ]]; then
    kubectl --context "${CONTEXT}" delete namespace "${namespace}" --wait=true >/dev/null
  elif kubectl --context "${CONTEXT}" get namespace "${namespace}" >/dev/null 2>&1; then
    echo "namespace ${namespace} retained: application label mismatch" >&2
    return 1
  fi
}

cleanup() {
  local status=$?
  for pid in "${PIDS[@]}"; do kill "${pid}" 2>/dev/null || true; done
  delete_owned_namespace "${WORK}/namespace" || status=1
  rm -f "${WORK}/kubeconfig.b64"
  [[ ! -s "${WORK}/self-host.mp4" ]] || echo "video=${WORK}/self-host.mp4"
  if [[ -s "${WORK}/self-namespace" ]]; then
    echo "self-hosted Orchestrator kept in namespace $(<"${WORK}/self-namespace"); open http://${SELF_HOST}/ after:"
    echo "  kubectl --context ${CONTEXT} -n traefik port-forward svc/traefik 18380:80"
    echo "  chromium --host-resolver-rules='MAP *.example.com 127.0.0.1:18380'"
  fi
  echo "run-id=${RUN_ID} evidence=${WORK} status=${status}"
  exit "${status}"
}
trap cleanup EXIT

[[ "$(kubectl config current-context)" == "${CONTEXT}" ]]
[[ -s "${TOKEN_FILE}" ]]
[[ -d "${REPO}/frontend/dist" ]]
[[ -d "${REPO}/frontend/node_modules/@playwright/test" ]]
command -v Xvfb >/dev/null
command -v ffmpeg >/dev/null
if kubectl --context "${CONTEXT}" get ingress -A -o json | jq -e --arg host "${SELF_HOST}" 'any(.items[].spec.rules[]?; .host == $host)' >/dev/null; then
  echo "${SELF_HOST} is already routed; delete the previous self-hosted namespace or set ORCH_SELF_SUBDOMAIN" >&2
  exit 1
fi
kubectl --context "${CONTEXT}" -n vault wait --for=condition=Ready pod/vault-uc12-0 --timeout=60s >/dev/null
kubectl --context "${CONTEXT}" -n vault-secrets-operator-system rollout status deployment/vault-secrets-operator-controller-manager --timeout=60s >/dev/null
kubectl --context "${CONTEXT}" -n traefik rollout status deployment/traefik --timeout=60s >/dev/null

kubectl --context "${CONTEXT}" -n vault port-forward svc/vault-uc12 18213:8200 > "${WORK}/vault-port-forward.log" 2>&1 &
PIDS+=($!)
for _ in $(seq 1 40); do
  if curl -fsS http://127.0.0.1:18213/v1/sys/seal-status > "${WORK}/seal-status.json" 2>/dev/null; then break; fi
  sleep 0.5
done
[[ "$(jq -r .sealed "${WORK}/seal-status.json")" == "false" ]]

kubectl --context "${CONTEXT}" -n traefik port-forward svc/traefik :80 > "${WORK}/traefik-port-forward.log" 2>&1 &
PIDS+=($!)
for _ in $(seq 1 40); do grep -q 'Forwarding from 127.0.0.1:' "${WORK}/traefik-port-forward.log" && break; sleep 0.5; done
TRAEFIK_PORT="$(sed -nE 's/^Forwarding from 127\.0\.0\.1:([0-9]+) .*/\1/p' "${WORK}/traefik-port-forward.log" | head -1)"
[[ -n "${TRAEFIK_PORT}" ]]

# The in-cluster Orchestrator reaches the API server through the cluster
# Service, keeping the host context name the seeded cluster refers to.
(umask 077 && kubectl config view --minify --flatten --context "${CONTEXT}" \
  | sed -E 's#^([[:space:]]*server:).*#\1 https://kubernetes.default.svc#' \
  | base64 -w0 > "${WORK}/kubeconfig.b64")

cd "${ROOT}"
echo "building images with tag ${RUN_ID}"
"${ROOT}/test/integration/build-images.sh" "${RUN_ID}" "${WORK}/bin"
docker build -q -t "orchestrator-backend:${RUN_ID}" "${ROOT}" > "${WORK}/docker-build.log"
docker build -q -t "orchestrator-frontend:${RUN_ID}" "${REPO}/frontend" >> "${WORK}/docker-build.log"
for image in "acceptance-frontend" "acceptance-backend" "orchestrator-backend" "orchestrator-frontend"; do
  kind load docker-image "${image}:${RUN_ID}" --name "${CLUSTER}"
done

go build -o "${WORK}/orchestrator" ./cmd/orchestrator
"${WORK}/orchestrator" -addr 127.0.0.1:0 -addr-file "${WORK}/api-addr" -state "${WORK}/state.json" \
  -adapters kubernetes -kube-context "${CONTEXT}" -cluster "${CLUSTER}" -run-id "${RUN_ID}" \
  -vault-address http://127.0.0.1:18213 -vault-token-file "${TOKEN_FILE}" \
  -vault-agent-address "${IN_CLUSTER_VAULT}" -vault-delivery vso \
  -ui-dir "${REPO}/frontend/dist" > "${WORK}/orchestrator.log" 2>&1 &
PIDS+=($!)
for _ in $(seq 1 80); do [[ -s "${WORK}/api-addr" ]] && break; sleep 0.5; done
[[ -s "${WORK}/api-addr" ]]
API="http://$(<"${WORK}/api-addr")"
for _ in $(seq 1 40); do curl -fsS "${API}/api/v1/healthz" >/dev/null 2>&1 && break; sleep 0.5; done
curl -fsS "${API}/api/v1/healthz" >/dev/null
video_require_secret_store "${API}" platform-vault

# WSLg owns :0, so pick the first free display from :90.
DISPLAY_NUMBER=""
for candidate in $(seq 90 99); do
  if [[ ! -e "/tmp/.X${candidate}-lock" && ! -e "/tmp/.X11-unix/X${candidate}" ]]; then DISPLAY_NUMBER="${candidate}"; break; fi
done
[[ -n "${DISPLAY_NUMBER}" ]]
Xvfb ":${DISPLAY_NUMBER}" -screen 0 "${SCREEN}x24" -nolisten tcp 2> "${WORK}/xvfb.log" &
PIDS+=($!)
# WSLg mounts /tmp/.X11-unix read-only, so Xvfb may listen only on the
# abstract socket; its lock file marks a started server.
for _ in $(seq 1 20); do [[ -e "/tmp/.X${DISPLAY_NUMBER}-lock" ]] && break; sleep 0.5; done
[[ -e "/tmp/.X${DISPLAY_NUMBER}-lock" ]]
sleep 1
kill -0 "${PIDS[-1]}"

DISPLAY=":${DISPLAY_NUMBER}" ORCH_E2E_SCREEN="${SCREEN}" \
  ORCH_E2E_URL="${API}" ORCH_E2E_RUN_ID="${RUN_ID}" ORCH_E2E_EVIDENCE_DIR="${WORK}" \
  ORCH_E2E_TRAEFIK_PORT="${TRAEFIK_PORT}" ORCH_E2E_SELF_SUBDOMAIN="${SUBDOMAIN}" \
  ORCH_E2E_KUBE_CONTEXT="${CONTEXT}" ORCH_E2E_CLUSTER="${CLUSTER}" ORCH_E2E_VAULT_ADDR="${IN_CLUSTER_VAULT}" \
  ORCH_E2E_KUBECONFIG_B64_FILE="${WORK}/kubeconfig.b64" ORCH_E2E_VAULT_TOKEN_FILE="${TOKEN_FILE}" \
  ORCH_E2E_SELF_NAMESPACE_FILE="${WORK}/self-namespace" ORCH_E2E_NAMESPACE_FILE="${WORK}/namespace" \
  node "${REPO}/frontend/test/e2e/self-host-kind.mjs" 2>&1 | tee "${WORK}/playwright.log"
