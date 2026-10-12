#!/usr/bin/env bash
# Registers the new kind cluster (default k8s-4f) in the persistent Docker Web
# Console and deploys the retained diagnostic acceptance app through the UI,
# recorded as a headed full-window MP4 with Vietnamese captions below the
# browser window. Nothing is deleted: the cluster, Connection, Secret Store,
# Application, namespace and frontend port-forward are retained for the user.
# Cleanup only stops this run's Xvfb. Requires the Compose stack with the kind
# overlay, deploy/local/workload-vault/start.sh <cluster> already run, images
# acceptance-{backend,frontend}:<K8S4F_IMAGE_TAG, default RUN_ID> loaded into the cluster and a
# single-context kubeconfig whose endpoint the backend container can reach.
#   k8s4f-dev-playwright.sh <new-evidence-dir>
# The evidence dir must not exist, must be outside the repository and is created
# owner-only (0700); failed runs keep every recording. Resume (K8S4F_RESUME=1)
# needs the approved run's K8S4F_RESUME_CONNECTION_KEY / _STORE_KEY (and
# _APPLICATION_ID with K8S4F_RESUME_APP=1); it never replays on its own.
# K8S4F_FORBIDDEN_APP_IDS (comma list) aborts if the new Application id matches;
# K8S4F_EXPECT_EMPTY_JOBS=1 asserts the new workload DB has no jobs before the first submit.
# K8S4F_FORWARD_PIDFILE overrides the fresh per-run supervisor pidfile.
# K8S4F_DRY_RUN=1 runs only offline preparation checks (paths, permissions,
# tools, JS parse): no Console, Docker, kubectl or browser access. Private file
# contents are never read here or printed.
set -euo pipefail
umask 077

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
REPO="$(cd "${ROOT}/.." && pwd)"
WORK="${1:?usage: $0 <new-evidence-dir>}"
WORK="$(realpath -m "${WORK}")"
case "${WORK}/" in "${REPO}/"*) echo "evidence path must be outside the repository" >&2; exit 2 ;; esac
if [[ -e "${WORK}" || -L "${WORK}" ]]; then echo "evidence path ${WORK} already exists; choose a new path" >&2; exit 2; fi
DRY_RUN="${K8S4F_DRY_RUN:-0}"
CLUSTER="${K8S4F_CLUSTER:-k8s-4f}"
CONTEXT="kind-${CLUSTER}"
RUN_ID="${K8S4F_RUN_ID:-k8s4f-dev}"
# Loaded acceptance image tag, independent of the run ID (defaults to it).
IMAGE_TAG="${K8S4F_IMAGE_TAG:-${RUN_ID}}"
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

[[ "${RUN_ID}" =~ ^[a-z0-9-]+$ ]] || { echo "K8S4F_RUN_ID must be a DNS label" >&2; exit 2; }
[[ "${IMAGE_TAG}" =~ ^[a-z0-9][a-z0-9._-]*$ ]] || { echo "K8S4F_IMAGE_TAG is not a valid image tag" >&2; exit 2; }
for private in "${KUBECONFIG_FILE}" "${STATE}/token"; do
  [[ -f "${private}" && -s "${private}" ]] || { echo "private input missing or empty: ${private}" >&2; exit 1; }
  (( (8#$(stat -c %a "${private}") & 8#077) == 0 )) || { echo "private input must be owner-only (chmod 600): ${private}" >&2; exit 1; }
done
# Fresh run-specific supervisor pidfile: the retained supervisor's recovery handle
# (frontend-forward.pid) is never reused or overwritten; an existing file is refused.
FORWARD_PIDFILE="${K8S4F_FORWARD_PIDFILE:-${STATE}/frontend-forward-${RUN_ID}-${K8S4F_FORWARD_PORT:-18480}.pid}"
FORWARD_PIDFILE="$(realpath -m "${FORWARD_PIDFILE}")"
case "${FORWARD_PIDFILE}" in "${REPO}/"*|"${WORK}/"*) echo "forward pidfile must be outside the repository and the evidence dir" >&2; exit 2 ;; esac
[[ "${FORWARD_PIDFILE}" != "$(realpath -m "${STATE}/frontend-forward.pid")" ]] || { echo "forward pidfile must not be the retained frontend-forward.pid" >&2; exit 2; }
if [[ -e "${FORWARD_PIDFILE}" || -L "${FORWARD_PIDFILE}" ]]; then echo "forward pidfile ${FORWARD_PIDFILE} already exists; the existing supervisor is preserved, choose a new path" >&2; exit 2; fi
[[ -d "$(dirname "${FORWARD_PIDFILE}")" ]] || { echo "forward pidfile directory missing" >&2; exit 2; }
RESUME="${K8S4F_RESUME:-0}" RESUME_APP="${K8S4F_RESUME_APP:-0}" CONN_ONLY="${K8S4F_RESUME_CONNECTION_ONLY:-0}"
if [[ "${CONN_ONLY}" == 1 ]]; then
  # Partial resume: exactly one approved Connection key; no store/app/full-resume input.
  [[ "${RESUME}" == 0 && "${RESUME_APP}" == 0 && -n "${K8S4F_RESUME_CONNECTION_KEY:-}" && -z "${K8S4F_RESUME_STORE_KEY:-}" && -z "${K8S4F_RESUME_APPLICATION_ID:-}" ]] || { echo "K8S4F_RESUME_CONNECTION_ONLY needs only K8S4F_RESUME_CONNECTION_KEY" >&2; exit 2; }
fi
if [[ "${RESUME}" == 1 ]]; then
  [[ -n "${K8S4F_RESUME_CONNECTION_KEY:-}" && -n "${K8S4F_RESUME_STORE_KEY:-}" ]] || { echo "resume needs K8S4F_RESUME_CONNECTION_KEY and K8S4F_RESUME_STORE_KEY from the approved run record" >&2; exit 2; }
  if [[ "${RESUME_APP}" == 1 ]]; then [[ -n "${K8S4F_RESUME_APPLICATION_ID:-}" ]] || { echo "app resume needs K8S4F_RESUME_APPLICATION_ID" >&2; exit 2; }; fi
elif [[ "${RESUME_APP}" == 1 ]]; then echo "K8S4F_RESUME_APP requires K8S4F_RESUME=1" >&2; exit 2
fi
node --check "${REPO}/frontend/test/e2e/k8s4f-dev-human.mjs"
[[ -d "${REPO}/frontend/node_modules/@playwright/test" ]]
video_require_tools
if [[ "${DRY_RUN}" == 1 ]]; then echo "dry-run OK: offline preparation checks passed; nothing contacted, nothing created"; trap - EXIT; exit 0; fi
mkdir -m 0700 "${WORK}"
curl -fsS "${CONSOLE}/api/v1/healthz" >/dev/null
[[ "$(docker inspect -f '{{.State.Health.Status}}' "${VAULT_CONTAINER}")" == healthy ]]
kubectl --context "${CONTEXT}" -n vault-secrets-operator-system rollout status deployment/vault-secrets-operator-controller-manager --timeout=60s >/dev/null
for workload in backend frontend; do
  docker exec "${CLUSTER}-control-plane" crictl images 2>/dev/null | grep -q "acceptance-${workload} *${IMAGE_TAG}"
done
FORWARD_PORT="${K8S4F_FORWARD_PORT:-18480}"
if ss -ltn | grep -q ":${FORWARD_PORT} "; then echo "local port ${FORWARD_PORT} is busy" >&2; exit 1; fi
XDOTOOL="${ORCH_VIDEO_XDOTOOL:-$(command -v xdotool || true)}"
[[ -n "${XDOTOOL}" ]] || XDOTOOL="$(video_xdotool)"

video_start_xvfb "${SCREEN}"
set +e
DISPLAY="${VIDEO_DISPLAY}" ORCH_E2E_SCREEN="${SCREEN}" ORCH_E2E_XDOTOOL="${XDOTOOL}" \
  ORCH_E2E_REUSE="${RESUME}" ORCH_E2E_REUSE_CONNECTION_ONLY="${CONN_ONLY}" ORCH_E2E_REUSE_APP="${RESUME_APP}" ORCH_E2E_REUSE_CONNECTION_KEY="${K8S4F_RESUME_CONNECTION_KEY:-}" \
  ORCH_E2E_REUSE_STORE_KEY="${K8S4F_RESUME_STORE_KEY:-}" ORCH_E2E_REUSE_APPLICATION_ID="${K8S4F_RESUME_APPLICATION_ID:-}" \
  ORCH_E2E_URL="${CONSOLE}" ORCH_E2E_RUN_ID="${RUN_ID}" ORCH_E2E_IMAGE_TAG="${IMAGE_TAG}" \
  ORCH_E2E_FORBIDDEN_APP_IDS="${K8S4F_FORBIDDEN_APP_IDS:-}" ORCH_E2E_EXPECT_EMPTY_JOBS="${K8S4F_EXPECT_EMPTY_JOBS:-0}" ORCH_E2E_KUBE_CONTEXT="${CONTEXT}" \
  ORCH_E2E_EVIDENCE_DIR="${WORK}" ORCH_E2E_KUBECONFIG_FILE="${KUBECONFIG_FILE}" \
  ORCH_E2E_STORE_TOKEN_FILE="${STATE}/token" \
  ORCH_E2E_STORE_BACKEND="http://${VAULT_CONTAINER}:8200" ORCH_E2E_STORE_WORKLOAD="http://${VAULT_CONTAINER}:8200" \
  ORCH_E2E_FORWARD_PIDFILE="${FORWARD_PIDFILE}" ORCH_E2E_APPLICATION_NAME="${K8S4F_APPLICATION:-${RUN_ID}}" ORCH_E2E_SUBDOMAIN="${K8S4F_SUBDOMAIN:-${RUN_ID}}" \
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
  video_validate "${WORK}/k8s4f-dev-raw.mp4" "${SCREEN}" "${ORCH_VIDEO_MIN_SECONDS:-1}" 10 final-connections
fi
if [[ -s "${WORK}/k8s4f-dev-captioned.mp4" ]]; then
  ffprobe -v error -show_entries format=duration,size:stream=codec_name,width,height,avg_frame_rate,nb_frames -of json "${WORK}/k8s4f-dev-captioned.mp4" > "${WORK}/captioned-ffprobe.json" || true
  ffmpeg -hide_banner -v error -xerror -i "${WORK}/k8s4f-dev-captioned.mp4" -f null - 2> "${WORK}/captioned-decode.log" || echo "captioned video decode failed" >> "${WORK}/captioned-decode.log"
  if [[ "${RUN_STATUS}" -eq 0 && -s "${WORK}/captioned-decode.log" ]]; then exit 1; fi
fi
exit "${RUN_STATUS}"
