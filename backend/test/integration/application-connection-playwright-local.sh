#!/usr/bin/env bash
# Local UC-01 connection-selection verification without Docker, kind or cloud:
# a fake-adapter backend with JSON state and a read-only kubectl stand-in
# serves the built Web Console. A Platform Engineer registers a second
# Connection and a matching cluster Definition, a Developer selects that
# Connection when creating an Application, previews and deploys, then the
# backend restarts on the same state and the binding must remain.
# Set ORCH_KEEP_EVIDENCE=1 to keep logs and state in the temporary directory;
# ORCH_EVIDENCE_DIR=<new path> keeps them there instead.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
REPO="$(cd "${ROOT}/.." && pwd)"
if [[ -n "${ORCH_EVIDENCE_DIR:-}" ]]; then
  [[ ! -e "${ORCH_EVIDENCE_DIR}" ]] || { echo "ORCH_EVIDENCE_DIR ${ORCH_EVIDENCE_DIR} already exists; choose a new path" >&2; exit 1; }
  mkdir -m 0700 "${ORCH_EVIDENCE_DIR}"
  WORK="${ORCH_EVIDENCE_DIR}"
  ORCH_KEEP_EVIDENCE=1
else
  WORK="$(mktemp -d)"
  chmod 0700 "${WORK}"
fi
RUN_ID="conn-$(date -u +%Y%m%d%H%M%S)-${RANDOM}"
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
  # Only the stand-in kubectl is reachable: no real cluster or cloud call.
  STUB_CONTEXTS="lab-context" "${WORK}/orchestrator" -addr 127.0.0.1:0 -addr-file "${WORK}/api-addr" -state "${WORK}/state.json" \
    -adapters fake -profile test -run-id "${RUN_ID}" -ui-dir "${REPO}/frontend/dist" \
    -kubectl "${ROOT}/test/integration/stubs/kubectl-connection-stub.sh" \
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
    ORCH_E2E_STATE_FILE="${WORK}/state.json" ORCH_E2E_SHOTS="${WORK}/screenshots" \
    node "${REPO}/frontend/test/e2e/application-connection-local.mjs" 2>&1 | tee "${WORK}/playwright-$1.log"
}

mkdir -p "${WORK}/screenshots"
(cd "${ROOT}" && go build -o "${WORK}/orchestrator" ./cmd/orchestrator)

start_backend first
run_phase create
stop_backend
[[ -s "${WORK}/state.json" ]] || { echo "JSON state was not written" >&2; exit 1; }

start_backend restarted
run_phase restart
echo "application connection verification passed: application=$(<"${WORK}/application-id")"
