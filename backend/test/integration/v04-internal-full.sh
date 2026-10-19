#!/usr/bin/env bash
# V04 integrated acceptance (issue #4): the ENTIRE acceptance application (frontend, backend, worker) with a REAL shared
# PostgreSQL and a REAL shared Redis on an existing Kubernetes cluster, placed from Resource Definitions and a resolved
# dependency graph only (ADR-016, ADR-017). One headed, captioned run: Internal account upload, Secret Store, Types,
# existing-cluster / namespace / PostgreSQL / Redis / workload Definitions, Environment keys and the secret, three Score
# imports, Score Preview and pending Preview, Deploy, the real application (check rows, job form, worker, PostgreSQL row,
# Redis write/hit/TTL expiry/refill), valid update and orchestrator restart; intentional dependency
# failure, ordered removal and the verified cleanup of everything the run owns.
#
#   v04-internal-full.sh --headed --captions vi --evidence NEW_PATH
#
# REAL: the Web Console bundle, the product built from this tree (planner -> pinned Terraform with encrypted PostgreSQL
#   state -> durable Vault Secret refs -> required pinned score-k8s -> real Kubernetes delivery), a disposable PostgreSQL
#   database with separate Owner/Console/State logins, the existing scoped Vault for credentials, a RUN-OWNED second Vault
#   (Kubernetes auth, bridged to the Pods) for the application secret, the existing cluster reached ONLY through the
#   uploaded kubeconfig account, the headed browser and the actual acceptance images. FAKE: nothing. The host kubeconfig
#   handle is read by this runner and its safety net ONLY (read-only assertions, port-forward, kubectl exec in the run's own
#   namespace, guarded delete of the exact run-owned namespaces); the product never sees it and runs with a cleared env.
# SCORE SOURCE: backend/internal/seed/scores.go through `v2ctl v04-scores`; every difference from that source is recorded in
#   scores/scores.json (literal variables become Environment keys, in-cluster classes, deployment-check keys, Service
#   reference). Requests/limits, probes, ports and images are preserved and asserted equal by Go tests.
#
# Required environment (every path is an externally prepared file; nothing is read from a default or retained location,
# and no private file is copied into the evidence):
#   ORCH_V04_PG_ADMIN_URL_FILE      0600 administrator URL of a disposable PostgreSQL server you own (ONE database, three roles).
#   ORCH_V04_OUTPUT_VAULT_TOKEN_FILE  0600 output writer token (orchestrator/outputs/*: create,read).
#   ORCH_V04_OUTPUT_CLEANUP_TOKEN_FILE 0600 distinct operator token, scoped to Vault KV metadata deletion/read under orchestrator/outputs/*.
#   ORCH_V04_VAULT_TOKEN_FILE       0600 CONNECTION-ONLY Vault token (orchestrator/connections/*).
#   ORCH_V04_VAULT_ADDR             Vault address, or ORCH_V04_RUNTIME_JSON with {"vaultAddress": "...", "vaultMount": "secret"}.
#   ORCH_V04_ACCOUNT_KUBECONFIG_FILE  single-context kubeconfig uploaded through the Console as the Internal account.
#   Intentional account/credential/dependency failure probes are outside current acceptance scope.
#   ORCH_V04_HOST_KUBECONFIG, ORCH_V04_HOST_CONTEXT   host kubeconfig handle (runner/safety net only) and its exact context.
#   ORCH_V04_KUBECTL, ORCH_V04_TERRAFORM (1.9.8), ORCH_V04_SCORE_K8S (0.15.0)   pinned binaries.
#   ORCH_V04_VAULT_BIN              pinned vault binary for the run-owned secret-store Vault.
#   ORCH_V04_SECRET_STORE_JSON      0600 JSON of the run-owned secret store: {"name","bridgeAddress","kubernetesHost","reviewerJwtFile","reviewerCaFile"}
#                                   (bridgeAddress: IPv4 of the Docker bridge reachable from the Pods; reviewer files: the EXISTING reviewer handles).
#   ORCH_V04_IMAGE_FRONTEND_A / _DIGEST, ORCH_V04_IMAGE_FRONTEND_B / _DIGEST, ORCH_V04_IMAGE_BACKEND, ORCH_V04_IMAGE_WORKER
#                                   the acceptance images (already loaded into the cluster) and the digests of the two frontend images.
# Optional:
#   ORCH_V04_DEADLINE_SECONDS (3600: independent supervisor), ORCH_V04_EXEC_SECONDS (3600: product execution TTL), ORCH_V04_CACHE_TTL_SECONDS (30),
#   ORCH_V04_VAULT_MOUNT (secret), ORCH_V04_ORG (acme), ORCH_V04_OWNER, ORCH_V04_RUN_ID,
#   ORCH_VIDEO_SCREEN (1440x900), ORCH_VIDEO_MIN_SECONDS (300).
# Needs: go, node, curl, Xvfb, xdotool, ffmpeg/ffprobe, a built frontend/dist and the Playwright Chromium.
#
# Evidence (NEW_PATH, outside the repository, must not exist; kept also on failure): V04-raw.mp4, V04-captioned.mp4 (captions BELOW the
# browser), captions.srt/.ass, marks.json, phases.json, assertions.txt/.json, result.txt, run.json (public inputs, source and binary hashes,
# Score hashes), checks.txt, cleanup.txt, setup-api-calls.txt, scores/ (Score documents + scores.json), orchestrator-*.log, failure.png.
# Credentials, state and handles live in NEW_PATH/private (0700). An independent supervisor (own session, survives a SIGKILL of this
# runner) stops every owned process group, the run-owned Vault and then cleans the run with v03-cleanup.sh. When the cleanup is not proven
# the database, state key, recovery artifacts and credentials are KEPT and CLEANUP-NOT-PROVEN.txt says so.
# Debug / fault checks (each makes the run exit non-zero by design):
#   ORCH_V04_INJECT_FAILURE=after-success   fail after the scenario; video, marks, assertions and cleanup survive.
set -euo pipefail
umask 077
export LC_ALL=C.UTF-8

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
REPO="$(cd "${ROOT}/.." && pwd)"
E2E="${REPO}/frontend/test/e2e"
usage() { echo "usage: $0 --headed --captions vi --evidence NEW_PATH" >&2; exit 2; }

HEADED=0 CAPTIONS="" EVIDENCE=""
while (( $# )); do
  case "$1" in
    --headed) HEADED=1; shift ;;
    --captions) [[ $# -ge 2 ]] || usage; CAPTIONS="$2"; shift 2 ;;
    --evidence) [[ $# -ge 2 ]] || usage; EVIDENCE="$2"; shift 2 ;;
    *) usage ;;
  esac
done
[[ "${HEADED}" == 1 && "${CAPTIONS}" == vi && -n "${EVIDENCE}" ]] || usage

need_file() { [[ -n "${!1:-}" && -f "${!1}" ]] || { echo "$1 must name an existing file" >&2; exit 2; }; }
need_exec() { [[ -n "${!1:-}" && -x "${!1}" ]] || { echo "$1 must name an executable" >&2; exit 2; }; }
need_value() { [[ -n "${!1:-}" ]] || { echo "$1 is required" >&2; exit 2; }; }
for name in ORCH_V04_PG_ADMIN_URL_FILE ORCH_V04_VAULT_TOKEN_FILE ORCH_V04_OUTPUT_VAULT_TOKEN_FILE ORCH_V04_OUTPUT_CLEANUP_TOKEN_FILE ORCH_V04_ACCOUNT_KUBECONFIG_FILE ORCH_V04_HOST_KUBECONFIG; do need_file "${name}"; done
for name in ORCH_V04_KUBECTL ORCH_V04_TERRAFORM ORCH_V04_SCORE_K8S; do need_exec "${name}"; done
for name in ORCH_V04_HOST_CONTEXT ORCH_V04_IMAGE_FRONTEND_A ORCH_V04_IMAGE_FRONTEND_A_DIGEST ORCH_V04_IMAGE_FRONTEND_B ORCH_V04_IMAGE_FRONTEND_B_DIGEST ORCH_V04_IMAGE_BACKEND ORCH_V04_IMAGE_WORKER; do need_value "${name}"; done
need_exec ORCH_V04_VAULT_BIN
need_file ORCH_V04_SECRET_STORE_JSON
[[ "${ORCH_V04_IMAGE_FRONTEND_A}" != "${ORCH_V04_IMAGE_FRONTEND_B}" && "${ORCH_V04_IMAGE_FRONTEND_A_DIGEST}" != "${ORCH_V04_IMAGE_FRONTEND_B_DIGEST}" ]] || { echo "the two frontend images must differ (reference and digest)" >&2; exit 2; }
[[ "${ORCH_V04_IMAGE_FRONTEND_A_DIGEST}" =~ ^sha256:[0-9a-f]{64}$ && "${ORCH_V04_IMAGE_FRONTEND_B_DIGEST}" =~ ^sha256:[0-9a-f]{64}$ ]] || { echo "image digests must be sha256:<64 hex>" >&2; exit 2; }
SS_JSON="$(cat "${ORCH_V04_SECRET_STORE_JSON}")"
node -e 'const s=JSON.parse(process.argv[1]); for (const k of ["name","bridgeAddress","kubernetesHost","reviewerJwtFile","reviewerCaFile"]) if(!s[k]) { console.error("secret store JSON lacks "+k); process.exit(1) } if(!/^https:\/\//.test(s.kubernetesHost)||!/^[0-9]+(\.[0-9]+){3}$/.test(s.bridgeAddress)) { console.error("secret store JSON has an invalid bridgeAddress or kubernetesHost"); process.exit(1) }' "${SS_JSON}" || exit 2
RESTRICTED="" # historical permission probe is outside main-flow acceptance
VAULT_ADDR_LOCAL="${ORCH_V04_VAULT_ADDR:-}"
MOUNT="${ORCH_V04_VAULT_MOUNT:-secret}"
if [[ -z "${VAULT_ADDR_LOCAL}" && -n "${ORCH_V04_RUNTIME_JSON:-}" ]]; then
  need_file ORCH_V04_RUNTIME_JSON
  VAULT_ADDR_LOCAL="$(node -e 'const j=JSON.parse(require("fs").readFileSync(process.argv[1],"utf8"));process.stdout.write(j.vaultAddress??"")' "${ORCH_V04_RUNTIME_JSON}")"
  if [[ -z "${ORCH_V04_VAULT_MOUNT:-}" ]]; then MOUNT="$(node -e 'const j=JSON.parse(require("fs").readFileSync(process.argv[1],"utf8"));process.stdout.write(j.vaultMount??"secret")' "${ORCH_V04_RUNTIME_JSON}")"; fi
fi
[[ "${VAULT_ADDR_LOCAL}" =~ ^https?://[A-Za-z0-9.:-]+$ ]] || { echo "set ORCH_V04_VAULT_ADDR or ORCH_V04_RUNTIME_JSON with vaultAddress" >&2; exit 2; }
for tool in node go curl ffmpeg ffprobe Xvfb; do command -v "${tool}" >/dev/null || { echo "${tool} is required" >&2; exit 2; }; done
node "${E2E}/v04-internal-full.mjs" --check V04 || exit 2

PG_ADMIN="${ORCH_V04_PG_ADMIN_URL_FILE}"
KUBECTL="${ORCH_V04_KUBECTL}" TERRAFORM="${ORCH_V04_TERRAFORM}" SCORE_K8S="${ORCH_V04_SCORE_K8S}"
HOST_KUBECONFIG="${ORCH_V04_HOST_KUBECONFIG}" HOST_CONTEXT="${ORCH_V04_HOST_CONTEXT}"
ORG="${ORCH_V04_ORG:-acme}" OWNER="${ORCH_V04_OWNER:-poc-v04}" EXEC_SECONDS="${ORCH_V04_EXEC_SECONDS:-3600}"
OUTPUT_VAULT_ADDR="${VAULT_ADDR_LOCAL}" OUTPUT_MOUNT="${MOUNT}"
CACHE_TTL="${ORCH_V04_CACHE_TTL_SECONDS:-30}"
VAULT_BIN="${ORCH_V04_VAULT_BIN}"
SUFFIX="$(head -c 3 /dev/urandom | od -An -tx1 | tr -d ' \n')"
RUN_ID="${ORCH_V04_RUN_ID:-v04-$(date -u +%m%d%H%M%S)-${SUFFIX}}"
[[ "${RUN_ID}" =~ ^[a-z0-9-]{6,40}$ ]] || { echo "ORCH_V04_RUN_ID must match [a-z0-9-]{6,40}" >&2; exit 2; }
NAMESPACE="orch-v2-test-${RUN_ID}"
COMPANION="${NAMESPACE}-aux"
APPLICATION_KEY="acc-${RUN_ID}"
[[ ${#COMPANION} -le 63 ]] || { echo "the run namespaces would exceed 63 characters; use a shorter ORCH_V04_RUN_ID" >&2; exit 2; }

EVIDENCE="$(realpath -m "${EVIDENCE}")"
case "${EVIDENCE}/" in "${REPO}/"*) echo "evidence path must be outside the repository" >&2; exit 2 ;; esac
if [[ -e "${EVIDENCE}" || -L "${EVIDENCE}" ]]; then echo "evidence path ${EVIDENCE} already exists; choose a new path" >&2; exit 2; fi
mkdir -p "$(dirname "${EVIDENCE}")"
mkdir -m 0700 "${EVIDENCE}"
WORK="${EVIDENCE}"
PRIVATE="${WORK}/private"
mkdir -m 0700 "${PRIVATE}" "${PRIVATE}/bin" "${PRIVATE}/tfroot" "${PRIVATE}/plugin-cache" "${PRIVATE}/module-cache" "${PRIVATE}/recovery" "${PRIVATE}/home" "${PRIVATE}/tmp" "${WORK}/scores"
export ORCH_VIDEO_DIR="${WORK}"
SCREEN="${ORCH_VIDEO_SCREEN:-1440x900}"
MIN_SECONDS="${ORCH_VIDEO_MIN_SECONDS:-300}"
STRIP=96
PIDS=() SERVER_PID="" NODE_PID="" PG_PROVISIONED=0 PG_DROP_CLEAN=1 STATUS=0 CLEANUP_FAILED=0 FINISHED=0 REOPEN_PASSED=0 FINAL_VIDEO_PASSED=0 SECRET_SCAN_PASSED=0 PRIVATE_REMOVED=0
# shellcheck source=video-lib.sh
source "${ROOT}/test/integration/video-lib.sh"
: > "${WORK}/cleanup.txt"
: > "${WORK}/checks.txt"
note() { printf '%s\n' "$*" >> "${WORK}/cleanup.txt"; }
check() { printf '%s\n' "$*" | tee -a "${WORK}/checks.txt"; }

# ---- Operator tools, built from this tree into the private directory ----------------------------
(cd "${ROOT}" && go build -o "${PRIVATE}/bin/v2ctl" ./test/integration/v2ctl && go build -o "${PRIVATE}/bin/orchestrator" ./cmd/orchestrator && go build -o "${PRIVATE}/bin/v2cleanup" ./test/integration/v2cleanup)
CTL="${PRIVATE}/bin/v2ctl"
ORCH="${PRIVATE}/bin/orchestrator"

# ---- Pinned binaries ------------------------------------------------------------------------------
"${TERRAFORM}" version | head -1 | grep -q 'Terraform v1\.9\.8' || { echo "Terraform 1.9.8 is required" >&2; exit 2; }
"${SCORE_K8S}" --version | head -1 | grep -q '0\.15\.0' || { echo "score-k8s 0.15.0 is required" >&2; exit 2; }
check "pinned binaries: terraform 1.9.8 sha256:$(sha256sum "${TERRAFORM}" | cut -d' ' -f1) score-k8s 0.15.0 sha256:$(sha256sum "${SCORE_K8S}" | cut -d' ' -f1) kubectl sha256:$(sha256sum "${KUBECTL}" | cut -d' ' -f1)"

# ---- Independent supervisor: owns the cleanup even when this runner is killed -----------------------------------------
CLEANUP="${ROOT}/test/integration/v03-cleanup.sh"
SUPERVISOR="${ROOT}/test/integration/v03-supervisor.sh"
{
  for name in WORK PRIVATE CTL KUBECTL HOST_KUBECONFIG HOST_CONTEXT RUN_ID ORG VAULT_ADDR_LOCAL MOUNT PG_ADMIN OUTPUT_VAULT_ADDR OUTPUT_MOUNT; do printf '%s=%q\n' "${name}" "${!name}"; done
  printf 'OUTPUT_CLEANUP_TOKEN=%q\nOUTPUT_REFS_FILE=%q\nOUTPUT_REFS_OWNER_URL_FILE=%q\n' "${PRIVATE}/output-cleanup-token" "${PRIVATE}/output-vault-refs.json" "${PRIVATE}/owner.url"
  printf 'OUTPUT_BASELINE_FILE=%q\n' "${PRIVATE}/output-vault-baseline.json"
} > "${PRIVATE}/cleanup.env"
chmod 0600 "${PRIVATE}/cleanup.env"
DEADLINE_EPOCH=$(( $(date +%s) + ${ORCH_V04_DEADLINE_SECONDS:-3600} ))
setsid nohup "${SUPERVISOR}" "${PRIVATE}/cleanup.env" "$$" "${DEADLINE_EPOCH}" >> "${WORK}/supervisor.log" 2>&1 < /dev/null &
SUPERVISOR_PID=$!
disown "${SUPERVISOR_PID}" 2>/dev/null || true
check "supervisor started pid=${SUPERVISOR_PID} deadline=$(date -u -d "@${DEADLINE_EPOCH}" +%FT%TZ) (own session; cleans the run if this runner dies)"

free_port() { node -e 'const s=require("net").createServer();s.listen(0,"127.0.0.1",()=>{console.log(s.address().port);s.close()})'; }
PORT="$(free_port)"
FORWARD_PORT="$(free_port)"
API="http://127.0.0.1:${PORT}"

# ---- Vault: exact ownership of the Internal account credential objects ----------------------------
printf 'X-Vault-Token: %s\n' "$(tr -d '\n' < "${ORCH_V04_VAULT_TOKEN_FILE}")" > "${PRIVATE}/vault-header"
cp "${ORCH_V04_VAULT_TOKEN_FILE}" "${PRIVATE}/vault-token"
cp "${ORCH_V04_OUTPUT_VAULT_TOKEN_FILE}" "${PRIVATE}/output-vault-token"
cp "${ORCH_V04_OUTPUT_CLEANUP_TOKEN_FILE}" "${PRIVATE}/output-cleanup-token"
chmod 0600 "${PRIVATE}/vault-token" "${PRIVATE}/output-vault-token" "${PRIVATE}/output-cleanup-token" "${PRIVATE}/vault-header"
cmp -s "${PRIVATE}/vault-token" "${PRIVATE}/output-vault-token" && { echo "the connection and output Vault tokens must be distinct files with distinct contents" >&2; exit 2; }
cmp -s "${PRIVATE}/output-vault-token" "${PRIVATE}/output-cleanup-token" && { echo "the output-writer and output-cleanup Vault tokens must be distinct" >&2; exit 2; }
curl -fsS "${VAULT_ADDR_LOCAL}/v1/sys/health" >/dev/null || { echo "Vault ${VAULT_ADDR_LOCAL} is not healthy" >&2; exit 2; }
curl -fsS "${VAULT_ADDR_LOCAL}/v1/sys/health" > "${WORK}/vault-health.json" || true

"${CTL}" v04-output-inventory -mode baseline -vault-addr "${OUTPUT_VAULT_ADDR}" -mount "${OUTPUT_MOUNT}" \
  -token-file "${PRIVATE}/output-cleanup-token" -baseline-file "${PRIVATE}/output-vault-baseline.json" -run-id "${RUN_ID}" >> "${WORK}/checks.txt"

# ---- Run-owned secret-store Vault (the application secret) --------------------------------------------------------
# A SECOND Vault dev server, owned by this run, listens ONLY on the Docker bridge address the Pods reach. Kubernetes auth uses the
# EXISTING reviewer handles (read from the operator's private files, never printed). Nothing on the shared Vault is touched; this
# Vault's values, policies and roles die with its process, which the supervisor stops.
free_port_early() { node -e 'const s=require("net").createServer();s.listen(0,"127.0.0.1",()=>{console.log(s.address().port);s.close()})'; }
SS_BRIDGE="$(node -e 'process.stdout.write(JSON.parse(process.argv[1]).bridgeAddress)' "${SS_JSON}")"
SS_PORT="$(free_port_early)"
SS_ADDR="http://${SS_BRIDGE}:${SS_PORT}"
SS_ROOT="$(head -c 24 /dev/urandom | od -An -tx1 | tr -d ' \n')"
printf 'X-Vault-Token: %s\n' "${SS_ROOT}" > "${PRIVATE}/store-root-header"
chmod 0600 "${PRIVATE}/store-root-header"
VAULT_DEV_ROOT_TOKEN_ID="${SS_ROOT}" setsid "${VAULT_BIN}" server -dev -dev-listen-address "${SS_BRIDGE}:${SS_PORT}" -dev-no-store-token > "${PRIVATE}/store-vault.log" 2>&1 &
SS_PID=$!
unset SS_ROOT
SS_START_TIME="$(sed 's/^.*) //' "/proc/${SS_PID}/stat" | awk '{print $20}')"
[[ "${SS_START_TIME}" =~ ^[0-9]+$ ]] || { echo "the run-owned Vault process identity could not be recorded" >&2; exit 1; }
printf '%s\n%s\n%s\n%s\n%s\n' "${SS_PID}" "${SS_PID}" "$(cat "/proc/${SS_PID}/comm" 2>/dev/null || echo vault)" "${SS_ADDR}" "${SS_START_TIME}" > "${PRIVATE}/aux-secret-vault.pid"
for _ in $(seq 1 60); do curl -fsS "${SS_ADDR}/v1/sys/health" >/dev/null 2>&1 && break; sleep 0.5; done
curl -fsS "${SS_ADDR}/v1/sys/health" >/dev/null || { echo "the run-owned secret-store Vault did not become healthy" >&2; exit 1; }
store_api() { curl -fsS -H @"${PRIVATE}/store-root-header" -X "$1" "${SS_ADDR}/v1/$2" ${3:+-d "$3"} > /dev/null; }
store_api POST sys/mounts/kv '{"type":"kv","options":{"version":"2"}}'
store_api POST sys/auth/kubernetes '{"type":"kubernetes"}'
node -e '
  const fs = require("fs"); const s = JSON.parse(process.argv[1]); const out = process.argv[2];
  fs.writeFileSync(out, JSON.stringify({ kubernetes_host: s.kubernetesHost, kubernetes_ca_cert: fs.readFileSync(s.reviewerCaFile, "utf8"), token_reviewer_jwt: fs.readFileSync(s.reviewerJwtFile, "utf8").trim() }), { mode: 0o600 });
' "${SS_JSON}" "${PRIVATE}/store-auth-config.json"
curl -fsS -H @"${PRIVATE}/store-root-header" -X POST "${SS_ADDR}/v1/auth/kubernetes/config" -d @"${PRIVATE}/store-auth-config.json" > /dev/null
rm -f "${PRIVATE}/store-auth-config.json"
store_api PUT sys/policies/acl/orch-store-admin '{"policy":"path \"auth/token/lookup-self\" { capabilities = [\"read\"] }\npath \"sys/capabilities-self\" { capabilities = [\"update\"] }\npath \"kv/data/orchestrator/apps/*\" { capabilities = [\"create\", \"read\", \"update\"] }\npath \"kv/metadata/orchestrator/apps/*\" { capabilities = [\"read\", \"list\", \"delete\"] }\npath \"sys/policies/acl/orch-*\" { capabilities = [\"create\", \"read\", \"update\", \"delete\"] }\npath \"auth/kubernetes/role/orch-*\" { capabilities = [\"create\", \"read\", \"update\", \"delete\"] }\npath \"auth/kubernetes/config\" { capabilities = [\"read\"] }\n"}'
curl -fsS -H @"${PRIVATE}/store-root-header" -X POST "${SS_ADDR}/v1/auth/token/create" -d '{"policies":["orch-store-admin"],"no_default_policy":true,"ttl":"4h"}' \
  | node -e 'let s="";process.stdin.on("data",d=>s+=d).on("end",()=>{const t=JSON.parse(s).auth?.client_token;if(!t)process.exit(1);process.stdout.write(t)})' > "${PRIVATE}/secret-store.token"
chmod 0600 "${PRIVATE}/secret-store.token"
[[ -s "${PRIVATE}/secret-store.token" ]] || { echo "the secret-store token was not created" >&2; exit 1; }
SECRET_STORE_JSON="$(node -e 'const s=JSON.parse(process.argv[1]);s.backendAddress=s.workloadAddress=process.argv[2];s.tokenFile=process.argv[3];s.mount="kv";s.authMount="kubernetes";process.stdout.write(JSON.stringify(s))' "${SS_JSON}" "${SS_ADDR}" "${PRIVATE}/secret-store.token")"
note "run-owned secret-store Vault on the Docker bridge (pid ${SS_PID}); existing reviewer handles used for Kubernetes auth; the shared Vault is untouched"

# ---- PREFLIGHT (read-only; never a product fallback) ----------------------------------------------
"${CTL}" v03-preflight -kubectl "${KUBECTL}" -kubeconfig "${HOST_KUBECONFIG}" -context "${HOST_CONTEXT}" -namespaces "${NAMESPACE},${COMPANION}" -out "${PRIVATE}/baseline.json" | tee "${WORK}/preflight-k8s.txt"
check "preflight ok: context ${HOST_CONTEXT} reachable, a Ready node, run namespaces ${NAMESPACE} and ${COMPANION} absent, namespace UID baseline recorded (no creation)"
video_require_tools
video_require_fresh_dist "${REPO}"
XDOTOOL="$(video_xdotool)"

# ---- Process groups, cleanup and the final scan ---------------------------------------------------------------------
# The product and the scenario run in their own process groups (setsid) recorded in private pid files; the supervisor and
# v03-cleanup.sh stop exactly those groups. Nothing is destroyed before they are proven stopped.
stop_group_file() { # file: kill the recorded group and wait until it is gone (used for the planned restarts)
  local file="$1" pgid
  [[ -s "${file}" ]] || return 0
  pgid="$(head -n1 "${file}" | tr -dc '0-9')"
  [[ -n "${pgid}" && "${pgid}" -gt 1 ]] || return 0
  kill -TERM -- "-${pgid}" 2>/dev/null || true
  for _ in $(seq 1 80); do pgrep -g "${pgid}" >/dev/null 2>&1 || break; sleep 0.25; done
  kill -KILL -- "-${pgid}" 2>/dev/null || true
  pgrep -g "${pgid}" >/dev/null 2>&1 && return 1
  : > "${file}"
  return 0
}
stop_server() { stop_group_file "${PRIVATE}/server.pid" && note "product server group stopped" || { note "FAILED product server group still alive"; return 1; }; }
run_cleanup() { "${CLEANUP}" "${PRIVATE}/cleanup.env" run; }
SCAN_DONE=0
# scan_public: every retained public file and the final artifacts are scanned for the uploaded credentials, SQL logins, state key and Vault token.
scan_public() {
  local secrets=() f
  for f in "${ORCH_V04_ACCOUNT_KUBECONFIG_FILE}" "${PRIVATE}/console.url" "${PRIVATE}/state.url" "${PRIVATE}/owner.url" "${PRIVATE}/state.key" "${PRIVATE}/vault-header" "${PRIVATE}/vault-token" "${PRIVATE}/output-vault-token" "${PRIVATE}/output-cleanup-token" "${PRIVATE}/app-secrets.txt" "${PRIVATE}/store-root-header" "${PRIVATE}/secret-store.token" "${PG_ADMIN}" "${RESTRICTED}"; do
    [[ -s "${f}" ]] && secrets+=("${f}")
  done
  local list; list="$(IFS=,; echo "${secrets[*]}")"
  "${CTL}" scan -public "${WORK}" -secrets "${list}" 2>&1 | tee -a "${WORK}/checks.txt"
  return "${PIPESTATUS[0]}"
}
cleanup() {
  local code=$? pid
  if [[ -n "${NODE_PID}" ]] && kill -0 "${NODE_PID}" 2>/dev/null; then
    kill -TERM "${NODE_PID}" 2>/dev/null || true
  fi
  if ! run_cleanup; then CLEANUP_FAILED=1; fi
  if [[ -n "${NODE_PID}" ]]; then wait "${NODE_PID}" 2>/dev/null || true; fi
  for pid in "${PIDS[@]}"; do kill "${pid}" 2>/dev/null || true; done
  for pid in "${PIDS[@]}"; do wait "${pid}" 2>/dev/null || true; done
  # A failure path that never reached the normal scan still gets its public files scanned.
  if [[ "${SCAN_DONE}" != 1 ]]; then if scan_public; then SECRET_SCAN_PASSED=1; else code=1; fi; fi
  if [[ "${CLEANUP_FAILED}" == 1 ]]; then
    code=1
    note "CLEANUP NOT PROVEN: the database, state key, recovery artifacts and credentials are kept in ${PRIVATE}"
  elif [[ "${SECRET_SCAN_PASSED}" != 1 ]]; then
    code=1
    note "physical cleanup proven; private scan canaries retained because public secret scan did not pass"
  else
    kill "${SUPERVISOR_PID}" 2>/dev/null || true
    rm -rf "${PRIVATE}"
    [[ ! -e "${PRIVATE}" ]] && PRIVATE_REMOVED=1 || code=1
    note "private credential directory removed (cleanup proven, public artifacts scanned)"
  fi
  if [[ "${code}" == 0 && "${STATUS}" == 0 && "${CLEANUP_FAILED}" == 0 && "${REOPEN_PASSED}" == 1 && "${FINAL_VIDEO_PASSED}" == 1 && "${SECRET_SCAN_PASSED}" == 1 && "${PRIVATE_REMOVED}" == 1 ]]; then
    note "emitting final V04 receipt after proven cleanup/private removal and successful reopen/video/secret checks"
    if ! node -e '
      const fs=require("fs"), crypto=require("crypto"), path=require("path");
      const dir=process.argv[1], out=path.join(dir,"v04-admission-receipt.json");
      const run=JSON.parse(fs.readFileSync(path.join(dir,"run.json"),"utf8"));
      const assertions=JSON.parse(fs.readFileSync(path.join(dir,"assertions.json"),"utf8"));
      const required=JSON.parse(fs.readFileSync(process.argv[2],"utf8"));
      const passed=new Set();
      for(const a of assertions){if(!a.pass||!["ui","private-read","api"].includes(a.source)||passed.has(a.id)) throw new Error("failed, simulated or duplicate assertion");passed.add(a.id);}
      for(const id of required) if(!passed.has(id)) throw new Error("required full-app assertion missing");
      const names=["run.json","source-files.sha256","assertions.json","phases.json","marks.json","assertions.txt","result.txt","V04-raw.mp4","V04-captioned.mp4","captions.srt","captions.ass","ffprobe.json","decode.log","decode-captioned.log","checks.txt","cleanup.txt","cleanup-state.txt"];
      const hashes={}; for(const name of names){const b=fs.readFileSync(path.join(dir,name));hashes[name]=crypto.createHash("sha256").update(b).digest("hex");}
      const receipt={version:2,acceptanceScope:"MAIN_FLOW_2026_10_19",completedAt:new Date().toISOString(),scenario:"V04",runId:run.runId,target:run.target,source:run.source,evidenceSha256:hashes,cleanupState:"CLOSED_CLEAN",privateMaterialRemoved:true,sqlReopenPassed:true,finalVideoPassed:true,finalSecretScanPassed:true,status:"PASS",exitCode:0};
      const tmp=out+".pending"; fs.writeFileSync(tmp,JSON.stringify(receipt,null,2)+"\n",{mode:0o600,flag:"wx"}); fs.renameSync(tmp,out);
    ' "${WORK}" "${ROOT}/test/integration/v2run/testdata/v04-required-assertions.json"; then code=1; note "integrated V04 final admission receipt could not be proven"; fi
  fi
  echo "evidence in ${WORK}"
  echo "run-id=${RUN_ID} status=${code}"
  exit "${code}"
}
trap cleanup EXIT
trap 'exit 129' HUP
trap 'exit 130' INT
trap 'exit 143' TERM

# ---- Disposable PostgreSQL with three separate logins ----------------------------------------------
"${CTL}" pg-provision -admin-url-file "${PG_ADMIN}" -out-dir "${PRIVATE}" | tee -a "${WORK}/cleanup.txt"
CONSOLE_ROLE="$("${CTL}" pg-role -manifest "${PRIVATE}/pg-manifest.json" -kind console)"
STATE_ROLE="$("${CTL}" pg-role -manifest "${PRIVATE}/pg-manifest.json" -kind state)"
ISOLATED=(env -i "PATH=${PATH}" "HOME=${PRIVATE}/home" "TMPDIR=${PRIVATE}/tmp" "LANG=C.UTF-8")
for proxy in HTTPS_PROXY HTTP_PROXY NO_PROXY https_proxy http_proxy no_proxy SSL_CERT_FILE; do
  if [[ -n "${!proxy:-}" ]]; then ISOLATED+=("${proxy}=${!proxy}"); fi
done
"${ISOLATED[@]}" "${ORCH}" -setup-database -database-owner-url-file "${PRIVATE}/owner.url" -console-database-role "${CONSOLE_ROLE}" -state-database-role "${STATE_ROLE}" | tee -a "${WORK}/cleanup.txt"
"${CTL}" keygen -out "${PRIVATE}/state.key"

# Existing delivery cleanup handles use original persisted workload targets; no secret value is serialized.
node -e '
 const fs=require("fs"),path=require("path");
 const [priv,runId,app,namespace,owner,terraform,kubectl,score,git,vault,mount]=process.argv.slice(1);
 const p=n=>path.join(priv,n);
 const h={runId,organizationKey:"acme",applicationKey:app,environmentKey:"staging",namespaceIdentity:namespace,owner,adapters:"kubernetes",terraform,kubectl,scoreK8s:score,git,databaseUrlFile:p("console.url"),stateDatabaseUrlFile:p("state.url"),stateKeyFile:p("state.key"),stateKeyId:"v4-"+runId,recoveryDir:p("recovery"),terraformRoot:p("tfroot"),pluginCacheDir:p("plugin-cache"),moduleCacheDir:p("module-cache"),vaultAddress:vault,vaultMount:mount,connectionTokenFile:p("vault-token"),outputTokenFile:p("output-vault-token"),cleanupBudget:"10m",invocationTimeout:"20m",readinessTimeout:"10m"};
 fs.writeFileSync(p("probe-cleanup-handles.json"),JSON.stringify(h),{mode:0o600,flag:"wx"});
' "${PRIVATE}" "${RUN_ID}" "${APPLICATION_KEY}" "${NAMESPACE}" "${OWNER}" "${TERRAFORM}" "${KUBECTL}" "${SCORE_K8S}" "$(command -v git)" "${VAULT_ADDR_LOCAL}" "${MOUNT}"

# ---- The product: real adapters, cleared environment, fixed port so it can be restarted -------------
ISO_Q="$(printf '%q ' "${ISOLATED[@]}")"
cat > "${PRIVATE}/start-server.sh" <<SCRIPT
#!/usr/bin/env bash
set -euo pipefail
N="\$1"
cd "${WORK}"
setsid ${ISO_Q}"${ORCH}" -addr 127.0.0.1:${PORT} -ui-dir "${REPO}/frontend/dist" \\
  -adapters kubernetes -profile acceptance-v2 -namespace "${NAMESPACE}" -run-id "${RUN_ID}" -owner "${OWNER}" -ttl "${EXEC_SECONDS}s" \\
  -score-k8s "${SCORE_K8S}" -terraform "${TERRAFORM}" -kubectl "${KUBECTL}" -git git \\
  -terraform-root "${PRIVATE}/tfroot" -terraform-plugin-cache "${PRIVATE}/plugin-cache" -terraform-module-cache-dir "${PRIVATE}/module-cache" \\
  -terraform-recovery-dir "${PRIVATE}/recovery" -terraform-state-key-file "${PRIVATE}/state.key" -terraform-state-key-id "v4-${RUN_ID}" \\
  -terraform-state-database-url-file "${PRIVATE}/state.url" -database-url-file "${PRIVATE}/console.url" \\
  -connection-credential-store vault -connection-vault-address "${VAULT_ADDR_LOCAL}" -connection-vault-token-file "${PRIVATE}/vault-token" -connection-vault-mount "${MOUNT}" \\
  -output-secret-store vault -output-secret-vault-address "${VAULT_ADDR_LOCAL}" -output-secret-vault-token-file "${PRIVATE}/output-vault-token" -output-secret-vault-mount "${MOUNT}" \\
  -terraform-invocation-timeout 20m -terraform-readiness-timeout 10m -deploy-deadline 60m \\
  -workload-delivery direct -vault-delivery vso -application-key "${APPLICATION_KEY}" \\
  > "${WORK}/orchestrator-\${N}.log" 2>&1 &
echo \$! > "${PRIVATE}/server.pid"
: > "${PRIVATE}/product-started"
for _ in \$(seq 1 240); do
  curl -fsS "${API}/api/v1/healthz" >/dev/null 2>&1 && exit 0
  kill -0 "\$(cat "${PRIVATE}/server.pid")" 2>/dev/null || break
  sleep 0.25
done
echo "the product did not start; see ${WORK}/orchestrator-\${N}.log" >&2
exit 1
SCRIPT
cat > "${PRIVATE}/restart-server.sh" <<SCRIPT
#!/usr/bin/env bash
set -euo pipefail
pid="\$(cat "${PRIVATE}/server.pid")"
kill -TERM -- "-\${pid}" 2>/dev/null || true
for _ in \$(seq 1 80); do pgrep -g "\${pid}" >/dev/null 2>&1 || break; sleep 0.25; done
kill -KILL -- "-\${pid}" 2>/dev/null || true
pgrep -g "\${pid}" >/dev/null 2>&1 && { echo "the old product process group is still alive" >&2; exit 1; }
echo "product server restarted (old group \${pid})" >> "${WORK}/cleanup.txt"
exec "${PRIVATE}/start-server.sh" 2
SCRIPT
chmod 0700 "${PRIVATE}/start-server.sh" "${PRIVATE}/restart-server.sh"
"${PRIVATE}/start-server.sh" 1 || { echo "the product did not start; see ${WORK}/orchestrator-1.log" >&2; exit 1; }
SERVER_PID="$(<"${PRIVATE}/server.pid")"

# ---- Score documents of the REAL application (public evidence, hashed, with the recorded differences from scores.go) --------
"${CTL}" v04-scores -out "${WORK}/scores" -frontend "${ORCH_V04_IMAGE_FRONTEND_A}" -frontend-b "${ORCH_V04_IMAGE_FRONTEND_B}" -backend "${ORCH_V04_IMAGE_BACKEND}" -worker "${ORCH_V04_IMAGE_WORKER}" \
  -application "${APPLICATION_KEY}" -environment staging -run-id "${RUN_ID}" -ttl "${CACHE_TTL}" | tee -a "${WORK}/checks.txt"
for f in backend worker frontend frontend-b; do [[ -s "${WORK}/scores/${f}.yaml" ]] || { echo "the Score ${f}.yaml was not generated" >&2; exit 1; }; done
SCORES_SHA="$(cat "${WORK}/scores/backend.yaml" "${WORK}/scores/worker.yaml" "${WORK}/scores/frontend.yaml" | sha256sum | cut -d' ' -f1)"
node -e '
  const fs = require("fs");
  const [out, runId, namespace, companion, appKey, account, hostKubeconfig, hostContext, kubectl, ctl, priv, baseline, owned, forward, imgA, digA, imgB, digB, imgBackend, imgWorker, scoreDir, bundle, ttl, secretStoreJson, vaultAddress, vaultMount] = process.argv.slice(1);
  const input = {
    runId, namespace, companionNamespace: companion, applicationKey: appKey, applicationName: "Acceptance Application",
    account: { name: `Lab k8s ${runId}`, kubeconfigFile: account },
    images: { frontendA: { ref: imgA, digest: digA }, frontendB: { ref: imgB, digest: digB }, backend: { ref: imgBackend }, worker: { ref: imgWorker } },
    scoreDir, secretStore: JSON.parse(secretStoreJson), vaultAddress, vaultMount, cache: { ttlSeconds: Number(ttl) },
    kube: { kubectl, kubeconfig: hostKubeconfig, context: hostContext }, v2ctl: ctl, privateDir: priv, baselineFile: baseline, ownedFile: owned,
    forwardPort: Number(forward), servicePort: 8080, bundle,
    timeouts: { deployMs: 45 * 60_000, readyMs: 20 * 60_000, cleanupMs: 20 * 60_000 },
  };
  fs.writeFileSync(out, JSON.stringify(input), { mode: 0o600 });
' "${PRIVATE}/scenario-input.json" "${RUN_ID}" "${NAMESPACE}" "${COMPANION}" "${APPLICATION_KEY}" "${ORCH_V04_ACCOUNT_KUBECONFIG_FILE}" "${HOST_KUBECONFIG}" "${HOST_CONTEXT}" "${KUBECTL}" "${CTL}" "${PRIVATE}" "${PRIVATE}/baseline.json" "${PRIVATE}/owned.json" "${FORWARD_PORT}" "${ORCH_V04_IMAGE_FRONTEND_A}" "${ORCH_V04_IMAGE_FRONTEND_A_DIGEST}" "${ORCH_V04_IMAGE_FRONTEND_B}" "${ORCH_V04_IMAGE_FRONTEND_B_DIGEST}" "${ORCH_V04_IMAGE_BACKEND}" "${ORCH_V04_IMAGE_WORKER}" "${WORK}/scores" "score-k8s-internal-v1" "${CACHE_TTL}" "${SECRET_STORE_JSON}" "${VAULT_ADDR_LOCAL}" "${MOUNT}"
GIT_HEAD="$(git -C "${REPO}" rev-parse HEAD 2>/dev/null || echo unknown)"
# Source provenance covers tracked AND untracked files (the V04 code is mostly new), minus generated and private material.
GIT_DIFF_SHA="$(git -C "${REPO}" diff HEAD 2>/dev/null | sha256sum | cut -d' ' -f1)"
SOURCE_SHA="$(node -e '
  const { execFileSync } = require("child_process");
  const { createHash } = require("crypto");
  const { readFileSync, existsSync, writeFileSync } = require("fs");
  const [repo, out] = process.argv.slice(1);
  const files = execFileSync("git", ["-C", repo, "ls-files", "-co", "--exclude-standard", "-z", "--", "backend", "frontend/src", "frontend/test", "frontend/package.json", "frontend/package-lock.json", "frontend/vite.config.ts", "frontend/tsconfig.json", "frontend/index.html", "frontend/eslint.config.js"], { encoding: "utf8" }).split("\0").filter(Boolean).sort()
    .filter((f) => !/(^|\/)(node_modules|dist|coverage)\//.test(f) && existsSync(`${repo}/${f}`));
  const lines = files.map((f) => `${createHash("sha256").update(readFileSync(`${repo}/${f}`)).digest("hex")}  ${f}`);
  writeFileSync(out, lines.join("\n") + "\n");
  process.stdout.write(createHash("sha256").update(lines.join("\n")).digest("hex"));
' "${REPO}" "${WORK}/source-files.sha256")"
DIST_SHA="$(find "${REPO}/frontend/dist" -type f -print0 | sort -z | xargs -0 sha256sum | sha256sum | cut -d' ' -f1)"
node -e '
  const [out, runId, headed, ns, aux, app, head, diff, dist, orch, scoresSha, sourceSha, scoreDir] = process.argv.slice(1);
  const scores = JSON.parse(require("fs").readFileSync(`${scoreDir}/scores.json`, "utf8"));
  require("fs").writeFileSync(out, JSON.stringify({
    scenario: "V04", target: "k8s-4f", runId, captions: "vi", namespace: ns, companionNamespace: aux, applicationKey: app,
    adapters: "REAL product: planner, dependency graph, pinned Terraform with encrypted PostgreSQL state, durable Vault Secret refs, required pinned score-k8s, real Kubernetes delivery", fake: "none",
    scope: "ENTIRE acceptance application (frontend, backend, worker) with real shared PostgreSQL and Redis on an existing cluster; no frontend-only or deferred coverage",
    source: { gitHead: head, gitDiffSha256: diff, sourceFilesSha256: sourceSha, sourceFilesList: "source-files.sha256 (tracked and untracked, generated and private files excluded)", frontendDistSha256: dist, orchestratorBinarySha256: orch },
    evidenceKinds: { live: "UI and private-read assertions on the real cluster, Vault and PostgreSQL of this run (assertions.json source field: ui | private-read | api)", localOnly: "Go/Vitest tests (graph fixed point, conflicts, cycles, flag combinations, blocked-consumer projection, Score identity) run locally and are not repeated by this run" },
    scores: { combinedSha256: scoresSha, workloads: scores.workloads, differencesFromRepositorySource: scores.changes, preserved: scores.preserved, configurationKeys: scores.config.map((c) => ({ name: c.name, kind: c.kind })) },
    optionalAwsSecretException: "not used: the application secret is a real scoped Vault secret and the config/secret checks are mandatory",
  }, null, 2) + "\n");
' "${WORK}/run.json" "${RUN_ID}" "${HEADED}" "${NAMESPACE}" "${COMPANION}" "${APPLICATION_KEY}" "${GIT_HEAD}" "${GIT_DIFF_SHA}" "${DIST_SHA}" "$(sha256sum "${ORCH}" | cut -d' ' -f1)" "${SCORES_SHA}" "${SOURCE_SHA}" "${WORK}/scores"

# ---- Headed browser (gated) and GO ---------------------------------------------------------------------
video_start_xvfb "${SCREEN}" "${PRIVATE}/aux-xvfb.pid"
RAW="${WORK}/V04-raw.mp4"
GATE="${PRIVATE}/start-gate"
: > "${WORK}/playwright.log"
tail -n +1 -f "${WORK}/playwright.log" &
PIDS+=($!)
DISPLAY="${VIDEO_DISPLAY}" ORCH_E2E_SCREEN="${SCREEN}" ORCH_E2E_XDOTOOL="${XDOTOOL}" ORCH_E2E_START_GATE="${GATE}" ORCH_E2E_RESTART_CMD="${PRIVATE}/restart-server.sh" \
  ORCH_E2E_URL="${API}" ORCH_E2E_EVIDENCE_DIR="${WORK}" ORCH_E2E_INPUT="${PRIVATE}/scenario-input.json" PATH="${PATH}" \
  setsid node "${E2E}/v04-internal-full.mjs" --scenario V04 --headed --captions "${CAPTIONS}" --evidence "${WORK}" > "${WORK}/playwright.log" 2>&1 &
NODE_PID=$!
printf '%s\n' "${NODE_PID}" > "${PRIVATE}/node.pid"
: > "${GATE}"
wait "${NODE_PID}" || STATUS=$?
# Keep the owned group receipt even when the leader exited: its recorder can
# still be alive. The cleanup supervisor must reap the group before fixtures.
if [[ "${STATUS}" != 0 ]]; then
  printf 'FAIL browser process exited with status %s; incomplete evidence is retained\n' "${STATUS}" >> "${WORK}/assertions.txt"
  printf 'FAIL\n' > "${WORK}/result.txt"
fi
sleep 1

# ---- Post-run checks -------------------------------------------------------------------------------------
# Each check records its own verdict; none stops the others, and every artifact is kept also on failure.
node "${E2E}/v04-internal-full.mjs" --verify-evidence "${WORK}" 2>&1 | tee -a "${WORK}/checks.txt" || STATUS=1
if [[ -s "${PRIVATE}/account-key" ]]; then
  # SQL reopen: stop the product and start it again on the SAME database, then read the whole run back through the API.
  if stop_server && "${PRIVATE}/start-server.sh" 3; then
    SERVER_PID="$(<"${PRIVATE}/server.pid")"
    node "${E2E}/v04-reopen.mjs" "${API}" "${PRIVATE}/scenario-input.json" "$(<"${PRIVATE}/account-key")" "${WORK}/assertions.json" 2>&1 | tee -a "${WORK}/checks.txt" && REOPEN_PASSED=1 || STATUS=1
  else
    check "FAIL the product did not restart on the same database"; STATUS=1
  fi
else
  check "FAIL the scenario did not record the account key; reopen check skipped"; STATUS=1
fi
# Physical cleanup, in the proven order: every owned process group stopped, namespaces by recorded UID, Vault by exact keys, then the database.
if run_cleanup; then
  check "cleanup proven: owned process groups stopped; run namespaces absent (exact tags, recorded UIDs; baseline namespaces unchanged); Vault objects of this run's exact account keys deleted and absent; disposable database dropped"
else
  check "FAIL cleanup is NOT proven (see cleanup.txt and CLEANUP-NOT-PROVEN.txt); the database, state key, recovery artifacts and credentials are kept"
  STATUS=1; CLEANUP_FAILED=1
fi

# Captions below the browser area, never over the UI; also after a failure.
CAPTIONED="${WORK}/V04-captioned.mp4"
W="${SCREEN%x*}" H="${SCREEN#*x}"
if [[ -s "${RAW}" && -s "${WORK}/captions.srt" ]]; then
  node "${E2E}/render-captions.mjs" "${WORK}/captions.srt" "${WORK}/captions.ass" "${W}" "$((H + STRIP))"
  ffmpeg -hide_banner -loglevel error -n -i "${RAW}" -vf "pad=${W}:$((H + STRIP)):0:0:black,ass=${WORK}/captions.ass" \
    -r 15 -c:v libx264 -preset veryfast -crf 24 -pix_fmt yuv420p -movflags +faststart "${CAPTIONED}" || { echo "caption burn-in failed" >&2; STATUS=1; }
fi
# The secret scan covers the FINAL public artifacts (captions, run.json, logs and videos), after they exist, and also after a failure.
if [[ "${ORCH_V04_INJECT_FAILURE:-}" == after-success ]]; then check "FAIL injected failure after the scenario (debug)"; STATUS=1; fi
if [[ "${STATUS}" != 0 ]]; then
  echo "scenario V04 failed; video, marks, assertions and checks kept in ${WORK}" >&2
  exit "${STATUS}"
fi
[[ "$(<"${WORK}/result.txt")" == PASS ]] || { echo "result.txt is not PASS" >&2; exit 1; }
video_validate "${RAW}" "${SCREEN}" "${MIN_SECONDS}" 15 signed-out
ffmpeg -hide_banner -v error -xerror -i "${CAPTIONED}" -f null - 2> "${WORK}/decode-captioned.log" || { echo "captioned video does not decode; see ${WORK}/decode-captioned.log" >&2; exit 1; }
[[ ! -s "${WORK}/decode-captioned.log" ]] || { echo "captioned video decode errors" >&2; exit 1; }
[[ "$(ffprobe -v error -select_streams v:0 -show_entries stream=width,height -of csv=p=0 "${CAPTIONED}")" == "${W},$((H + STRIP))" ]] || { echo "captioned video has unexpected size" >&2; exit 1; }
FINAL_VIDEO_PASSED=1
if scan_public; then SCAN_DONE=1; SECRET_SCAN_PASSED=1; else SCAN_DONE=1; exit 1; fi
echo "captioned=${CAPTIONED}"
