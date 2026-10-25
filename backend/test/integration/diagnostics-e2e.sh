#!/usr/bin/env bash
# Infrastructure Account diagnostics: real end-user browser acceptance (ADR-022,
# UC-04 DIAG-01..14). One run owns EVERYTHING it creates and removes exactly
# that:
#
#   * a Docker Compose project (PostgreSQL with separate owner/Console/State
#     logins, a real Vault, the backend and the built nginx frontend) built from
#     the COMMITTED tree (the working tree must be clean);
#   * a NEW kind cluster named cdiag-<run>, reached ONLY through the kubeconfig
#     uploaded in the Console form (an existing cluster is never reused);
#   * a headless Chromium driven like a user (frontend/test/e2e/account-diagnostics.mjs).
#
# No product API is mocked or intercepted and no fake diagnostic adapter is
# configured. Fixture provisioning is limited to platform content (Resource
# Types, Definitions, one Application with one saved, NEVER deployed workload
# draft), done through the product HTTP API by test/integration/diagfixture and
# logged in setup-api-calls.txt. The faults and repairs of the owned cluster
# (removing/creating a StorageClass) are native kubectl and are recorded in
# fixture-actions.txt. Everything the scenario claims about the user (onboarding,
# runs, inspection, repair/rerun, cancellation, switching, history, export,
# reload, restart) goes through the Console.
#
#   diagnostics-e2e.sh --evidence NEW_PATH
#
#
# Needs: docker (+compose), kind, kubectl, go, node, the Playwright Chromium.
# Evidence (NEW_PATH, outside the repository, must not exist): owned.json,
# run.json, assertions.json/.txt, result.txt, setup-api-calls.txt,
# fixture-actions.txt, playwright-*.log, screenshots/, cleanup.txt.
# Credentials live in NEW_PATH/private (0700) and are removed at the end.
set -euo pipefail
umask 077
export LC_ALL=C.UTF-8

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
REPO="$(cd "${ROOT}/.." && pwd)"
EVIDENCE=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --evidence) EVIDENCE="$2"; shift 2 ;;
    *) echo "usage: $0 --evidence NEW_PATH" >&2; exit 2 ;;
  esac
done
[[ -n "${EVIDENCE}" && ! -e "${EVIDENCE}" ]] || { echo "--evidence must name a path that does not exist" >&2; exit 2; }
case "${EVIDENCE}" in "${REPO}"/*) echo "evidence must live outside the repository" >&2; exit 2 ;; esac
for tool in docker kind kubectl go node curl python3; do command -v "${tool}" >/dev/null || { echo "missing ${tool}" >&2; exit 2; }; done
docker compose version >/dev/null || { echo "docker compose is required" >&2; exit 2; }
[[ -z "$(git -C "${REPO}" status --porcelain --untracked-files=no)" ]] || { echo "the working tree must be clean: acceptance runs the committed source" >&2; exit 2; }

SOURCE_SHA="$(git -C "${REPO}" rev-parse HEAD)"
RUN_ID="cdiag-$(date -u +%y%m%d%H%M%S)-${RANDOM}"
PROJECT="${RUN_ID}"
PRIVATE="${EVIDENCE}/private"
mkdir -p "${EVIDENCE}/screenshots" "${PRIVATE}"
chmod 0700 "${EVIDENCE}" "${PRIVATE}"
CLUSTER="${RUN_ID}"
CLUSTER_CREATED=0
BACKEND_IMAGE="orchestrator-backend:${RUN_ID}"
FRONTEND_IMAGE="orchestrator-frontend:${RUN_ID}"
BACKEND_PORT="$(python3 -c 'import socket;s=socket.socket();s.bind(("127.0.0.1",0));print(s.getsockname()[1])')"
FRONTEND_PORT="$(python3 -c 'import socket;s=socket.socket();s.bind(("127.0.0.1",0));print(s.getsockname()[1])')"
COMPOSE=(docker compose --env-file /dev/null -p "${PROJECT}" -f "${REPO}/docker-compose.yml" -f "${REPO}/deploy/local/compose.diagnostics-e2e.yml")
export BACKEND_IMAGE FRONTEND_IMAGE BACKEND_PORT FRONTEND_PORT
STATUS="NOT-STARTED"

deny_cluster() {
  case "$1" in
    k8s-4f|idp-internal|diagnose-review-*|kind|"") return 0 ;;
  esac
  [[ "$1" == cdiag-* ]] || return 0
  return 1
}
# The generated name must be new: nothing existing is ever adopted or deleted.
deny_cluster "${CLUSTER}" && { echo "refusing cluster ${CLUSTER}" >&2; exit 2; }
kind get clusters 2>/dev/null | grep -qx "${CLUSTER}" && { echo "cluster ${CLUSTER} already exists" >&2; exit 2; }

cat > "${EVIDENCE}/owned.json" <<JSON
{"runId":"${RUN_ID}","sourceSha":"${SOURCE_SHA}","composeProject":"${PROJECT}","cluster":"${CLUSTER}","clusterCreatedByRun":true,
 "images":["${BACKEND_IMAGE}","${FRONTEND_IMAGE}"],"ports":{"backend":${BACKEND_PORT},"frontend":${FRONTEND_PORT}}}
JSON

HOST_KUBECONFIG="${PRIVATE}/host.kubeconfig"
ACCOUNT_KUBECONFIG="${PRIVATE}/account.kubeconfig"
FRONTEND_URL="http://127.0.0.1:${FRONTEND_PORT}"
BACKEND_URL="http://127.0.0.1:${BACKEND_PORT}"
CLEANUP_OK=0

cleanup() {
  local code=$?
  set +e
  {
    echo "run ${RUN_ID} exit=${code}"
    bad=0
    "${COMPOSE[@]}" down -v --remove-orphans --timeout 20 2>&1 | tail -3
    [[ "${PIPESTATUS[0]}" == "0" ]] || { echo "ABSENCE-FAILED compose down failed"; bad=1; }
    docker rmi -f "${BACKEND_IMAGE}" "${FRONTEND_IMAGE}" >/dev/null 2>&1
    if [[ "${CLUSTER_CREATED}" == "1" ]]; then
      kind delete cluster --name "${CLUSTER}" 2>&1 | tail -1
    fi
    # Native absence of everything this run owned; a failed inventory is not "empty".
    out="$(docker ps -aq --filter "label=com.docker.compose.project=${PROJECT}" 2>&1)" && [[ -z "${out}" ]] || { echo "ABSENCE-FAILED containers: ${out}"; bad=1; }
    out="$(docker volume ls -q --filter "label=com.docker.compose.project=${PROJECT}" 2>&1)" && [[ -z "${out}" ]] || { echo "ABSENCE-FAILED volumes: ${out}"; bad=1; }
    out="$(docker network ls -q --filter "label=com.docker.compose.project=${PROJECT}" 2>&1)" && [[ -z "${out}" ]] || { echo "ABSENCE-FAILED network: ${out}"; bad=1; }
    out="$(docker images -q "${BACKEND_IMAGE}" "${FRONTEND_IMAGE}" 2>&1)" && [[ -z "${out}" ]] || { echo "ABSENCE-FAILED images: ${out}"; bad=1; }
    clusters="$(kind get clusters 2>&1)"; rc=$?
    if [[ "${rc}" != "0" ]] || grep -qx "${CLUSTER}" <<<"${clusters}"; then echo "ABSENCE-FAILED kind cluster inventory/remains"; bad=1; fi
    docker ps -a --format '{{.Names}}' | grep -q "^${CLUSTER}-" && { echo "ABSENCE-FAILED kind node container remains"; bad=1; }
    if [[ "${bad}" == "0" ]]; then echo "CLEANUP-PROVEN"; else echo "CLEANUP-NOT-PROVEN"; fi
  } > "${EVIDENCE}/cleanup.txt" 2>&1
  grep -q CLEANUP-PROVEN "${EVIDENCE}/cleanup.txt" && CLEANUP_OK=1
  # Credentials stay for recovery when cleanup is not proven.
  if [[ "${CLEANUP_OK}" == "1" ]]; then rm -rf "${PRIVATE}"; fi
  [[ "${code}" == "0" && "${CLEANUP_OK}" == "1" ]] && STATUS="PASS" || STATUS="FAIL"
  echo "${STATUS} run=${RUN_ID} source=${SOURCE_SHA}" | tee "${EVIDENCE}/result.txt"
  [[ "${STATUS}" == "PASS" ]] && exit 0 || exit 1
}
trap cleanup EXIT

fixture() { echo "$(date -u +%FT%TZ) $*" >> "${EVIDENCE}/fixture-actions.txt"; }

# 1. New owned kind cluster. The flag is set before creation so a partial
# creation is deleted too.
CLUSTER_CREATED=1
KUBECONFIG="${HOST_KUBECONFIG}" kind create cluster --name "${CLUSTER}" --wait 180s >/dev/null
kind get kubeconfig --internal --name "${CLUSTER}" > "${ACCOUNT_KUBECONFIG}"
chmod 0600 "${HOST_KUBECONFIG}" "${ACCOUNT_KUBECONFIG}"
CONTEXT="kind-${CLUSTER}"
K=(kubectl --kubeconfig "${HOST_KUBECONFIG}" --context "${CONTEXT}")
# The account kubeconfig must hold exactly one context.
python3 - "${ACCOUNT_KUBECONFIG}" <<'PY'
import re, sys
text = open(sys.argv[1]).read()
assert len(re.findall(r"^- context:", text, re.M)) == 1, "the account kubeconfig must hold one context"
PY
# The class the acceptance Definition requires ("fast") must not exist in a
# fresh cluster; the real local-path provisioner must be rolled out before
# the first check.
"${K[@]}" get storageclass fast >/dev/null 2>&1 && { echo "unexpected StorageClass fast in a fresh cluster" >&2; exit 1; }
fixture "fresh cluster ${CLUSTER}: StorageClass fast absent (start state)"
"${K[@]}" -n local-path-storage rollout status deploy/local-path-provisioner --timeout=180s >/dev/null
fixture "local-path-provisioner rollout complete"

# 2. Real stack built from the committed tree.
"${COMPOSE[@]}" up -d --build --wait --wait-timeout 600 > "${EVIDENCE}/compose-up.log" 2>&1
for _ in $(seq 1 60); do curl -fsS "${BACKEND_URL}/api/v1/healthz" >/dev/null 2>&1 && break; sleep 1; done
curl -fsS "${BACKEND_URL}/api/v1/healthz" >/dev/null
BACKEND_CONTAINER="$("${COMPOSE[@]}" ps -q backend)"

cat > "${EVIDENCE}/run.json" <<JSON
{"runId":"${RUN_ID}","sourceSha":"${SOURCE_SHA}","frontend":"${FRONTEND_URL}","backend":"${BACKEND_URL}","context":"${CONTEXT}",
 "composeProject":"${PROJECT}","startedAt":"$(date -u +%FT%TZ)"}
JSON

export ORCH_E2E_URL="${FRONTEND_URL}" ORCH_E2E_BACKEND="${BACKEND_URL}" ORCH_E2E_EVIDENCE="${EVIDENCE}" ORCH_E2E_ACCOUNT_KUBECONFIG="${ACCOUNT_KUBECONFIG}"
export ORCH_E2E_HOST_KUBECONFIG="${HOST_KUBECONFIG}" ORCH_E2E_CONTEXT="${CONTEXT}" ORCH_E2E_COMPOSE_PROJECT="${PROJECT}"
export ORCH_E2E_BACKEND_CONTAINER="${BACKEND_CONTAINER}" ORCH_E2E_STATE="${PRIVATE}/state.json" ORCH_E2E_RUN_ID="${RUN_ID}"
PLAYWRIGHT=(node "${REPO}/frontend/test/e2e/account-diagnostics.mjs")

phase() { ORCH_E2E_PHASE="$1" "${PLAYWRIGHT[@]}" 2>&1 | tee "${EVIDENCE}/playwright-$1.log"; return "${PIPESTATUS[0]}"; }

# 3. Onboarding through the form, then a general check without an application.
phase general
ACCOUNT_KEY="$(python3 -c 'import json;print(json.load(open("'"${ORCH_E2E_STATE}"'"))["accountKey"])')"

# 4. Platform content for the desired-state scenario (API, logged).
(cd "${ROOT}" && go run ./test/integration/diagfixture -base "${BACKEND_URL}" -account "${ACCOUNT_KEY}" -log "${EVIDENCE}/setup-api-calls.txt" -storage-class fast) > "${PRIVATE}/fixture.json"
python3 - "${ORCH_E2E_STATE}" "${PRIVATE}/fixture.json" <<'PY'
import json, sys
state = json.load(open(sys.argv[1])); state.update(json.load(open(sys.argv[2]))); json.dump(state, open(sys.argv[1], "w"))
PY

# 5. Desired-state, repair, execution, cancellation, switching, restart, interruption, roles.
phase desired
echo "scenario complete" >&2
