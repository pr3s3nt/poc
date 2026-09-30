#!/usr/bin/env bash
# Local UC-00/UC-01 onboarding verification without Docker, kind or cloud:
# a fake-adapter backend with JSON state serves the built Web Console, a
# headless Chromium signs in, creates an Application, signs out and back in,
# then the backend restarts on the same state and the Application must remain.
# Set ORCH_KEEP_EVIDENCE=1 to keep logs and state in the temporary directory.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
REPO="$(cd "${ROOT}/.." && pwd)"
WORK="$(mktemp -d)"
chmod 0700 "${WORK}"
RUN_ID="onb-$(date -u +%Y%m%d%H%M%S)-${RANDOM}"
BACKEND_PID=""

stop_backend() {
  if [[ -n "${BACKEND_PID}" ]]; then
    kill "${BACKEND_PID}" 2>/dev/null || true
    wait "${BACKEND_PID}" 2>/dev/null || true
    BACKEND_PID=""
  fi
}

cleanup() {
  local status=$?
  stop_backend
  if [[ "${ORCH_KEEP_EVIDENCE:-0}" == "1" || "${status}" != "0" ]]; then
    echo "evidence kept in ${WORK}"
  else
    rm -rf "${WORK}"
  fi
  echo "run-id=${RUN_ID} status=${status}"
  exit "${status}"
}
trap cleanup EXIT

[[ -f "${REPO}/frontend/dist/index.html" ]] || { echo "build the Web Console first: cd frontend && npm run build" >&2; exit 1; }
[[ -d "${REPO}/frontend/node_modules/@playwright/test" ]] || { echo "install frontend dependencies first" >&2; exit 1; }

start_backend() {
  local label="$1"
  rm -f "${WORK}/api-addr"
  "${WORK}/orchestrator" -addr 127.0.0.1:0 -addr-file "${WORK}/api-addr" -state "${WORK}/state.json" \
    -adapters fake -profile test -run-id "${RUN_ID}" -ui-dir "${REPO}/frontend/dist" \
    > "${WORK}/orchestrator-${label}.log" 2>&1 &
  BACKEND_PID=$!
  for _ in $(seq 1 80); do [[ -s "${WORK}/api-addr" ]] && break; sleep 0.25; done
  [[ -s "${WORK}/api-addr" ]] || { echo "backend did not start; see ${WORK}/orchestrator-${label}.log" >&2; return 1; }
  API="http://$(<"${WORK}/api-addr")"
  for _ in $(seq 1 40); do curl -fsS "${API}/api/v1/healthz" >/dev/null 2>&1 && return 0; sleep 0.25; done
  echo "backend is not healthy" >&2
  return 1
}

run_phase() {
  ORCH_E2E_URL="${API}" ORCH_E2E_PHASE="$1" ORCH_E2E_RUN_ID="${RUN_ID}" ORCH_E2E_APP_FILE="${WORK}/application-id" \
    node "${REPO}/frontend/test/e2e/onboarding-local.mjs" 2>&1 | tee "${WORK}/playwright-$1.log"
}

(cd "${ROOT}" && go build -o "${WORK}/orchestrator" ./cmd/orchestrator)

start_backend first
run_phase create
stop_backend
[[ -s "${WORK}/state.json" ]] || { echo "JSON state was not written" >&2; exit 1; }

start_backend restarted
run_phase restart
echo "onboarding verification passed: application=$(<"${WORK}/application-id")"
