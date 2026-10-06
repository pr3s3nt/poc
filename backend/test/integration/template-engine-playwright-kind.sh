#!/usr/bin/env bash
# Browser-driven verification that the Definition-selected score-k8s renderer
# deploys the diagnostic acceptance app on existing kind (always headed and
# recorded). Requires score-k8s 0.15.0 on PATH or SCORE_K8S_BIN.
#
# Records a headed Chromium window (tabs and address bar included)
# on a private Xvfb display with ffmpeg: template-engine-review.mp4, marks.json,
# ffprobe.json and frames/*.png stay in the printed evidence directory, also
# on failure. Requires Xvfb, ffmpeg, ffprobe and xdotool (see video-lib.sh).
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
REPO="$(cd "${ROOT}/.." && pwd)"
WORK="$(mktemp -d)"
RUN_ID="rendered-$(date -u +%Y%m%d%H%M%S)-$RANDOM"
CONTEXT="kind-idp-internal"
TOKEN_FILE="${VAULT_BACKEND_TOKEN_FILE:-/home/thanhnt1/.local/share/poc-vault/vault-uc12-backend-token}"
SCREEN="${ORCH_VIDEO_SCREEN:-1440x900}"
PIDS=()
PLAYWRIGHT_SCRIPT="template-engine-kind-human.mjs"
HUMAN=1
[[ $# -eq 0 ]] || { echo "usage: $0" >&2; exit 2; }
# shellcheck source=video-lib.sh
source "${ROOT}/test/integration/video-lib.sh"

# namespace_lookup <name>: prints namespace/<name> when it exists, nothing
# when the API server confirms it is absent; fails on any lookup error.
namespace_lookup() {
  kubectl --context "${CONTEXT}" get namespace "$1" --ignore-not-found -o name
}

cleanup() {
  local status=$?
  local pid
  for pid in "${PIDS[@]}"; do kill "${pid}" 2>/dev/null || true; done
  for pid in "${PIDS[@]}"; do wait "${pid}" 2>/dev/null || true; done
  # Only a namespace carrying this run's Application label is deleted.
  # Absence is affirmed only by a successful lookup that returns nothing; a
  # failed lookup (API or connection error) fails cleanup instead.
  if [[ -s "${WORK}/namespace" ]]; then
    local namespace app_id actual present
    namespace="$(<"${WORK}/namespace")"
    if [[ "${namespace}" =~ ^app-([a-z0-9-]+)-staging$ ]]; then
      app_id="${BASH_REMATCH[1]}"
      if ! present="$(namespace_lookup "${namespace}")"; then
        echo "cleanup: namespace ${namespace} lookup failed" >&2
        status=1
      elif [[ -n "${present}" ]]; then
        if ! actual="$(kubectl --context "${CONTEXT}" get namespace "${namespace}" -o jsonpath='{.metadata.labels.orchestrator\.io/application}')"; then
          echo "cleanup: namespace ${namespace} label lookup failed" >&2
          status=1
        elif [[ "${actual}" == "${app_id}" ]]; then
          kubectl --context "${CONTEXT}" delete namespace "${namespace}" --ignore-not-found --wait=true --timeout=300s >/dev/null || status=1
        else
          echo "namespace ${namespace} retained: application label mismatch" >&2
          status=1
        fi
      fi
      if ! present="$(namespace_lookup "${namespace}")"; then
        echo "cleanup: namespace ${namespace} absence not verified: lookup failed" >&2
        status=1
      elif [[ -n "${present}" ]]; then
        echo "cleanup: namespace ${namespace} still exists" >&2
        status=1
      else
        echo "cleanup: namespace ${namespace} absent" | tee "${WORK}/cleanup.txt"
      fi
    fi
  fi
  rm -f "${WORK}/state.json"
  # Run-tagged local images (docker and kind node cache) are this run's own.
  docker rmi "acceptance-frontend:${RUN_ID}" "acceptance-backend:${RUN_ID}" "acceptance-worker:${RUN_ID}" >/dev/null 2>&1 || true
  docker exec idp-internal-control-plane crictl rmi "docker.io/library/acceptance-frontend:${RUN_ID}" "docker.io/library/acceptance-backend:${RUN_ID}" >/dev/null 2>&1 || true
  if [[ -s "${WORK}/template-engine-review.mp4" ]]; then echo "video=${WORK}/template-engine-review.mp4"; fi
  echo "run-id=${RUN_ID} evidence=${WORK} status=${status}"
  exit "${status}"
}
trap cleanup EXIT
# A signal still runs cleanup and reports a nonzero status: 128 + signal.
trap 'exit 129' HUP
trap 'exit 130' INT
trap 'exit 143' TERM

[[ "$(kubectl config current-context)" == "${CONTEXT}" ]]
[[ -s "${TOKEN_FILE}" ]]
SCORE_K8S="$(readlink -f "${SCORE_K8S_BIN:-$(command -v score-k8s)}")"
[[ -x "${SCORE_K8S}" ]]
"${SCORE_K8S}" --version | head -n 1 | tee "${WORK}/score-k8s-version.txt" | grep -q '^score-k8s 0\.15\.0 '
sha256sum "${SCORE_K8S}" | tee "${WORK}/score-k8s-sha256.txt" >/dev/null
# The orchestrator is configured with this logging wrapper, which records each
# CLI invocation (argv only) and then execs the real pinned binary unchanged.
SCORE_K8S_REAL="${SCORE_K8S}"
SCORE_K8S_LOG="${WORK}/score-k8s-invocations.log"
mkdir -p "${WORK}/score-k8s-wrapper"
: > "${SCORE_K8S_LOG}"
cat > "${WORK}/score-k8s-wrapper/score-k8s" <<EOF
#!/bin/bash
printf '%s\t%s\n' "\$(/bin/date -u +%FT%TZ)" "\$*" >> "${SCORE_K8S_LOG}"
exec "${SCORE_K8S_REAL}" "\$@"
EOF
chmod 0755 "${WORK}/score-k8s-wrapper/score-k8s"
SCORE_K8S="${WORK}/score-k8s-wrapper/score-k8s"
[[ -d "${REPO}/frontend/dist" ]]
[[ -d "${REPO}/frontend/node_modules/@playwright/test" ]]
if [[ -n "${HUMAN}" ]]; then
  video_require_tools
  video_require_fresh_dist "${REPO}"
  XDOTOOL="$(video_xdotool)"
fi
kubectl --context "${CONTEXT}" -n vault wait --for=condition=Ready pod/vault-uc12-0 --timeout=60s >/dev/null
kubectl --context "${CONTEXT}" -n vault-secrets-operator-system rollout status deployment/vault-secrets-operator-controller-manager --timeout=60s >/dev/null
# The port-forward picks a free local port; it is read from this run's own
# log while the port-forward PID is alive. Other port owners are never touched.
kubectl --context "${CONTEXT}" -n vault port-forward svc/vault-uc12 :8200 > "${WORK}/vault-port-forward.log" 2>&1 &
VAULT_PF_PID=$!
PIDS+=("${VAULT_PF_PID}")
VAULT_PORT=""
for _ in $(seq 1 40); do
  kill -0 "${VAULT_PF_PID}" 2>/dev/null || { echo "Vault port-forward exited; see ${WORK}/vault-port-forward.log" >&2; exit 1; }
  VAULT_PORT="$(sed -n 's/^Forwarding from 127\.0\.0\.1:\([0-9][0-9]*\) -> 8200$/\1/p' "${WORK}/vault-port-forward.log" | head -n 1)"
  [[ -n "${VAULT_PORT}" ]] && break
  sleep 0.5
done
[[ -n "${VAULT_PORT}" ]] || { echo "Vault port-forward did not report a local port" >&2; exit 1; }
VAULT_ADDR_LOCAL="http://127.0.0.1:${VAULT_PORT}"
for _ in $(seq 1 40); do
  kill -0 "${VAULT_PF_PID}" 2>/dev/null || { echo "Vault port-forward exited; see ${WORK}/vault-port-forward.log" >&2; exit 1; }
  if curl -fsS "${VAULT_ADDR_LOCAL}/v1/sys/seal-status" > "${WORK}/seal-status.json" 2>/dev/null; then break; fi
  sleep 0.5
done
[[ "$(jq -r .sealed "${WORK}/seal-status.json")" == "false" ]]

cd "${ROOT}"
"${ROOT}/test/integration/build-images.sh" "${RUN_ID}" "${WORK}/bin"
for workload in frontend backend; do
  kind load docker-image "acceptance-${workload}:${RUN_ID}" --name idp-internal
done
go build -o "${WORK}/orchestrator" ./cmd/orchestrator
kill -0 "${VAULT_PF_PID}" 2>/dev/null || { echo "Vault port-forward exited; see ${WORK}/vault-port-forward.log" >&2; exit 1; }
# Temporary JSON state only: inherited database/Vault/Terraform/AWS defaults
# are dropped and the database URL file is explicitly blank. The scoped Vault
# token is passed by path; its value is never read or printed here.
env -u ORCHESTRATOR_DATABASE_URL_FILE -u ORCHESTRATOR_VAULT_ADDR -u ORCHESTRATOR_VAULT_TOKEN_FILE \
  -u ORCHESTRATOR_VAULT_AGENT_ADDR -u TF_PLUGIN_CACHE_DIR -u AWS_PROFILE -u AWS_REGION -u AWS_DEFAULT_REGION \
  -u AWS_ACCESS_KEY_ID -u AWS_SECRET_ACCESS_KEY -u AWS_SESSION_TOKEN \
  "${WORK}/orchestrator" -addr 127.0.0.1:0 -addr-file "${WORK}/api-addr" -state "${WORK}/state.json" -database-url-file "" \
  -adapters kubernetes -score-k8s "${SCORE_K8S}" -kube-context "${CONTEXT}" -cluster idp-internal -run-id "${RUN_ID}" \
  -vault-address "${VAULT_ADDR_LOCAL}" -vault-token-file "${TOKEN_FILE}" \
  -vault-agent-address http://vault-uc12.vault.svc:8200 -vault-delivery vso \
  -ui-dir "${REPO}/frontend/dist" > "${WORK}/orchestrator.log" 2>&1 &
PIDS+=($!)
for _ in $(seq 1 80); do [[ -s "${WORK}/api-addr" ]] && break; sleep 0.5; done
[[ -s "${WORK}/api-addr" ]]
API="http://$(<"${WORK}/api-addr")"
for _ in $(seq 1 40); do curl -fsS "${API}/api/v1/healthz" >/dev/null 2>&1 && break; sleep 0.5; done
curl -fsS "${API}/api/v1/healthz" >/dev/null

video_start_xvfb "${SCREEN}"
DISPLAY="${VIDEO_DISPLAY}" ORCH_E2E_SCREEN="${SCREEN}" ORCH_E2E_XDOTOOL="${XDOTOOL}" \
  ORCH_E2E_URL="${API}" ORCH_E2E_RUN_ID="${RUN_ID}" ORCH_E2E_KUBE_CONTEXT="${CONTEXT}" \
  ORCH_E2E_NAMESPACE_FILE="${WORK}/namespace" ORCH_E2E_SCORE_K8S_LOG="${SCORE_K8S_LOG}" ORCH_E2E_EVIDENCE_DIR="${WORK}" \
  node "${REPO}/frontend/test/e2e/${PLAYWRIGHT_SCRIPT}" 2>&1 | tee "${WORK}/playwright.log"
video_validate "${WORK}/template-engine-review.mp4" "${SCREEN}" "${ORCH_VIDEO_MIN_SECONDS:-240}" 10 app-reviewed
