#!/usr/bin/env bash
# Human-paced UC-02/03/04 registration video: fake-adapter backend, temporary
# JSON state, headed Chromium on a private Xvfb display recorded by ffmpeg.
# No Docker, cloud or API fixtures: only the seeded local test accounts and
# catalog exist; every shown step is a UI action.
#
# Usage: uc02-04-video-local.sh [--kind]
#   default  no cluster is contacted; the seeded Connection and an ID
#            rejection are shown.
#   --kind   the backend's real kubectl Connection verifier (always wired by
#            cmd/orchestrator, independent of -adapters) checks the existing
#            kind-idp-internal context read-only: a missing context is
#            rejected and a run-unique Connection becomes READY. Executor
#            adapters stay fake, so no Kubernetes object is created; the
#            Connection exists only in this run's temporary state.
#
# Requires Xvfb, ffmpeg, ffprobe and xdotool (see video-lib.sh).
#
# Output: ${ORCH_VIDEO_DIR:-/tmp/<run id>.XXXXXX}/uc02-04[-kind]-review.mp4
# plus run.json, marks.json, ffprobe.json and frames/*.png. The directory is
# kept, also on failure; nothing is committed.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
REPO="$(cd "${ROOT}/.." && pwd)"
KIND_CONTEXT=""
case "${1:-}" in
  "") ;;
  --kind) KIND_CONTEXT="kind-idp-internal" ;;
  *) echo "usage: $0 [--kind]" >&2; exit 2 ;;
esac
RUN_ID="uc02-04-${KIND_CONTEXT:+kind-}video-$(date -u +%Y%m%d%H%M%S)-${RANDOM}"
SCREEN="${ORCH_VIDEO_SCREEN:-1440x900}"
MIN_SECONDS="${ORCH_VIDEO_MIN_SECONDS:-90}"
VIDEO_NAME="uc02-04${KIND_CONTEXT:+-kind}-review.mp4"
MIN_MARKS=17
[[ -z "${KIND_CONTEXT}" ]] || MIN_MARKS=19
PIDS=()

# Always write into a directory this run creates, so earlier evidence or any
# other files are never overwritten or removed.
if [[ -n "${ORCH_VIDEO_DIR:-}" ]]; then
  if [[ -e "${ORCH_VIDEO_DIR}" || -L "${ORCH_VIDEO_DIR}" ]]; then
    echo "ORCH_VIDEO_DIR ${ORCH_VIDEO_DIR} already exists; choose a new path" >&2
    exit 1
  fi
  mkdir -m 0700 "${ORCH_VIDEO_DIR}"
  WORK="$(cd "${ORCH_VIDEO_DIR}" && pwd)"
else
  WORK="$(mktemp -d "${TMPDIR:-/tmp}/${RUN_ID}.XXXXXX")"
fi
# shellcheck source=video-lib.sh
source "${ROOT}/test/integration/video-lib.sh"

cleanup() {
  local status=$?
  local pid
  for pid in "${PIDS[@]}"; do kill "${pid}" 2>/dev/null || true; done
  for pid in "${PIDS[@]}"; do wait "${pid}" 2>/dev/null || true; done
  rm -f "${WORK}/state.json"
  echo "evidence in ${WORK}"
  echo "run-id=${RUN_ID} status=${status}"
  exit "${status}"
}
trap cleanup EXIT
# A signal still runs cleanup and reports a nonzero status: 128 + signal.
trap 'exit 129' HUP
trap 'exit 130' INT
trap 'exit 143' TERM

video_require_tools
video_require_fresh_dist "${REPO}"
XDOTOOL="$(video_xdotool)"
if [[ -n "${KIND_CONTEXT}" ]]; then
  # Read-only preflight; the current context is neither read nor changed.
  kubectl config get-contexts -o name | grep -qx "${KIND_CONTEXT}" || { echo "context ${KIND_CONTEXT} is not configured" >&2; exit 1; }
  kubectl --context "${KIND_CONTEXT}" version -o json >/dev/null
fi

(cd "${ROOT}" && go build -o "${WORK}/orchestrator" ./cmd/orchestrator)
# Temporary JSON state only: inherited database/Vault/Terraform/AWS defaults
# are dropped and the corresponding flags are explicitly blank.
env -u ORCHESTRATOR_DATABASE_URL_FILE -u ORCHESTRATOR_VAULT_ADDR -u ORCHESTRATOR_VAULT_TOKEN_FILE \
  -u ORCHESTRATOR_VAULT_AGENT_ADDR -u TF_PLUGIN_CACHE_DIR -u AWS_PROFILE -u AWS_REGION -u AWS_DEFAULT_REGION \
  -u AWS_ACCESS_KEY_ID -u AWS_SECRET_ACCESS_KEY -u AWS_SESSION_TOKEN \
  "${WORK}/orchestrator" -addr 127.0.0.1:0 -addr-file "${WORK}/api-addr" -state "${WORK}/state.json" \
  -database-url-file "" -vault-address "" -vault-token-file "" -vault-agent-address "" \
  -adapters fake -profile test -run-id "${RUN_ID}" -ui-dir "${REPO}/frontend/dist" \
  > "${WORK}/orchestrator.log" 2>&1 &
PIDS+=($!)
for _ in $(seq 1 80); do [[ -s "${WORK}/api-addr" ]] && break; sleep 0.25; done
[[ -s "${WORK}/api-addr" ]] || { echo "backend did not start; see ${WORK}/orchestrator.log" >&2; exit 1; }
API="http://$(<"${WORK}/api-addr")"
for _ in $(seq 1 40); do curl -fsS "${API}/api/v1/healthz" >/dev/null 2>&1 && break; sleep 0.25; done
curl -fsS "${API}/api/v1/healthz" >/dev/null

video_start_xvfb "${SCREEN}"
DISPLAY="${VIDEO_DISPLAY}" ORCH_E2E_SCREEN="${SCREEN}" ORCH_E2E_XDOTOOL="${XDOTOOL}" \
  ORCH_E2E_URL="${API}" ORCH_E2E_RUN_ID="${RUN_ID}" ORCH_E2E_EVIDENCE_DIR="${WORK}" \
  ORCH_E2E_KIND_CONTEXT="${KIND_CONTEXT}" \
  node "${REPO}/frontend/test/e2e/uc02-04-video-local.mjs" 2>&1 | tee "${WORK}/playwright.log"

video_validate "${WORK}/${VIDEO_NAME}" "${SCREEN}" "${MIN_SECONDS}" "${MIN_MARKS}" signed-out
