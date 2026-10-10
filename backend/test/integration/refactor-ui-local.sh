#!/usr/bin/env bash
# Local UI scenario runner for the Web Console refactor (refactor.md, V-UI):
#   refactor-ui-local.sh --scenario Txx --headed --captions vi --evidence NEW_PATH
# A fake-adapter backend with temporary JSON state serves the built Web Console;
# a headed Chromium on a private Xvfb display is recorded by ffmpeg and driven
# through the UI like a person (visible cursor, typed input, reading pauses).
# Vietnamese captions are burned into a strip below the 1440x900 browser area.
# Scenarios live in frontend/test/e2e/refactor-local.mjs; an unsupported Txx is
# rejected before anything starts. No Docker, cluster, cloud, database, Vault or
# .env: inherited external configuration is dropped. NEW_PATH must not exist
# and must be outside the repository; it is kept, also on failure.
# Debug only: ORCH_REFACTOR_INJECT_FAILURE=after-sign-in makes T01 fail on
# purpose to check that video, marks and assertions survive a failure.
#
# Requires Xvfb, ffmpeg, ffprobe and xdotool (see video-lib.sh).
#
# Output in NEW_PATH: <Txx>-raw.mp4, <Txx>-captioned.mp4, captions.srt/.ass,
# marks.json, assertions.txt, result.txt, run.json, ffprobe.json, frames/*.png.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
REPO="$(cd "${ROOT}/.." && pwd)"
E2E="${REPO}/frontend/test/e2e"
usage() { echo "usage: $0 --scenario Txx --headed --captions vi --evidence NEW_PATH" >&2; exit 2; }

SCENARIO="" HEADED=0 CAPTIONS="" EVIDENCE=""
while (( $# )); do
  case "$1" in
    --scenario) [[ $# -ge 2 ]] || usage; SCENARIO="$2"; shift 2 ;;
    --headed) HEADED=1; shift ;;
    --captions) [[ $# -ge 2 ]] || usage; CAPTIONS="$2"; shift 2 ;;
    --evidence) [[ $# -ge 2 ]] || usage; EVIDENCE="$2"; shift 2 ;;
    *) usage ;;
  esac
done
[[ -n "${SCENARIO}" && "${HEADED}" == 1 && -n "${CAPTIONS}" && -n "${EVIDENCE}" ]] || usage
[[ "${SCENARIO}" =~ ^T[0-9]{2}[A-Z]?$ ]] || { echo "invalid scenario ${SCENARIO}; expected Txx" >&2; exit 2; }
[[ "${CAPTIONS}" == vi ]] || { echo "unsupported captions ${CAPTIONS}; only vi" >&2; exit 2; }
command -v node >/dev/null || { echo "node is required" >&2; exit 1; }
# Reject an unsupported scenario before any build, process or directory.
node "${E2E}/refactor-local.mjs" --check "${SCENARIO}" || exit 2

EVIDENCE="$(realpath -m "${EVIDENCE}")"
case "${EVIDENCE}/" in "${REPO}/"*) echo "evidence path must be outside the repository" >&2; exit 2 ;; esac
if [[ -e "${EVIDENCE}" || -L "${EVIDENCE}" ]]; then
  echo "evidence path ${EVIDENCE} already exists; choose a new path" >&2
  exit 2
fi
mkdir -p "$(dirname "${EVIDENCE}")"
mkdir -m 0700 "${EVIDENCE}"
WORK="${EVIDENCE}"
export ORCH_VIDEO_DIR="${WORK}"
RUN_ID="refactor-$(date -u +%Y%m%d%H%M%S)-${RANDOM}"
SCREEN="${ORCH_VIDEO_SCREEN:-1440x900}"
MIN_SECONDS="${ORCH_VIDEO_MIN_SECONDS:-30}"
STRIP=96
PIDS=()
NODE_PID=""
NODE_PGID=""
source "${ROOT}/test/integration/video-lib.sh"

# Stops only the processes this run started (backend, Xvfb) and removes its
# temporary JSON state; evidence stays.
cleanup() {
  local status=$?
  local pid
  # The scenario process runs in its own process group (node, Chromium,
  # ffmpeg). SIGTERM lets it finalize the MP4 and evidence; only then is the
  # group killed, so a hung recorder cannot outlive the wrapper.
  if [[ -n "${NODE_PID}" ]] && kill -0 "${NODE_PID}" 2>/dev/null; then
    kill -TERM "${NODE_PID}" 2>/dev/null || true
    for _ in $(seq 1 80); do kill -0 "${NODE_PID}" 2>/dev/null || break; sleep 0.5; done
  fi
  # NODE_PGID outlives NODE_PID: the group holds only processes this run
  # started (setsid made node its leader), so a leftover Chromium or ffmpeg
  # after an unexpected node exit is killed without touching other processes.
  if [[ -n "${NODE_PGID}" ]] && kill -0 -- "-${NODE_PGID}" 2>/dev/null; then
    kill -KILL -- "-${NODE_PGID}" 2>/dev/null || true
  fi
  # Reap only after the group is gone, so a hung node cannot block cleanup.
  if [[ -n "${NODE_PID}" ]]; then wait "${NODE_PID}" 2>/dev/null || true; fi
  for pid in "${PIDS[@]}"; do kill "${pid}" 2>/dev/null || true; done
  for pid in "${PIDS[@]}"; do wait "${pid}" 2>/dev/null || true; done
  rm -f "${WORK}/state.json"
  echo "evidence in ${WORK}"
  echo "run-id=${RUN_ID} status=${status}"
  exit "${status}"
}
trap cleanup EXIT
trap 'exit 129' HUP
trap 'exit 130' INT
trap 'exit 143' TERM

video_require_tools
video_require_fresh_dist "${REPO}"
XDOTOOL="$(video_xdotool)"

(cd "${ROOT}" && go build -o "${WORK}/orchestrator" ./cmd/orchestrator)
# Temporary JSON state only: inherited database/Vault/Terraform/AWS/Kubernetes
# configuration is dropped and the matching flags are explicitly blank.
env -u ORCHESTRATOR_DATABASE_URL_FILE -u ORCHESTRATOR_VAULT_ADDR -u ORCHESTRATOR_VAULT_TOKEN_FILE \
  -u ORCHESTRATOR_CONNECTION_CREDENTIAL_STORE -u ORCHESTRATOR_CONNECTION_VAULT_ADDR -u ORCHESTRATOR_CONNECTION_VAULT_TOKEN_FILE \
  -u ORCHESTRATOR_VAULT_AGENT_ADDR -u ORCHESTRATOR_POSTGRES_TEST_URL -u ORCH_KIND_VERIFY -u KUBECONFIG \
  -u TF_PLUGIN_CACHE_DIR -u AWS_PROFILE -u AWS_REGION -u AWS_DEFAULT_REGION \
  -u AWS_ACCESS_KEY_ID -u AWS_SECRET_ACCESS_KEY -u AWS_SESSION_TOKEN \
  "${WORK}/orchestrator" -addr 127.0.0.1:0 -addr-file "${WORK}/api-addr" -state "${WORK}/state.json" \
  -database-url-file "" -vault-address "" -vault-token-file "" -vault-agent-address "" \
  -connection-credential-store none -adapters fake -profile test -run-id "${RUN_ID}" -ui-dir "${REPO}/frontend/dist" \
  > "${WORK}/orchestrator.log" 2>&1 &
PIDS+=($!)
for _ in $(seq 1 80); do [[ -s "${WORK}/api-addr" ]] && break; sleep 0.25; done
[[ -s "${WORK}/api-addr" ]] || { echo "backend did not start; see ${WORK}/orchestrator.log" >&2; exit 1; }
API="http://$(<"${WORK}/api-addr")"
for _ in $(seq 1 40); do curl -fsS "${API}/api/v1/healthz" >/dev/null 2>&1 && break; sleep 0.25; done
curl -fsS "${API}/api/v1/healthz" >/dev/null
printf '{"scenario":"%s","runId":"%s","adapters":"fake","captions":"%s"}\n' "${SCENARIO}" "${RUN_ID}" "${CAPTIONS}" > "${WORK}/run.json"

video_start_xvfb "${SCREEN}"
RAW="${WORK}/${SCENARIO}-raw.mp4"
STATUS=0
: > "${WORK}/playwright.log"
tail -n +1 -f "${WORK}/playwright.log" &
PIDS+=($!)
DISPLAY="${VIDEO_DISPLAY}" ORCH_E2E_SCREEN="${SCREEN}" ORCH_E2E_XDOTOOL="${XDOTOOL}" \
  ORCH_E2E_URL="${API}" ORCH_E2E_RUN_ID="${RUN_ID}" ORCH_E2E_EVIDENCE_DIR="${WORK}" \
  setsid node "${E2E}/refactor-local.mjs" --scenario "${SCENARIO}" --headed --captions "${CAPTIONS}" --evidence "${WORK}" \
  > "${WORK}/playwright.log" 2>&1 &
NODE_PID=$!
NODE_PGID="${NODE_PID}"
wait "${NODE_PID}" || STATUS=$?
NODE_PID=""
sleep 1

# Captions below the browser area, never over the UI. Built also after a
# failed run so the failure can be reviewed.
CAPTIONED="${WORK}/${SCENARIO}-captioned.mp4"
W="${SCREEN%x*}" H="${SCREEN#*x}"
if [[ -s "${RAW}" && -s "${WORK}/captions.srt" ]]; then
  node "${E2E}/render-captions.mjs" "${WORK}/captions.srt" "${WORK}/captions.ass" "${W}" "$((H + STRIP))"
  ffmpeg -hide_banner -loglevel error -n -i "${RAW}" -vf "pad=${W}:$((H + STRIP)):0:0:black,ass=${WORK}/captions.ass" \
    -r 15 -c:v libx264 -preset veryfast -crf 24 -pix_fmt yuv420p -movflags +faststart "${CAPTIONED}" || { echo "caption burn-in failed" >&2; STATUS=1; }
fi

if [[ "${STATUS}" != 0 ]]; then
  echo "scenario ${SCENARIO} failed; video, marks and assertions kept in ${WORK}" >&2
  exit "${STATUS}"
fi
[[ "$(<"${WORK}/result.txt")" == PASS ]] || { echo "result.txt is not PASS" >&2; exit 1; }
video_validate "${RAW}" "${SCREEN}" "${MIN_SECONDS}" 8 signed-out
ffmpeg -hide_banner -v error -xerror -i "${CAPTIONED}" -f null - 2> "${WORK}/decode-captioned.log" \
  || { echo "captioned video does not decode; see ${WORK}/decode-captioned.log" >&2; exit 1; }
[[ ! -s "${WORK}/decode-captioned.log" ]] || { echo "captioned video decode errors" >&2; exit 1; }
[[ "$(ffprobe -v error -select_streams v:0 -show_entries stream=width,height -of csv=p=0 "${CAPTIONED}")" == "${W},$((H + STRIP))" ]] \
  || { echo "captioned video has unexpected size" >&2; exit 1; }
echo "captioned=${CAPTIONED}"
