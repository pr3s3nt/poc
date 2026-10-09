#!/usr/bin/env bash
# Registers the new kind cluster (default k8s-4f) in the persistent Docker Web
# Console and deploys the retained diagnostic acceptance app through the UI,
# recorded as a headed full-window MP4 with Vietnamese captions below the
# browser window. Nothing is deleted: the cluster, Connection, Secret Store,
# Application, namespace and frontend port-forward are retained for the user.
# Cleanup only stops this run's Xvfb. Requires the Compose stack with the kind
# overlay, deploy/local/workload-vault/start.sh <cluster> already run, images
# acceptance-{backend,frontend}:<RUN_ID> loaded into the cluster and a
# single-context kubeconfig whose endpoint the backend container can reach.
#   k8s4f-dev-playwright.sh <evidence-dir>
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
REPO="$(cd "${ROOT}/.." && pwd)"
WORK="${1:?usage: $0 <evidence-dir>}"
mkdir -p "${WORK}"
CLUSTER="${K8S4F_CLUSTER:-k8s-4f}"
CONTEXT="kind-${CLUSTER}"
RUN_ID="${K8S4F_RUN_ID:-k8s4f-dev}"
STATE="${WORKLOAD_VAULT_STATE_DIR:-${HOME}/.local/share/poc-${CLUSTER}-vault}"
KUBECONFIG_FILE="${K8S4F_KUBECONFIG_FILE:-${STATE}/${CLUSTER}-new.kubeconfig}"
VAULT_CONTAINER="${WORKLOAD_VAULT_CONTAINER:-${CLUSTER}-workload-vault}"
CONSOLE="${ORCH_CONSOLE_URL:-http://127.0.0.1:3001}"
SCREEN="${ORCH_VIDEO_SCREEN:-1440x900}"
CAPTION_STRIP=96
PIDS=()
# shellcheck source=video-lib.sh
source "${ROOT}/test/integration/video-lib.sh"

cleanup() {
  local status=$? pid
  for pid in "${PIDS[@]}"; do kill "${pid}" 2>/dev/null || true; done
  for pid in "${PIDS[@]}"; do wait "${pid}" 2>/dev/null || true; done
  echo "run-id=${RUN_ID} evidence=${WORK} status=${status}"
  exit "${status}"
}
trap cleanup EXIT
trap 'exit 129' HUP
trap 'exit 130' INT
trap 'exit 143' TERM

[[ -s "${KUBECONFIG_FILE}" && -s "${STATE}/token" ]]
curl -fsS "${CONSOLE}/api/v1/healthz" >/dev/null
[[ "$(docker inspect -f '{{.State.Health.Status}}' "${VAULT_CONTAINER}")" == healthy ]]
kubectl --context "${CONTEXT}" -n vault-secrets-operator-system rollout status deployment/vault-secrets-operator-controller-manager --timeout=60s >/dev/null
for workload in backend frontend; do
  docker exec "${CLUSTER}-control-plane" crictl images 2>/dev/null | grep -q "acceptance-${workload} *${RUN_ID}"
done
FORWARD_PORT="${K8S4F_FORWARD_PORT:-18480}"
if ss -ltn | grep -q ":${FORWARD_PORT} "; then echo "local port ${FORWARD_PORT} is busy" >&2; exit 1; fi
video_require_tools
[[ -d "${REPO}/frontend/node_modules/@playwright/test" ]]
XDOTOOL="${ORCH_VIDEO_XDOTOOL:-$(command -v xdotool || true)}"
[[ -n "${XDOTOOL}" ]] || XDOTOOL="$(video_xdotool)"

video_start_xvfb "${SCREEN}"
rm -f "${WORK}/assertions.txt" "${WORK}/result.txt"
set +e
DISPLAY="${VIDEO_DISPLAY}" ORCH_E2E_SCREEN="${SCREEN}" ORCH_E2E_XDOTOOL="${XDOTOOL}" \
  ORCH_E2E_URL="${CONSOLE}" ORCH_E2E_RUN_ID="${RUN_ID}" ORCH_E2E_KUBE_CONTEXT="${CONTEXT}" \
  ORCH_E2E_EVIDENCE_DIR="${WORK}" ORCH_E2E_KUBECONFIG_FILE="${KUBECONFIG_FILE}" \
  ORCH_E2E_STORE_TOKEN_FILE="${STATE}/token" \
  ORCH_E2E_STORE_BACKEND="http://${VAULT_CONTAINER}:8200" ORCH_E2E_STORE_WORKLOAD="http://${VAULT_CONTAINER}:8200" \
  ORCH_E2E_FORWARD_PIDFILE="${STATE}/frontend-forward.pid" ORCH_E2E_APPLICATION_NAME="${K8S4F_APPLICATION:-${RUN_ID}}" ORCH_E2E_SUBDOMAIN="${K8S4F_SUBDOMAIN:-${RUN_ID}}" \
  ORCH_E2E_FORWARD_SCRIPT="${ROOT}/test/integration/retained-port-forward.sh" ORCH_E2E_FORWARD_PORT="${FORWARD_PORT}" \
  node "${REPO}/frontend/test/e2e/k8s4f-dev-human.mjs" 2>&1 | tee "${WORK}/playwright.log"
RUN_STATUS=${PIPESTATUS[0]}
set -e

# Captions are burned below the browser window: the canvas grows by a strip.
# Done on success and failure so the retained video is always reviewable.
if [[ -s "${WORK}/k8s4f-dev-raw.mp4" && -s "${WORK}/captions.srt" ]]; then
  W="${SCREEN%x*}" H="${SCREEN#*x}"
  node "${REPO}/frontend/test/e2e/render-captions.mjs" "${WORK}/captions.srt" "${WORK}/captions.ass" "${W}" "$((H + CAPTION_STRIP))"
  ffmpeg -hide_banner -loglevel error -y -i "${WORK}/k8s4f-dev-raw.mp4" \
    -vf "pad=${W}:$((H + CAPTION_STRIP)):0:0:black,ass=${WORK}/captions.ass" \
    -c:v libx264 -preset veryfast -crf 24 -pix_fmt yuv420p -movflags +faststart "${WORK}/k8s4f-dev-captioned.mp4"
fi
# Probe and fully decode whatever was recorded, also after a failed run (the
# run's own exit status is preserved either way).
if [[ -s "${WORK}/k8s4f-dev-raw.mp4" ]]; then
  ffprobe -v error -show_entries format=duration,size:stream=codec_name,width,height,avg_frame_rate,nb_frames -of json "${WORK}/k8s4f-dev-raw.mp4" > "${WORK}/ffprobe.json" || true
  ffmpeg -hide_banner -v error -xerror -i "${WORK}/k8s4f-dev-raw.mp4" -f null - 2> "${WORK}/decode.log" || echo "raw video decode failed" >> "${WORK}/decode.log"
fi
if [[ "${RUN_STATUS}" -eq 0 ]]; then
  video_validate "${WORK}/k8s4f-dev-raw.mp4" "${SCREEN}" "${ORCH_VIDEO_MIN_SECONDS:-150}" 10 final-connections
fi
if [[ -s "${WORK}/k8s4f-dev-captioned.mp4" ]]; then
  ffprobe -v error -show_entries format=duration,size:stream=codec_name,width,height,avg_frame_rate,nb_frames -of json "${WORK}/k8s4f-dev-captioned.mp4" > "${WORK}/captioned-ffprobe.json" || true
  ffmpeg -hide_banner -v error -xerror -i "${WORK}/k8s4f-dev-captioned.mp4" -f null - 2> "${WORK}/captioned-decode.log" || echo "captioned video decode failed" >> "${WORK}/captioned-decode.log"
  if [[ "${RUN_STATUS}" -eq 0 && -s "${WORK}/captioned-decode.log" ]]; then exit 1; fi
fi
exit "${RUN_STATUS}"
