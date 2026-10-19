#!/usr/bin/env bash
# V03 integrated acceptance (issue #4): the Orchestrator places the REAL acceptance FRONTEND on an
# existing Kubernetes cluster from Resource Definitions only (ADR-016). One headed, captioned run:
# Internal account upload, Types, existing-cluster / Terraform namespace / workload Definitions with a
# companion namespace, Score import, Preview, Deploy, open the real frontend, update (image and
# configuration), orchestrator restart, an operation after the restart, negatives, removal and the
# verified cleanup of the owned namespaces.
#
#   v03-internal-frontend.sh --headed --captions vi --evidence NEW_PATH
#
# REAL: the Web Console bundle, the product built from this tree (planner -> pinned Terraform with
#   encrypted PostgreSQL state -> required pinned score-k8s -> real Kubernetes delivery), a disposable
#   PostgreSQL database with separate Owner/Console/State logins, the existing scoped Vault for
#   credentials, the existing cluster reached ONLY through the uploaded kubeconfig account, the headed
#   browser and the actual acceptance frontend image. FAKE: nothing. The host kubeconfig handle is read
#   by this runner and its safety net ONLY (read-only assertions, port-forward, guarded delete of the
#   exact run-owned namespaces); the product never sees it and runs with a cleared environment.
# SCOPE: frontend only, by explicit user decision. Backend, worker, PostgreSQL and Redis business
#   functions are DEFERRED to V04; the frontend's dependency rows are asserted FAIL, never green.
#
# Required environment (every path is an externally prepared file; nothing is read from a default
# or retained location, and no private file is copied into the evidence):
#   ORCH_V03_PG_ADMIN_URL_FILE      0600 administrator URL of a disposable PostgreSQL server you own.
#                                   The run creates ONE database and three roles and drops exactly those.
#   ORCH_V03_OUTPUT_VAULT_TOKEN_FILE  0600 DISTINCT output-only Vault token (orchestrator/outputs/*: create,read); the product
#                                   refuses one token for both the connection and the output adapter.
#   ORCH_V03_VAULT_TOKEN_FILE       0600 CONNECTION-ONLY Vault token scoped to orchestrator/connections/* (create,read data;
#                                   read,list,delete metadata); it has no access to orchestrator/outputs/*, which only the
#                                   distinct output token above may use.
#   ORCH_V03_VAULT_ADDR             Vault address, or
#   ORCH_V03_RUNTIME_JSON           public JSON with {"vaultAddress": "...", "vaultMount": "secret"}.
#   ORCH_V03_ACCOUNT_KUBECONFIG_FILE  single-context kubeconfig uploaded through the Console as the Internal account.
#   ORCH_V03_HOST_KUBECONFIG        host kubeconfig handle (runner and safety net only) and
#   ORCH_V03_HOST_CONTEXT           its exact context, for example kind-k8s-4f.
#   ORCH_V03_KUBECTL, ORCH_V03_TERRAFORM (1.9.8), ORCH_V03_SCORE_K8S (0.15.0)   pinned binaries.
#   ORCH_V03_IMAGE_A / ORCH_V03_IMAGE_A_DIGEST and ORCH_V03_IMAGE_B / ORCH_V03_IMAGE_B_DIGEST
#                                   the two frontend images (already loaded into the cluster) and their digests.
#   ORCH_V03_RESTRICTED_KUBECONFIG_FILE  a REAL restricted credential: the live permission negative (V03.08) is
#                                   mandatory, so the run refuses to start without it.
# Optional:
#   ORCH_V03_DEADLINE_SECONDS (5400: the independent supervisor stops and cleans the run after this long),
#   ORCH_V03_VAULT_MOUNT (secret), ORCH_V03_ORG (acme), ORCH_V03_OWNER, ORCH_V03_RUN_ID, ORCH_V03_EXEC_SECONDS (7200),
#   ORCH_VIDEO_SCREEN (1440x900), ORCH_VIDEO_MIN_SECONDS (120).
# Needs: go, node, curl, Xvfb, xdotool, ffmpeg/ffprobe, a built frontend/dist and the Playwright Chromium.
#
# Evidence (NEW_PATH, outside the repository, must not exist; kept also on failure): V03-raw.mp4,
# V03-captioned.mp4 (captions BELOW the browser), captions.srt/.ass, marks.json, phases.json,
# assertions.txt/.json, result.txt, run.json (public inputs, source and binary hashes, Score hashes),
# checks.txt, cleanup.txt, setup-api-calls.txt, deferred-v04.txt, scores/, orchestrator-*.log, failure.png.
# Credentials, state and handles live in NEW_PATH/private (0700). An independent supervisor (own session, survives
# a SIGKILL of this runner) stops every owned process group and then cleans the run with v03-cleanup.sh: namespaces
# by recorded UID and exact tags (atomic on the UID), Vault objects of the run's exact account keys, and only then
# the database. When the cleanup is not proven the database, state key, recovery artifacts and credentials are KEPT
# and CLEANUP-NOT-PROVEN.txt says so. The private directory is removed only after a proven cleanup and a clean scan
# of the final public artifacts.
# Debug / fault checks (each makes the run exit non-zero by design):
#   ORCH_V03_INJECT_FAILURE=after-success   fail after the scenario; video, marks, assertions and cleanup survive.
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
for name in ORCH_V03_PG_ADMIN_URL_FILE ORCH_V03_VAULT_TOKEN_FILE ORCH_V03_OUTPUT_VAULT_TOKEN_FILE ORCH_V03_ACCOUNT_KUBECONFIG_FILE ORCH_V03_HOST_KUBECONFIG; do need_file "${name}"; done
for name in ORCH_V03_KUBECTL ORCH_V03_TERRAFORM ORCH_V03_SCORE_K8S; do need_exec "${name}"; done
for name in ORCH_V03_HOST_CONTEXT ORCH_V03_IMAGE_A ORCH_V03_IMAGE_A_DIGEST ORCH_V03_IMAGE_B ORCH_V03_IMAGE_B_DIGEST; do need_value "${name}"; done
[[ "${ORCH_V03_IMAGE_A}" != "${ORCH_V03_IMAGE_B}" && "${ORCH_V03_IMAGE_A_DIGEST}" != "${ORCH_V03_IMAGE_B_DIGEST}" ]] || { echo "the two frontend images must differ (reference and digest)" >&2; exit 2; }
[[ "${ORCH_V03_IMAGE_A_DIGEST}" =~ ^sha256:[0-9a-f]{64}$ && "${ORCH_V03_IMAGE_B_DIGEST}" =~ ^sha256:[0-9a-f]{64}$ ]] || { echo "image digests must be sha256:<64 hex>" >&2; exit 2; }
need_file ORCH_V03_RESTRICTED_KUBECONFIG_FILE
RESTRICTED="${ORCH_V03_RESTRICTED_KUBECONFIG_FILE}"
VAULT_ADDR_LOCAL="${ORCH_V03_VAULT_ADDR:-}"
MOUNT="${ORCH_V03_VAULT_MOUNT:-secret}"
if [[ -z "${VAULT_ADDR_LOCAL}" && -n "${ORCH_V03_RUNTIME_JSON:-}" ]]; then
  need_file ORCH_V03_RUNTIME_JSON
  VAULT_ADDR_LOCAL="$(node -e 'const j=JSON.parse(require("fs").readFileSync(process.argv[1],"utf8"));process.stdout.write(j.vaultAddress??"")' "${ORCH_V03_RUNTIME_JSON}")"
  if [[ -z "${ORCH_V03_VAULT_MOUNT:-}" ]]; then MOUNT="$(node -e 'const j=JSON.parse(require("fs").readFileSync(process.argv[1],"utf8"));process.stdout.write(j.vaultMount??"secret")' "${ORCH_V03_RUNTIME_JSON}")"; fi
fi
[[ "${VAULT_ADDR_LOCAL}" =~ ^https?://[A-Za-z0-9.:-]+$ ]] || { echo "set ORCH_V03_VAULT_ADDR or ORCH_V03_RUNTIME_JSON with vaultAddress" >&2; exit 2; }
for tool in node go curl ffmpeg ffprobe Xvfb; do command -v "${tool}" >/dev/null || { echo "${tool} is required" >&2; exit 2; }; done
node "${E2E}/v03-internal-frontend.mjs" --check V03 || exit 2

PG_ADMIN="${ORCH_V03_PG_ADMIN_URL_FILE}"
KUBECTL="${ORCH_V03_KUBECTL}" TERRAFORM="${ORCH_V03_TERRAFORM}" SCORE_K8S="${ORCH_V03_SCORE_K8S}"
HOST_KUBECONFIG="${ORCH_V03_HOST_KUBECONFIG}" HOST_CONTEXT="${ORCH_V03_HOST_CONTEXT}"
ORG="${ORCH_V03_ORG:-acme}" OWNER="${ORCH_V03_OWNER:-poc-v03}" EXEC_SECONDS="${ORCH_V03_EXEC_SECONDS:-7200}"
SUFFIX="$(head -c 3 /dev/urandom | od -An -tx1 | tr -d ' \n')"
RUN_ID="${ORCH_V03_RUN_ID:-v03-$(date -u +%m%d%H%M%S)-${SUFFIX}}"
[[ "${RUN_ID}" =~ ^[a-z0-9-]{6,40}$ ]] || { echo "ORCH_V03_RUN_ID must match [a-z0-9-]{6,40}" >&2; exit 2; }
NAMESPACE="orch-v3-${RUN_ID}"
COMPANION="${NAMESPACE}-aux"
APPLICATION_KEY="acc-${RUN_ID}"
[[ ${#COMPANION} -le 63 ]] || { echo "the run namespaces would exceed 63 characters; use a shorter ORCH_V03_RUN_ID" >&2; exit 2; }

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
MIN_SECONDS="${ORCH_VIDEO_MIN_SECONDS:-120}"
STRIP=96
PIDS=() SERVER_PID="" NODE_PID="" PG_PROVISIONED=0 PG_DROP_CLEAN=1 STATUS=0 CLEANUP_FAILED=0 FINISHED=0
# shellcheck source=video-lib.sh
source "${ROOT}/test/integration/video-lib.sh"
: > "${WORK}/cleanup.txt"
: > "${WORK}/checks.txt"
note() { printf '%s\n' "$*" >> "${WORK}/cleanup.txt"; }
check() { printf '%s\n' "$*" | tee -a "${WORK}/checks.txt"; }

# ---- Operator tools, built from this tree into the private directory ----------------------------
(cd "${ROOT}" && go build -o "${PRIVATE}/bin/v2ctl" ./test/integration/v2ctl && go build -o "${PRIVATE}/bin/orchestrator" ./cmd/orchestrator)
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
  for name in WORK PRIVATE CTL KUBECTL HOST_KUBECONFIG HOST_CONTEXT RUN_ID ORG VAULT_ADDR_LOCAL MOUNT PG_ADMIN; do printf '%s=%q\n' "${name}" "${!name}"; done
} > "${PRIVATE}/cleanup.env"
chmod 0600 "${PRIVATE}/cleanup.env"
DEADLINE_EPOCH=$(( $(date +%s) + ${ORCH_V03_DEADLINE_SECONDS:-5400} ))
setsid nohup "${SUPERVISOR}" "${PRIVATE}/cleanup.env" "$$" "${DEADLINE_EPOCH}" >> "${WORK}/supervisor.log" 2>&1 < /dev/null &
SUPERVISOR_PID=$!
disown "${SUPERVISOR_PID}" 2>/dev/null || true
check "supervisor started pid=${SUPERVISOR_PID} deadline=$(date -u -d "@${DEADLINE_EPOCH}" +%FT%TZ) (own session; cleans the run if this runner dies)"

free_port() { node -e 'const s=require("net").createServer();s.listen(0,"127.0.0.1",()=>{console.log(s.address().port);s.close()})'; }
PORT="$(free_port)"
FORWARD_PORT="$(free_port)"
API="http://127.0.0.1:${PORT}"

# ---- Vault: exact ownership of the Internal account credential objects ----------------------------
printf 'X-Vault-Token: %s\n' "$(tr -d '\n' < "${ORCH_V03_VAULT_TOKEN_FILE}")" > "${PRIVATE}/vault-header"
cp "${ORCH_V03_VAULT_TOKEN_FILE}" "${PRIVATE}/vault-token"
cp "${ORCH_V03_OUTPUT_VAULT_TOKEN_FILE}" "${PRIVATE}/output-vault-token"
chmod 0600 "${PRIVATE}/vault-token" "${PRIVATE}/output-vault-token" "${PRIVATE}/vault-header"
cmp -s "${PRIVATE}/vault-token" "${PRIVATE}/output-vault-token" && { echo "the connection and output Vault tokens must be distinct files with distinct contents" >&2; exit 2; }
curl -fsS "${VAULT_ADDR_LOCAL}/v1/sys/health" >/dev/null || { echo "Vault ${VAULT_ADDR_LOCAL} is not healthy" >&2; exit 2; }
curl -fsS "${VAULT_ADDR_LOCAL}/v1/sys/health" > "${WORK}/vault-health.json" || true

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
  for f in "${ORCH_V03_ACCOUNT_KUBECONFIG_FILE}" "${PRIVATE}/unreachable.kubeconfig" "${PRIVATE}/console.url" "${PRIVATE}/state.url" "${PRIVATE}/owner.url" "${PRIVATE}/state.key" "${PRIVATE}/vault-header" "${PRIVATE}/vault-token" "${PRIVATE}/output-vault-token" "${PG_ADMIN}" "${RESTRICTED}"; do
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
  if [[ "${SCAN_DONE}" != 1 ]] && ! scan_public; then code=1; fi
  if [[ "${CLEANUP_FAILED}" == 1 ]]; then
    code=1
    note "CLEANUP NOT PROVEN: the database, state key, recovery artifacts and credentials are kept in ${PRIVATE}"
  else
    kill "${SUPERVISOR_PID}" 2>/dev/null || true
    rm -rf "${PRIVATE}"
    note "private credential directory removed (cleanup proven, public artifacts scanned)"
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
  -terraform-recovery-dir "${PRIVATE}/recovery" -terraform-state-key-file "${PRIVATE}/state.key" -terraform-state-key-id "v3-${RUN_ID}" \\
  -terraform-state-database-url-file "${PRIVATE}/state.url" -database-url-file "${PRIVATE}/console.url" \\
  -connection-credential-store vault -connection-vault-address "${VAULT_ADDR_LOCAL}" -connection-vault-token-file "${PRIVATE}/vault-token" -connection-vault-mount "${MOUNT}" \\
  -output-secret-store vault -output-secret-vault-address "${VAULT_ADDR_LOCAL}" -output-secret-vault-token-file "${PRIVATE}/output-vault-token" -output-secret-vault-mount "${MOUNT}" \\
  -workload-delivery direct -application-key "${APPLICATION_KEY}" \\
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

# ---- Score documents of the REAL frontend (public evidence, hashed) and the scenario input -----------
BACKEND_A="http://acceptance-backend.v04-deferred.invalid:8080"
BACKEND_B="http://acceptance-backend-b.v04-deferred.invalid:8081"
write_score() { # file image   (configuration is Environment variables PORT and BACKEND_URL that the Score only references)
  cat > "$1" <<YAML
apiVersion: score.dev/v1b1
metadata:
  name: frontend
containers:
  main:
    image: $2
    variables:
      PORT: \${resources.env.PORT}
      BACKEND_URL: \${resources.env.BACKEND_URL}
    readinessProbe:
      path: /healthz
      port: 8080
    livenessProbe:
      path: /healthz
      port: 8080
service:
  ports:
    http:
      port: 8080
      targetPort: 8080
resources:
  env:
    type: environment
YAML
}
write_score "${WORK}/scores/frontend-a.yaml" "${ORCH_V03_IMAGE_A}"
write_score "${WORK}/scores/frontend-b.yaml" "${ORCH_V03_IMAGE_B}"
SHA_A="$(sha256sum "${WORK}/scores/frontend-a.yaml" | cut -d' ' -f1)"
SHA_B="$(sha256sum "${WORK}/scores/frontend-b.yaml" | cut -d' ' -f1)"
# A kubeconfig whose endpoint cannot be reached: the live "cluster not ready" negative (nothing is stored).
cat > "${PRIVATE}/unreachable.kubeconfig" <<KUBE
apiVersion: v1
kind: Config
clusters:
- name: unreachable
  cluster:
    server: https://127.0.0.1:1
    insecure-skip-tls-verify: true
contexts:
- name: unreachable
  context: {cluster: unreachable, user: nobody}
current-context: unreachable
users:
- name: nobody
  user: {token: v03-unreachable-$(head -c 12 /dev/urandom | od -An -tx1 | tr -d ' \n')}
KUBE
chmod 0600 "${PRIVATE}/unreachable.kubeconfig"
node -e '
  const fs = require("fs");
  const [out, runId, namespace, companion, appKey, account, restricted, unreachable, hostKubeconfig, hostContext, kubectl, ctl, priv, baseline, owned, forward, a, aDigest, b, bDigest, urlA, urlB, scoreA, scoreB, shaA, shaB, bundle] = process.argv.slice(1);
  const input = {
    runId, namespace, companionNamespace: companion, applicationKey: appKey, applicationName: "Acceptance Application",
    account: { name: `Lab k8s ${runId}`, kubeconfigFile: account },
    unreachable: { name: `Unreachable ${runId}`, kubeconfigFile: unreachable },
    restricted: restricted ? { name: `Restricted ${runId}`, kubeconfigFile: restricted } : null,
    negativeApplication: { name: `Negative ${runId}` },
    images: { a: { ref: a, digest: aDigest }, b: { ref: b, digest: bDigest } },
    backendUrl: { a: urlA, b: urlB }, scoreFiles: { a: scoreA, b: scoreB }, scoreHashes: { a: shaA, b: shaB },
    kube: { kubectl, kubeconfig: hostKubeconfig, context: hostContext }, v2ctl: ctl, privateDir: priv, baselineFile: baseline, ownedFile: owned,
    forwardPort: Number(forward), servicePort: 8080, bundle,
    timeouts: { deployMs: 25 * 60_000, cleanupMs: 15 * 60_000 },
  };
  fs.writeFileSync(out, JSON.stringify(input), { mode: 0o600 });
' "${PRIVATE}/scenario-input.json" "${RUN_ID}" "${NAMESPACE}" "${COMPANION}" "${APPLICATION_KEY}" "${ORCH_V03_ACCOUNT_KUBECONFIG_FILE}" "${RESTRICTED}" "${PRIVATE}/unreachable.kubeconfig" "${HOST_KUBECONFIG}" "${HOST_CONTEXT}" "${KUBECTL}" "${CTL}" "${PRIVATE}" "${PRIVATE}/baseline.json" "${PRIVATE}/owned.json" "${FORWARD_PORT}" "${ORCH_V03_IMAGE_A}" "${ORCH_V03_IMAGE_A_DIGEST}" "${ORCH_V03_IMAGE_B}" "${ORCH_V03_IMAGE_B_DIGEST}" "${BACKEND_A}" "${BACKEND_B}" "${WORK}/scores/frontend-a.yaml" "${WORK}/scores/frontend-b.yaml" "${SHA_A}" "${SHA_B}" "score-k8s-internal-v1"
GIT_HEAD="$(git -C "${REPO}" rev-parse HEAD 2>/dev/null || echo unknown)"
# Source provenance covers tracked AND untracked files (the V03 code is mostly new), minus generated and private material.
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
  const [out, runId, headed, ns, aux, app, head, diff, dist, orch, shaA, shaB, imgA, digA, imgB, digB, restricted, sourceSha] = process.argv.slice(1);
  require("fs").writeFileSync(out, JSON.stringify({
    scenario: "V03", runId, captions: "vi", namespace: ns, companionNamespace: aux, applicationKey: app,
    adapters: "REAL product: planner, pinned Terraform with encrypted PostgreSQL state, required pinned score-k8s, real Kubernetes delivery", fake: "none",
    scope: "real acceptance FRONTEND only; backend/worker/PostgreSQL/Redis deferred to V04",
    source: { gitHead: head, gitDiffSha256: diff, sourceFilesSha256: sourceSha, sourceFilesList: "source-files.sha256 (tracked and untracked, generated and private files excluded)", frontendDistSha256: dist, orchestratorBinarySha256: orch },
    evidenceKinds: { live: "UI and private-read assertions on the real cluster, Vault and PostgreSQL of this run (assertions.json source field: ui | private-read | api)", localOnly: "Go tests (wrong account kind, organization, status, version, wrong-target apply) run locally and are not repeated by this run" },
    scores: { a: shaA, b: shaB }, images: { a: { ref: imgA, digest: digA }, b: { ref: imgB, digest: digB } }, restrictedCredentialNegative: true,
  }, null, 2) + "\n");
' "${WORK}/run.json" "${RUN_ID}" "${HEADED}" "${NAMESPACE}" "${COMPANION}" "${APPLICATION_KEY}" "${GIT_HEAD}" "${GIT_DIFF_SHA}" "${DIST_SHA}" "$(sha256sum "${ORCH}" | cut -d' ' -f1)" "${SHA_A}" "${SHA_B}" "${ORCH_V03_IMAGE_A}" "${ORCH_V03_IMAGE_A_DIGEST}" "${ORCH_V03_IMAGE_B}" "${ORCH_V03_IMAGE_B_DIGEST}" "1" "${SOURCE_SHA}"

# ---- Headed browser (gated) and GO ---------------------------------------------------------------------
video_start_xvfb "${SCREEN}" "${PRIVATE}/aux-xvfb.pid"
RAW="${WORK}/V03-raw.mp4"
GATE="${PRIVATE}/start-gate"
: > "${WORK}/playwright.log"
tail -n +1 -f "${WORK}/playwright.log" &
PIDS+=($!)
DISPLAY="${VIDEO_DISPLAY}" ORCH_E2E_SCREEN="${SCREEN}" ORCH_E2E_XDOTOOL="${XDOTOOL}" ORCH_E2E_START_GATE="${GATE}" ORCH_E2E_RESTART_CMD="${PRIVATE}/restart-server.sh" \
  ORCH_E2E_URL="${API}" ORCH_E2E_EVIDENCE_DIR="${WORK}" ORCH_E2E_INPUT="${PRIVATE}/scenario-input.json" PATH="${PATH}" \
  setsid node "${E2E}/v03-internal-frontend.mjs" --scenario V03 --headed --captions "${CAPTIONS}" --evidence "${WORK}" > "${WORK}/playwright.log" 2>&1 &
NODE_PID=$!
printf '%s\n' "${NODE_PID}" > "${PRIVATE}/node.pid"
: > "${GATE}"
wait "${NODE_PID}" || STATUS=$?
: > "${PRIVATE}/node.pid"
NODE_PID=""
sleep 1

# ---- Post-run checks -------------------------------------------------------------------------------------
# Each check records its own verdict; none stops the others, and every artifact is kept also on failure.
ORCH_V03_RESTRICTED=1 node "${E2E}/v03-internal-frontend.mjs" --verify-evidence "${WORK}" 2>&1 | tee -a "${WORK}/checks.txt" || STATUS=1
[[ -s "${WORK}/deferred-v04.txt" ]] || { check "FAIL deferred-v04.txt is missing"; STATUS=1; }
if [[ -s "${PRIVATE}/account-key" ]]; then
  # SQL reopen: stop the product and start it again on the SAME database, then read the whole run back through the API.
  if stop_server && "${PRIVATE}/start-server.sh" 3; then
    SERVER_PID="$(<"${PRIVATE}/server.pid")"
    node "${E2E}/v03-reopen.mjs" "${API}" "${PRIVATE}/scenario-input.json" "$(<"${PRIVATE}/account-key")" "${WORK}/assertions.json" 2>&1 | tee -a "${WORK}/checks.txt" || STATUS=1
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
CAPTIONED="${WORK}/V03-captioned.mp4"
W="${SCREEN%x*}" H="${SCREEN#*x}"
if [[ -s "${RAW}" && -s "${WORK}/captions.srt" ]]; then
  node "${E2E}/render-captions.mjs" "${WORK}/captions.srt" "${WORK}/captions.ass" "${W}" "$((H + STRIP))"
  ffmpeg -hide_banner -loglevel error -n -i "${RAW}" -vf "pad=${W}:$((H + STRIP)):0:0:black,ass=${WORK}/captions.ass" \
    -r 15 -c:v libx264 -preset veryfast -crf 24 -pix_fmt yuv420p -movflags +faststart "${CAPTIONED}" || { echo "caption burn-in failed" >&2; STATUS=1; }
fi
# The secret scan covers the FINAL public artifacts (captions, run.json, logs and videos), after they exist, and also after a failure.
if scan_public; then SCAN_DONE=1; else SCAN_DONE=1; STATUS=1; fi
if [[ "${ORCH_V03_INJECT_FAILURE:-}" == after-success ]]; then check "FAIL injected failure after the scenario (debug)"; STATUS=1; fi
if [[ "${STATUS}" != 0 ]]; then
  echo "scenario V03 failed; video, marks, assertions and checks kept in ${WORK}" >&2
  exit "${STATUS}"
fi
[[ "$(<"${WORK}/result.txt")" == PASS ]] || { echo "result.txt is not PASS" >&2; exit 1; }
video_validate "${RAW}" "${SCREEN}" "${MIN_SECONDS}" 15 signed-out
ffmpeg -hide_banner -v error -xerror -i "${CAPTIONED}" -f null - 2> "${WORK}/decode-captioned.log" || { echo "captioned video does not decode; see ${WORK}/decode-captioned.log" >&2; exit 1; }
[[ ! -s "${WORK}/decode-captioned.log" ]] || { echo "captioned video decode errors" >&2; exit 1; }
[[ "$(ffprobe -v error -select_streams v:0 -show_entries stream=width,height -of csv=p=0 "${CAPTIONED}")" == "${W},$((H + STRIP))" ]] || { echo "captioned video has unexpected size" >&2; exit 1; }
echo "captioned=${CAPTIONED}"
