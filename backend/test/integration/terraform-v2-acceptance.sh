#!/usr/bin/env bash
# Real Terraform v2 acceptance runner (M04 I04-10): V2-A01/E01/E02 on k8s-4f
# and V2-A02/A03 on AWS after the I04-11 gate.
#
#   terraform-v2-acceptance.sh --config PRIVATE_CONFIG.json --headed --captions vi
#
# WHAT IS REAL: the product binary built from this tree (planner -> pinned
# Terraform 1.9.8 with ENCRYPTED PostgreSQL state and claims on the State login
# -> typed outputs and DURABLE scoped Vault Secret refs -> the REQUIRED pinned
# score-k8s 0.15.0 -> real Kubernetes delivery and readiness), a disposable
# PostgreSQL with three separate logins (Owner, Console, State), a run-owned
# Vault dev server holding the scoped Connection credentials and the output
# secrets, the headed browser and the Web Console, and the acceptance app on
# the real k8s-4f cluster. The Connection is registered through the Console with
# a single-context kubeconfig; the product never reads the host kubeconfig,
# KUBECONFIG, or an AWS profile (its environment is cleared).
# WHAT IS FAKE: nothing. There is no fake provider, executor, renderer or
# interception. The host kubeconfig handle is used ONLY by this runner and the
# cleanup supervisor (preflight, read-only technical assertions, port-forward,
# safety-net cleanup), never by the product.
#
# The private configuration is a 0600 JSON file OUTSIDE the repository (see
# backend/test/integration/v2run/config.go; ORCH_V2_CONFIG_EXAMPLE in the I04-10
# handoff). Evidence goes to the NEW directory named by the configuration
# (0700, outside Git, kept also on failure); credentials, states and handles
# stay under evidence/private (0700) and are removed only when the independent
# supervisor reports CLEAN. Public logs, captions and videos are scanned.
set -euo pipefail
umask 077
# xdotool types the Vietnamese option names of native selects; it needs a UTF-8 locale.
export LC_ALL=C.UTF-8

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
REPO="$(cd "${ROOT}/.." && pwd)"
E2E="${REPO}/frontend/test/e2e"
usage() { echo "usage: $0 --config PRIVATE_CONFIG.json [--prepare-only (AWS)] --headed --captions vi" >&2; exit 2; }

CONFIG="" HEADED=0 CAPTIONS="" PREPARE_ONLY=0
while (( $# )); do
  case "$1" in
    --config) [[ $# -ge 2 ]] || usage; CONFIG="$2"; shift 2 ;;
    --headed) HEADED=1; shift ;;
    --prepare-only) PREPARE_ONLY=1; shift ;;
    --captions) [[ $# -ge 2 ]] || usage; CAPTIONS="$2"; shift 2 ;;
    *) usage ;;
  esac
done
[[ -n "${CONFIG}" && ( "${PREPARE_ONLY}" == 1 || ( "${HEADED}" == 1 && "${CAPTIONS}" == vi ) ) ]] || usage
REQUIRED_TOOLS=(node go ffmpeg ffprobe Xvfb xdotool curl)
if [[ "${PREPARE_ONLY}" == 1 ]]; then REQUIRED_TOOLS=(go); fi
for tool in "${REQUIRED_TOOLS[@]}"; do command -v "${tool}" >/dev/null || { echo "${tool} is required" >&2; exit 2; }; done

# ---- Operator tools, built outside the repository ---------------------------
TOOLS="$(mktemp -d)"
chmod 0700 "${TOOLS}"
trap 'rm -rf "${TOOLS}"' EXIT   # replaced by the full cleanup trap once the run directory exists
(cd "${ROOT}" && go build -o "${TOOLS}/v2ctl" ./test/integration/v2ctl && go build -o "${TOOLS}/v2supervisor" ./test/integration/v2supervisor && go build -o "${TOOLS}/orchestrator" ./cmd/orchestrator && go build -o "${TOOLS}/v2cleanup" ./test/integration/v2cleanup)
CTL="${TOOLS}/v2ctl"
cfg() { "${CTL}" config-get -config "${CONFIG}" -field "$1"; }

"${CTL}" validate-config -fresh -config "${CONFIG}" > /dev/null
SCENARIO="$(cfg scenario)"
TARGET="$(cfg target)"
IS_AWS=0
if [[ "${TARGET}" == aws ]]; then
  IS_AWS=1
  # AWS: the read-only gates run FIRST for every AWS invocation: pinned binaries, STS account/region/principal, explicit /32 egress, pod capacity,
  # permission simulation, an empty run tag and the I04-11 gate (A01, E01, E02 passed and closed clean). Without --prepare-only the run then
  # continues; nothing is created before these pass, the estimate is complete and the supervisor is armed.
  "${CTL}" verify-binaries -config "${CONFIG}"
  AWS_PREFLIGHT="$("${CTL}" aws-preflight -config "${CONFIG}")"
  printf '%s\n' "${AWS_PREFLIGHT}"
  "${CTL}" aws-gate -config "${CONFIG}"
  if [[ "${PREPARE_ONLY}" == 1 ]]; then
    echo "AWS preparation verified (read-only); no resource was created"
    rm -rf "${TOOLS}"
    exit 0
  fi
  [[ "$(cfg awsEstimateFile)" != "" ]] || { echo "aws.estimateFile is required" >&2; exit 2; }
fi
RUN_ID="$(cfg runId)"
EVIDENCE="$(cfg evidenceDir)"
OWNER="$(cfg owner)"
EXEC_SECONDS="$(cfg executionSeconds)"
CLEANUP_SECONDS="$(cfg cleanupSeconds)"
if [[ "${IS_AWS}" == 1 ]]; then NAMESPACE="orch-v2-test-$(cfg runId)"; else NAMESPACE="$(cfg namespace)"; fi
APPLICATION_KEY="$(cfg applicationKey)"
SECRET_STORE_TOKEN="$(cfg secretStoreTokenFile)"
SECRET_STORE_JSON="$(cfg secretStoreJson)"
KUBECTL="$(cfg kubectl)"
if [[ "${IS_AWS}" == 1 ]]; then KUBECONFIG_HANDLE="" KUBE_CONTEXT=""; else KUBECONFIG_HANDLE="$(cfg kubeconfig)"; KUBE_CONTEXT="$(cfg kubeContext)"; fi
TERRAFORM="$(cfg terraform)"
SCORE_K8S="$(cfg scoreK8s)"
VAULT_BIN="$(cfg vaultBin)"
VAULT_DELIVERY="$(cfg vaultDelivery)"
PG_ADMIN="$(cfg pgAdminUrlFile)"

# No new run while a previous one is dirty.
"${CTL}" dirty -root "$(dirname "${EVIDENCE}")"

mkdir -m 0700 "${EVIDENCE}"
# The immutable run marker exists before ANY create: a run whose cleanup is not verified stays dirty even if its private files are lost.
"${CTL}" run-marker -dir "${EVIDENCE}" -run "${RUN_ID}" -target "${TARGET}" -state STARTED
PRIVATE="${EVIDENCE}/private"
mkdir -m 0700 "${PRIVATE}" "${PRIVATE}/recovery" "${PRIVATE}/tfroot" "${PRIVATE}/plugin-cache" "${PRIVATE}/module-cache" "${PRIVATE}/home" "${PRIVATE}/tmp" "${PRIVATE}/bin"
WORK="${EVIDENCE}"
cp "${TOOLS}/v2ctl" "${TOOLS}/v2supervisor" "${TOOLS}/orchestrator" "${TOOLS}/v2cleanup" "${PRIVATE}/bin/"
CTL="${PRIVATE}/bin/v2ctl"
SUPERVISOR="${PRIVATE}/bin/v2supervisor"
ORCH="${PRIVATE}/bin/orchestrator"
CLEANUP_BIN="${PRIVATE}/bin/v2cleanup"
rm -rf "${TOOLS}"
MANIFEST="${PRIVATE}/manifest.json"
SCREEN="${ORCH_VIDEO_SCREEN:-1440x900}"
MIN_SECONDS="${ORCH_VIDEO_MIN_SECONDS:-60}"
STRIP=96
export ORCH_VIDEO_DIR="${WORK}"
PIDS=() SERVER_PID="" VAULT_PID="" VAULT2_PID="" NODE_PID="" PG_PROVISIONED=0 PG_DROP_CLEAN=1 SUPERVISED=0 FINISHED=0 STATUS=0 CLEANUP_PROVEN=0 CLOSED_CLEAN=0 LOCAL_STOP_CLEAN=1 PRIVATE_CLEAN=1
# shellcheck source=video-lib.sh
source "${ROOT}/test/integration/video-lib.sh"

: > "${WORK}/cleanup.txt"
note() { printf '%s\n' "$*" >> "${WORK}/cleanup.txt"; }
check() { printf '%s\n' "$*" | tee -a "${WORK}/checks.txt"; }
: > "${WORK}/checks.txt"

owned_child_stopped() {
  local child="$1" state
  if ! kill -0 "${child}" 2>/dev/null; then return 0; fi
  state="$(ps -o stat= -p "${child}" 2>/dev/null | awk '{print substr($1,1,1)}')"
  [[ "${state}" == Z ]]
}
stop_owned_child() {
  local child="$1" iteration
  kill -TERM "${child}" 2>/dev/null || true
  for iteration in $(seq 1 40); do
    if owned_child_stopped "${child}"; then wait "${child}" 2>/dev/null || true; return 0; fi
    sleep 0.25
  done
  kill -KILL "${child}" 2>/dev/null || true
  for iteration in $(seq 1 20); do
    if owned_child_stopped "${child}"; then wait "${child}" 2>/dev/null || true; return 0; fi
    sleep 0.25
  done
  disown "${child}" 2>/dev/null || true
  return 1
}

# The cluster-side cleanup is the independent supervisor's job. This trap stops
# only local fixtures and, if the run ends early, asks the supervisor to run the
# same verified cleanup. State and recovery evidence stay when it needs attention.
cleanup() {
  local code=$? pid
  if [[ "${SUPERVISED}" == 1 && "${FINISHED}" == 0 ]]; then
    "${CTL}" supervisor-finish -private "${PRIVATE}" 2>/dev/null || true
    FINISHED=1
  fi
  if [[ -n "${NODE_PID}" ]] && kill -0 "${NODE_PID}" 2>/dev/null; then
    if ! stop_owned_child "${NODE_PID}"; then LOCAL_STOP_CLEAN=0; code=1; note "headed browser process did not stop within the bounded cleanup window"; fi
  fi
  if [[ "${SUPERVISED}" == 1 ]]; then
    if SUMMARY="$("${CTL}" supervisor-wait -private "${PRIVATE}" -timeout "${CLEANUP_SECONDS}s" 2>&1)"; then
      note "supervisor cleanup: CLEAN"
    else
      note "supervisor cleanup NEEDS ATTENTION: state and recovery evidence are kept in ${PRIVATE}"
      code=1; KEEP_PRIVATE=1
    fi
    printf '%s\n' "${SUMMARY:-}" | grep -v -i -E 'token|password|secret' > "${WORK}/cleanup-summary.json" 2>/dev/null || true
    if [[ "${CLEANUP_PROVEN}" != 1 ]]; then
      # A supervisor CLEAN result alone does not prove the independent physical
      # absence/sentinel/SLR checks completed. Retain state and its dependencies
      # before touching the disposable database or run-owned Vaults.
      code=1; KEEP_PRIVATE=1
      note "external cleanup proof was not completed by the runner; private state and supporting fixtures are retained"
    fi
  fi
  if [[ -n "${SERVER_PID}" ]] && kill -0 "${SERVER_PID}" 2>/dev/null; then
    if stop_owned_child "${SERVER_PID}"; then note "orchestrator stopped"; else LOCAL_STOP_CLEAN=0; code=1; note "orchestrator did not stop within the bounded cleanup window"; fi
  fi
  for pid in "${PIDS[@]}"; do kill "${pid}" 2>/dev/null || true; done
  for pid in "${PIDS[@]}"; do wait "${pid}" 2>/dev/null || true; done
  if [[ "${PG_PROVISIONED}" == 1 && "${KEEP_PRIVATE:-0}" == 1 ]]; then
    # Encrypted Terraform state and claims are the ownership evidence: never drop them while cleanup is unverified.
    note "disposable database KEPT (encrypted state and claims are evidence); drop it with: v2ctl pg-drop -admin-url-file <admin> -manifest ${PRIVATE}/pg-manifest.json"
  elif [[ "${PG_PROVISIONED}" == 1 ]]; then
    if "${CTL}" pg-drop -admin-url-file "${PG_ADMIN}" -manifest "${PRIVATE}/pg-manifest.json" >> "${WORK}/cleanup.txt" 2>&1; then note "disposable database and roles dropped"; else note "FAILED to drop the disposable database; pg-manifest.json kept"; code=1; PG_DROP_CLEAN=0; KEEP_PRIVATE=1; fi
  fi
  if [[ -n "${VAULT_PID}" ]]; then
    if [[ "${KEEP_PRIVATE:-0}" == 1 ]]; then
      # Output-secret objects are needed to continue a manual destroy.
      disown "${VAULT_PID}" 2>/dev/null || true
      LOCAL_STOP_CLEAN=0
      note "run-owned Vault dev server KEPT RUNNING for manual continuation: pid=${VAULT_PID} address=${VAULT_ADDR_LOCAL}; stop it with kill ${VAULT_PID} after cleanup completes"
    else
      if stop_owned_child "${VAULT_PID}"; then note "vault dev server stopped"; else LOCAL_STOP_CLEAN=0; code=1; note "Vault dev server did not stop within the bounded cleanup window"; fi
    fi
  fi
  # Keep the scoped Vault alive whenever state or handles must be retained for
  # manual continuation. It is run-owned and contains only this run's values.
  if [[ -n "${VAULT2_PID}" ]]; then
    if [[ "${KEEP_PRIVATE:-0}" == 1 ]]; then
      disown "${VAULT2_PID}" 2>/dev/null || true
      LOCAL_STOP_CLEAN=0
      note "run-owned secret-store Vault KEPT RUNNING for manual continuation: pid=${VAULT2_PID}"
    else
      if stop_owned_child "${VAULT2_PID}"; then note "run-owned secret-store Vault stopped (its values, policies and roles are gone)"; else LOCAL_STOP_CLEAN=0; code=1; note "run-owned secret-store Vault did not stop within the bounded cleanup window"; fi
    fi
  fi
  if [[ "${LOCAL_STOP_CLEAN}" != 1 ]]; then code=1; KEEP_PRIVATE=1; note "a run-owned local process is not proven stopped; private handles are retained and the run remains dirty"; fi
  if [[ "${KEEP_PRIVATE:-0}" == 1 ]]; then
    note "private directory kept (needs attention): ${PRIVATE}; the run stays DIRTY until the supervisor reports CLEAN or an operator closes it"
  else
    if [[ "${SUPERVISED}" == 1 && "${CLEANUP_PROVEN}" == 1 && "${PG_DROP_CLEAN}" == 1 ]]; then
      if "${CTL}" run-marker -dir "${EVIDENCE}" -run "${RUN_ID}" -target "${TARGET}" -state CLOSED_CLEAN >> "${WORK}/cleanup.txt" 2>&1; then CLOSED_CLEAN=1; else note "run marker could not be closed; the run stays dirty"; code=1; KEEP_PRIVATE=1; fi
    else
      note "cleanup was not fully proven (supervisor, physical absence, private database); run marker remains dirty"
      KEEP_PRIVATE=1
    fi
    # Credentials, state key, handles and binaries are removed once everything is verified clean.
    if [[ "${KEEP_PRIVATE:-0}" == 1 ]]; then
      note "private directory kept because clean closure was not acknowledged: ${PRIVATE}"
    else
      if rm -rf "${PRIVATE}"; then note "private credential directory removed"; else PRIVATE_CLEAN=0; code=1; KEEP_PRIVATE=1; note "private credential directory removal failed"; fi
    fi
  fi
  if ! node -e 'const fs=require("fs");const [path,runId,scenario,target,status,code,cleanup]=process.argv.slice(1);const doc={runId,scenario,target,status,exitCode:Number(code),cleanupState:cleanup};const tmp=path+".tmp";fs.writeFileSync(tmp,JSON.stringify(doc)+"\n",{mode:0o600,flag:"wx"});fs.renameSync(tmp,path)' \
    "${WORK}/runner-completion.json" "${RUN_ID}" "${SCENARIO}" "${TARGET}" "$([[ "${code}" == 0 && "${CLOSED_CLEAN}" == 1 && "${LOCAL_STOP_CLEAN}" == 1 && "${PRIVATE_CLEAN}" == 1 ]] && echo PASS || echo FAIL)" "${code}" "$([[ "${CLOSED_CLEAN}" == 1 ]] && echo CLOSED_CLEAN || echo INCOMPLETE)"; then
    note "runner completion receipt could not be written"; code=1
  fi
  echo "evidence in ${WORK}"
  echo "run-id=${RUN_ID} scenario=${SCENARIO} status=${code}"
  exit "${code}"
}
trap cleanup EXIT
trap 'exit 129' HUP
trap 'exit 130' INT
trap 'exit 143' TERM

# ---- PREFLIGHT (read-only, host chain, never a product fallback) ------------
"${CTL}" verify-binaries -config "${CONFIG}" > "${WORK}/preflight-binaries.json"
if [[ "${IS_AWS}" == 1 ]]; then
  printf '%s\n' "${AWS_PREFLIGHT}" > "${WORK}/preflight-aws.json"
else
  "${CTL}" k8s-preflight -config "${CONFIG}" > "${WORK}/preflight-k8s.json"
fi
"${CTL}" validate-config -config "${CONFIG}" > "${WORK}/public-config.json"
if [[ "${IS_AWS}" == 1 ]]; then
  check "preflight ok: pinned binaries, STS account/region/principal, /32 egress, pod capacity, permission simulation, empty run tag, I04-11 gate (no creation)"
else
  check "preflight ok: pinned binaries, k8s-4f context ${KUBE_CONTEXT}, no run namespace, sentinel UID, storage class, namespace create permission (no creation)"
fi
video_require_tools
video_require_fresh_dist "${REPO}"
XDOTOOL="$(video_xdotool)"

# ---- Disposable PostgreSQL with three separate logins -----------------------
"${CTL}" pg-provision -admin-url-file "${PG_ADMIN}" -out-dir "${PRIVATE}" | tee -a "${WORK}/cleanup.txt"
PG_PROVISIONED=1
CONSOLE_ROLE="$("${CTL}" pg-role -manifest "${PRIVATE}/pg-manifest.json" -kind console)"
STATE_ROLE="$("${CTL}" pg-role -manifest "${PRIVATE}/pg-manifest.json" -kind state)"
ADAPTERS=kubernetes AWS_FLAGS=()
if [[ "${IS_AWS}" == 1 ]]; then ADAPTERS=aws; AWS_FLAGS=(-region "$(cfg awsRegion)" -account-id "$(cfg awsAccountId)"); fi
ISOLATED=(env -i "PATH=${PATH}" "HOME=${PRIVATE}/home" "TMPDIR=${PRIVATE}/tmp" "LANG=C.UTF-8")
for proxy in HTTPS_PROXY HTTP_PROXY NO_PROXY https_proxy http_proxy no_proxy SSL_CERT_FILE; do
  if [[ -n "${!proxy:-}" ]]; then ISOLATED+=("${proxy}=${!proxy}"); fi
done
"${ISOLATED[@]}" "${ORCH}" -setup-database -database-owner-url-file "${PRIVATE}/owner.url" \
  -console-database-role "${CONSOLE_ROLE}" -state-database-role "${STATE_ROLE}" | tee -a "${WORK}/cleanup.txt"
# The state encryption key lives outside SQL.
"${CTL}" keygen -out "${PRIVATE}/state.key"

# ---- Run-owned Vault: scoped Connection token and scoped output-secret token -
synthetic_token() { head -c 32 /dev/urandom | od -An -tx1 | tr -d ' \n'; }
free_port() { node -e 'const s=require("net").createServer();s.listen(0,"127.0.0.1",()=>{console.log(s.address().port);s.close()})'; }
ROOT_TOKEN="$(synthetic_token)"
printf 'X-Vault-Token: %s\n' "${ROOT_TOKEN}" > "${PRIVATE}/root-header"
VAULT_PORT="$(free_port)"
VAULT_ADDR_LOCAL="http://127.0.0.1:${VAULT_PORT}"
# Its own session: the independent cleanup needs the output secrets (Vault) after the runner and the browser were killed.
VAULT_DEV_ROOT_TOKEN_ID="${ROOT_TOKEN}" setsid "${VAULT_BIN}" server -dev -dev-listen-address "127.0.0.1:${VAULT_PORT}" -dev-no-store-token > "${PRIVATE}/vault.log" 2>&1 &
VAULT_PID=$!
printf '%s\n' "${VAULT_PID}" > "${PRIVATE}/vault.pid"
unset ROOT_TOKEN
for _ in $(seq 1 60); do curl -fsS "${VAULT_ADDR_LOCAL}/v1/sys/health" >/dev/null 2>&1 && break; sleep 0.5; done
curl -fsS "${VAULT_ADDR_LOCAL}/v1/sys/health" >/dev/null || { echo "Vault did not become healthy" >&2; exit 1; }
vault_put_policy() { curl -fsS -H @"${PRIVATE}/root-header" -X PUT "${VAULT_ADDR_LOCAL}/v1/sys/policies/acl/$1" -d "$2" >/dev/null; }
vault_token() {
  curl -fsS -H @"${PRIVATE}/root-header" -X POST "${VAULT_ADDR_LOCAL}/v1/auth/token/create" -d "{\"policies\":[\"$1\"],\"no_default_policy\":true,\"ttl\":\"$2\"}" \
    | node -e 'let s="";process.stdin.on("data",d=>s+=d).on("end",()=>{const t=JSON.parse(s).auth?.client_token;if(!t)process.exit(1);process.stdout.write(t)})'
}
TOKEN_TTL="$(( EXEC_SECONDS / 3600 + 2 ))h"
vault_put_policy orch-connection-credentials '{"policy":"path \"secret/data/orchestrator/connections/*\" { capabilities = [\"create\", \"read\"] }\npath \"secret/metadata/orchestrator/connections/*\" { capabilities = [\"read\", \"list\", \"delete\"] }\n"}'
vault_put_policy orch-output-secrets '{"policy":"path \"secret/data/orchestrator/outputs/*\" { capabilities = [\"create\", \"read\"] }\n"}'
vault_token orch-connection-credentials "${TOKEN_TTL}" > "${PRIVATE}/connection-vault-token"
vault_token orch-output-secrets "${TOKEN_TTL}" > "${PRIVATE}/output-vault-token"
[[ -s "${PRIVATE}/connection-vault-token" && -s "${PRIVATE}/output-vault-token" ]] || { echo "scoped Vault tokens were not created" >&2; exit 1; }

# Read-only snapshot of the EXISTING configuration Vault's policies and roles: only objects created after it can be this run's.
SS_MODE="$(node -e 'const s=JSON.parse(process.argv[1]);process.stdout.write(!s?"none":s.runOwned?"run-owned":"shared")' "${SECRET_STORE_JSON}")"
if [[ "${SS_MODE}" == run-owned ]]; then
  # A SECOND run-owned Vault serves the application KV, workload policies and Kubernetes-auth roles. It listens ONLY on the Docker bridge address the
  # Pods reach; Kubernetes auth uses the EXISTING reviewer handles (read by the operator into 0600 files, never printed). Nothing on the shared Vault is touched.
  SS_BRIDGE="$(node -e 'process.stdout.write(JSON.parse(process.argv[1]).bridgeAddress)' "${SECRET_STORE_JSON}")"
  SS_PORT="$(free_port)"
  SS_ADDR="http://${SS_BRIDGE}:${SS_PORT}"
  SS_ROOT="$(synthetic_token)"
  printf 'X-Vault-Token: %s\n' "${SS_ROOT}" > "${PRIVATE}/store-root-header"
  VAULT_DEV_ROOT_TOKEN_ID="${SS_ROOT}" setsid "${VAULT_BIN}" server -dev -dev-listen-address "${SS_BRIDGE}:${SS_PORT}" -dev-no-store-token > "${PRIVATE}/store-vault.log" 2>&1 &
  VAULT2_PID=$!
  unset SS_ROOT
  for _ in $(seq 1 60); do curl -fsS "${SS_ADDR}/v1/sys/health" >/dev/null 2>&1 && break; sleep 0.5; done
  curl -fsS "${SS_ADDR}/v1/sys/health" >/dev/null || { echo "the run-owned secret-store Vault did not become healthy" >&2; exit 1; }
  store_api() { curl -fsS -H @"${PRIVATE}/store-root-header" -X "$1" "${SS_ADDR}/v1/$2" ${3:+-d "$3"} > /dev/null; }
  store_api POST sys/mounts/kv '{"type":"kv","options":{"version":"2"}}'
  store_api POST sys/auth/kubernetes '{"type":"kubernetes"}'
  node -e '
    const fs = require("fs"); const [jwtF, caF, host, out] = process.argv.slice(1);
    fs.writeFileSync(out, JSON.stringify({ kubernetes_host: host, kubernetes_ca_cert: fs.readFileSync(caF, "utf8"), token_reviewer_jwt: fs.readFileSync(jwtF, "utf8").trim() }), { mode: 0o600 });
  ' "$(node -e 'process.stdout.write(JSON.parse(process.argv[1]).reviewerJwtFile)' "${SECRET_STORE_JSON}")" "$(node -e 'process.stdout.write(JSON.parse(process.argv[1]).reviewerCaFile)' "${SECRET_STORE_JSON}")" "$(node -e 'process.stdout.write(JSON.parse(process.argv[1]).kubernetesHost)' "${SECRET_STORE_JSON}")" "${PRIVATE}/store-auth-config.json"
  curl -fsS -H @"${PRIVATE}/store-root-header" -X POST "${SS_ADDR}/v1/auth/kubernetes/config" -d @"${PRIVATE}/store-auth-config.json" > /dev/null
  rm -f "${PRIVATE}/store-auth-config.json"
  store_api PUT sys/policies/acl/orch-store-admin '{"policy":"path \"auth/token/lookup-self\" { capabilities = [\"read\"] }\npath \"sys/capabilities-self\" { capabilities = [\"update\"] }\npath \"kv/data/orchestrator/apps/*\" { capabilities = [\"create\", \"read\", \"update\"] }\npath \"kv/metadata/orchestrator/apps/*\" { capabilities = [\"read\", \"list\", \"delete\"] }\npath \"sys/policies/acl/orch-*\" { capabilities = [\"create\", \"read\", \"update\", \"delete\"] }\npath \"auth/kubernetes/role/orch-*\" { capabilities = [\"create\", \"read\", \"update\", \"delete\"] }\npath \"auth/kubernetes/config\" { capabilities = [\"read\"] }\n"}'
  curl -fsS -H @"${PRIVATE}/store-root-header" -X POST "${SS_ADDR}/v1/auth/token/create" -d '{"policies":["orch-store-admin"],"no_default_policy":true,"ttl":"'"${TOKEN_TTL}"'"}' \
    | node -e 'let s="";process.stdin.on("data",d=>s+=d).on("end",()=>{const t=JSON.parse(s).auth?.client_token;if(!t)process.exit(1);process.stdout.write(t)})' > "${PRIVATE}/secret-store.token"
  chmod 0600 "${PRIVATE}/secret-store.token"
  [[ -s "${PRIVATE}/secret-store.token" ]] || { echo "the secret-store token was not created" >&2; exit 1; }
  SECRET_STORE_TOKEN="${PRIVATE}/secret-store.token"
  SECRET_STORE_JSON="$(node -e 'const s=JSON.parse(process.argv[1]);s.backendAddress=s.workloadAddress=process.argv[2];s.tokenFile=process.argv[3];s.mount="kv";s.authMount="kubernetes";process.stdout.write(JSON.stringify(s))' "${SECRET_STORE_JSON}" "${SS_ADDR}" "${SECRET_STORE_TOKEN}")"
  note "run-owned secret-store Vault on the Docker bridge; existing reviewer handles used for Kubernetes auth; the shared Vault is untouched"
elif [[ "${SS_MODE}" == shared ]]; then
  "${CTL}" vault-snapshot -config "${CONFIG}" -out "${PRIVATE}/vault-snapshot.json" | tee -a "${WORK}/cleanup.txt"
else
  note "optional configuration Vault is not configured; the run-owned Connection/output Vault remains available"
fi

# Finite, nested bounds recorded in the manifest before the first create (readiness <= one Terraform invocation <= Deploy lease <= execution deadline;
# the cleanup steps fit the cleanup budget). AWS: 45m per invocation inside the 180m run, because EKS alone takes 15-25 minutes.
declare -A TIMEOUT
while IFS='=' read -r key value; do TIMEOUT["${key}"]="${value}"; done < <("${CTL}" timeouts -config "${CONFIG}")
TF_INVOCATION_TIMEOUT="${TIMEOUT[terraformInvocation]}s" TF_READINESS_TIMEOUT="${TIMEOUT[terraformReadiness]}s" DEPLOY_DEADLINE="${TIMEOUT[deployLease]}s"

# ---- The product: real adapters, isolated environment ------------------------
# env -i: no KUBECONFIG, no AWS_* / profile, no inherited database or Vault
# settings; only PATH and proxy variables for the Terraform provider download.
start_product() {
  local addr_file="$1" log="$2"
  rm -f "${addr_file}"
  "${ISOLATED[@]}" "${ORCH}" -addr 127.0.0.1:0 -addr-file "${addr_file}" -ui-dir "${REPO}/frontend/dist" \
    -adapters "${ADAPTERS}" ${AWS_FLAGS[@]+"${AWS_FLAGS[@]}"} -profile acceptance-v2 -namespace "${NAMESPACE}" -run-id "${RUN_ID}" -owner "${OWNER}" -ttl "${EXEC_SECONDS}s" \
    -score-k8s "${SCORE_K8S}" -terraform "${TERRAFORM}" -kubectl "${KUBECTL}" -git git \
    -terraform-root "${PRIVATE}/tfroot" -terraform-plugin-cache "${PRIVATE}/plugin-cache" -terraform-module-cache-dir "${PRIVATE}/module-cache" \
    -terraform-recovery-dir "${PRIVATE}/recovery" -terraform-state-key-file "${PRIVATE}/state.key" -terraform-state-key-id "v2-${RUN_ID}" \
    -terraform-state-database-url-file "${PRIVATE}/state.url" -database-url-file "${PRIVATE}/console.url" \
    -connection-credential-store vault -connection-vault-address "${VAULT_ADDR_LOCAL}" -connection-vault-token-file "${PRIVATE}/connection-vault-token" -connection-vault-mount secret \
    -output-secret-store vault -output-secret-vault-address "${VAULT_ADDR_LOCAL}" -output-secret-vault-token-file "${PRIVATE}/output-vault-token" -output-secret-vault-mount secret \
    -terraform-invocation-timeout "${TF_INVOCATION_TIMEOUT}" -terraform-readiness-timeout "${TF_READINESS_TIMEOUT}" -deploy-deadline "${DEPLOY_DEADLINE}" \
    -workload-delivery direct -vault-delivery "${VAULT_DELIVERY}" -application-key "${APPLICATION_KEY}" \
    > "${log}" 2>&1 &
  SERVER_PID=$!
  for _ in $(seq 1 240); do [[ -s "${addr_file}" ]] && break; kill -0 "${SERVER_PID}" 2>/dev/null || break; sleep 0.25; done
  [[ -s "${addr_file}" ]] || { echo "the product did not start; see ${log}" >&2; return 1; }
  API="http://$(<"${addr_file}")"
  for _ in $(seq 1 60); do curl -fsS "${API}/api/v1/healthz" >/dev/null 2>&1 && break; sleep 0.25; done
  curl -fsS "${API}/api/v1/healthz" >/dev/null
}
start_product "${WORK}/api-addr" "${WORK}/orchestrator.log"

# ---- Manifest: pins, planned counts, expiry BEFORE the first create ----------
"${CTL}" manifest-init -config "${CONFIG}" -manifest "${MANIFEST}"
pin() { "${CTL}" manifest-pin -manifest "${MANIFEST}" "$@"; }
pin -name terraform -version 1.9.8 -sha256 "$(sha256sum "${TERRAFORM}" | cut -d' ' -f1)"
pin -name score-k8s -version 0.15.0 -sha256 "$(sha256sum "${SCORE_K8S}" | cut -d' ' -f1)"
pin -name kubectl -sha256 "$(sha256sum "${KUBECTL}" | cut -d' ' -f1)"
if [[ "${IS_AWS}" == 1 ]]; then
  pin -name aws -sha256 "$(sha256sum "$(cfg awsBinary)" | cut -d' ' -f1)"
  pin -name probe:postgres -version "$(cfg awsProbePostgres)"
  pin -name probe:redis -version "$(cfg awsProbeRedis)"
fi
pin -name timeout:terraform-invocation -version "${TF_INVOCATION_TIMEOUT}"
pin -name timeout:terraform-readiness -version "${TF_READINESS_TIMEOUT}"
pin -name timeout:deploy-lease -version "${DEPLOY_DEADLINE}"
pin -name timeout:execution -version "${TIMEOUT[execution]}s"
pin -name timeout:cleanup-budget -version "${TIMEOUT[cleanupBudget]}s"
pin -name vault -sha256 "$(sha256sum "${VAULT_BIN}" | cut -d' ' -f1)"
while IFS=$'\t' read -r module revision sha; do pin -name "module:${module}" -version "${revision}" -sha256 "${sha}"; done < <("${CTL}" module-pins)
plan() { "${CTL}" manifest-plan -manifest "${MANIFEST}" -kind "$1" -count "$2"; }
case "${SCENARIO}" in
  A02|A03) while IFS=$'\t' read -r kind count; do plan "${kind}" "${count}"; done < <("${CTL}" planned-resources -scenario "${SCENARIO}") ;;
  A01) plan kubernetes_namespace_v1 1; plan kubernetes_stateful_set_v1 2; plan kubernetes_persistent_volume_claim_v1 2; plan workload_deployments 3; plan workload_services 2 ;;
  E01) plan kubernetes_namespace_v1 0; plan kubernetes_stateful_set_v1 0; plan workload_deployments 0 ;;
  E02) plan kubernetes_namespace_v1 1; plan kubernetes_stateful_set_v1 1; plan kubernetes_persistent_volume_claim_v1 1; plan workload_deployments 0 ;;
esac
if [[ "${IS_AWS}" == 1 ]]; then
  # The complete public-price estimate must match the planned counts of the manifest BEFORE the first create; no USD ceiling is invented.
  "${CTL}" estimate-check -config "${CONFIG}" -manifest "${MANIFEST}" -estimate "$(cfg awsEstimateFile)" | tee -a "${WORK}/checks.txt"
  # Snapshot of the account-wide service-linked roles (EKS, node group, RDS, ElastiCache): protected, never deleted, compared after cleanup.
  while read -r _ role; do
    [[ -n "${role}" ]] && "${CTL}" manifest-asset -manifest "${MANIFEST}" -kind iam-service-linked-role -id "${role}" -layer aux -owner PREEXISTING
  done < <("${CTL}" aws-slr -config "${CONFIG}" -out "${PRIVATE}/slr-snapshot.json")
else
  # The outside-run sentinel is protected: PREEXISTING assets never reach a delete step.
  "${CTL}" manifest-asset -manifest "${MANIFEST}" -kind sentinel -id "$("${CTL}" config-get -config "${CONFIG}" -field sentinelName)" -layer aux -owner PREEXISTING
fi
"${CTL}" manifest-arm -manifest "${MANIFEST}"
printf '{"scenario":"%s","runId":"%s","target":"'"${TARGET}"'","adapters":"REAL product: planner, pinned Terraform with encrypted PostgreSQL state, durable Vault Secret refs, required pinned score-k8s, real Kubernetes delivery","fake":"none","captions":"%s"}\n' "${SCENARIO}" "${RUN_ID}" "${CAPTIONS}" > "${WORK}/run.json"

# ---- Scenario input (private: handles and image refs) -------------------------
if [[ "${IS_AWS}" == 1 ]]; then
  : > "${PRIVATE}/connection.kubeconfig"   # no kubeconfig Connection on AWS: the AWS Connection is registered with the scoped key
  "${CTL}" aws-input -config "${CONFIG}" -catalog "${ROOT}/test/integration/v2run/testdata/aws-catalog.json" > "${PRIVATE}/aws-input.json"
else
  "${KUBECTL}" --kubeconfig "${KUBECONFIG_HANDLE}" --context "${KUBE_CONTEXT}" config view --minify --flatten --raw > "${PRIVATE}/connection.kubeconfig"
fi
chmod 0600 "${PRIVATE}/connection.kubeconfig" "${PRIVATE}"/aws-input.json 2>/dev/null || true
FORWARD_PORT="$(free_port)"
# Generated application secrets (the Console-entered secret, the PostgreSQL and Redis passwords) are appended by the scenario; never printed.
: > "${PRIVATE}/app-secrets.txt"
EXEC_MS=$(( EXEC_SECONDS * 1000 ))
node -e '
  const [out, applicationKey, secretStoreJson, runId, scenario, namespace, kubectl, kubeconfig, context, connKubeconfig, ctl, privateDir, configFile, manifestFile, forwardPort, execMs, cleanupSeconds, imgF, imgB, imgW] = process.argv.slice(1);
  const secretStore = JSON.parse(secretStoreJson);
  const input = {
    runId, scenario, namespace, applicationName: "Acceptance Application", applicationKey,
    secretStore,
    connection: { name: "k8s-4f", kubeconfigFile: connKubeconfig },
    images: { frontend: imgF, backend: imgB, worker: imgW },
    kube: { kubectl, kubeconfig, context },
    cache: { ttlSeconds: 45 }, scoreVersion: "0.15.0", forwardPort: Number(forwardPort),
    deployTimeoutMs: Math.min(Number(execMs), 40 * 60 * 1000), readyTimeoutMs: 10 * 60 * 1000, cleanupUiTimeoutMs: 15 * 60 * 1000, cleanupWaitDuration: `${cleanupSeconds}s`,
    v2ctl: ctl, privateDir, configFile, manifestFile,
    e01: { invalidDatabase: "Bad-Name!", fieldPattern: "database" },
    e02: { redisRepository: "redis:7.4.7-alpine", failureTimeoutMs: 25 * 60 * 1000 },
  };
  const fs = require("fs");
  const awsFile = privateDir + "/aws-input.json";
  if (fs.existsSync(awsFile)) {
    // AWS scenarios (A02/A03): the key handle content is typed only into the masked Console fields; the deploy chain may use the whole execution deadline.
    input.aws = JSON.parse(fs.readFileSync(awsFile, "utf8"));
    input.deployTimeoutMs = Number(execMs);
    input.readyTimeoutMs = 25 * 60 * 1000;
    input.cleanupUiTimeoutMs = 15 * 60 * 1000;
    input.cache = { ttlSeconds: 45 };
  }
  fs.writeFileSync(out, JSON.stringify(input), { mode: 0o600 });
' "${PRIVATE}/scenario-input.json" "${APPLICATION_KEY}" "${SECRET_STORE_JSON}" "${RUN_ID}" "${SCENARIO}" "${NAMESPACE}" "${KUBECTL}" "${KUBECONFIG_HANDLE}" "${KUBE_CONTEXT}" "${PRIVATE}/connection.kubeconfig" "${CTL}" "${PRIVATE}" "${CONFIG}" "${MANIFEST}" "${FORWARD_PORT}" "${EXEC_MS}" "${CLEANUP_SECONDS}" "$(cfg imageFrontend)" "$(cfg imageBackend)" "$(cfg imageWorker)"

# ---- Headed browser (gated), then the independent supervisor, then GO --------
video_start_xvfb "${SCREEN}"
RAW="${WORK}/${SCENARIO}-raw.mp4"
GATE="${PRIVATE}/start-gate"
: > "${WORK}/playwright.log"
tail -n +1 -f "${WORK}/playwright.log" &
PIDS+=($!)
DISPLAY="${VIDEO_DISPLAY}" ORCH_E2E_SCREEN="${SCREEN}" ORCH_E2E_XDOTOOL="${XDOTOOL}" ORCH_E2E_START_GATE="${GATE}" \
  ORCH_E2E_URL="${API}" ORCH_E2E_EVIDENCE_DIR="${WORK}" ORCH_E2E_INPUT="${PRIVATE}/scenario-input.json" \
  PATH="${PATH}" setsid node "${E2E}/terraform-v2-acceptance.mjs" --scenario "${SCENARIO}" --headed --captions "${CAPTIONS}" --evidence "${WORK}" \
  > "${WORK}/playwright.log" 2>&1 &
NODE_PID=$!
# The supervisor is a SEPARATE process group and session, armed before the gate opens. Its safety net rebuilds the product wiring from private handles.
"${CTL}" handles-write -config "${CONFIG}" -private "${PRIVATE}" -vault-address "${VAULT_ADDR_LOCAL}" -invocation-timeout "${TF_INVOCATION_TIMEOUT}" -readiness-timeout "${TF_READINESS_TIMEOUT}"
"${CTL}" supervisor-start -config "${CONFIG}" -manifest "${MANIFEST}" -private "${PRIVATE}" -supervisor "${SUPERVISOR}" -cleanup "${CLEANUP_BIN}" -handles "${PRIVATE}/cleanup-handles.json" -runner-pid "$$" -browser-pid "${NODE_PID}" | tee "${WORK}/supervisor-armed.json"
SUPERVISED=1
"${CTL}" run-marker -dir "${EVIDENCE}" -run "${RUN_ID}" -target "${TARGET}" -state ARMED
note "cleanup supervisor armed before the first create (own session and process group)"
if [[ "${IS_AWS}" == 1 ]]; then
  # ECR repositories are pre-recorded in the armed manifest and created while
  # the independent cleanup supervisor is already watching this run.
  "${CTL}" aws-ecr-prepare -config "${CONFIG}" -manifest "${MANIFEST}" -out "${PRIVATE}/aws-ecr-publication.json" | tee -a "${WORK}/checks.txt"
  node -e '
    const fs = require("fs");
    const inputPath = process.argv[1];
    const publication = JSON.parse(fs.readFileSync(process.argv[2], "utf8"));
    const input = JSON.parse(fs.readFileSync(inputPath, "utf8"));
    if (Object.keys(publication.images ?? {}).sort().join(",") !== "backend,frontend,worker" || publication.repositories?.length !== 3) process.exit(1);
    input.images = publication.images;
    input.aws.ecrPreparation = publication;
    fs.writeFileSync(inputPath, JSON.stringify(input), { mode: 0o600 });
  ' "${PRIVATE}/scenario-input.json" "${PRIVATE}/aws-ecr-publication.json"
fi
: > "${GATE}"
wait "${NODE_PID}" || STATUS=$?
NODE_PID=""
sleep 1

# ---- Post-run checks ------------------------------------------------------------
# The scenario already asked the supervisor to clean up; ask again only if it did not get that far.
if [[ "${FINISHED}" == 0 ]]; then "${CTL}" supervisor-finish -private "${PRIVATE}" 2>/dev/null || true; FINISHED=1; fi
SUPERVISOR_CLEAN=0
PHYSICAL_ABSENT=0
SENTINEL_PRESERVED=1
if SUMMARY="$("${CTL}" supervisor-wait -private "${PRIVATE}" -timeout "${CLEANUP_SECONDS}s" 2>&1)"; then
  SUPERVISOR_CLEAN=1
  check "independent supervisor: CLEAN"
else
  check "FAIL independent supervisor needs attention; state and recovery evidence are kept"; STATUS=1; KEEP_PRIVATE=1
fi
printf '%s\n' "${SUMMARY}" | grep -v -i -E 'token|password|secret' > "${WORK}/cleanup-summary.json" || true
if [[ "${IS_AWS}" == 1 ]]; then
  if "${CTL}" aws-verify -config "${CONFIG}" -manifest "${MANIFEST}" -layer aux > "${WORK}/cleanup-queries.txt" 2>&1; then check "every run-tagged and exactly linked AWS asset verified absent (VPC, EKS, Aurora, cache, network, EC2, EBS, EIP, ECR, IAM)"; PHYSICAL_ABSENT=1; else check "FAIL AWS assets are not verified absent (see cleanup-queries.txt)"; STATUS=1; KEEP_PRIVATE=1; fi
else
  if "${CTL}" k8s-absent -config "${CONFIG}" -manifest "${MANIFEST}" > "${WORK}/cleanup-queries.txt" 2>&1; then check "namespace and run-owned PVs verified absent"; PHYSICAL_ABSENT=1; else check "FAIL run objects are not verified absent (see cleanup-queries.txt)"; STATUS=1; KEEP_PRIVATE=1; fi
  if "${CTL}" k8s-sentinel -config "${CONFIG}" > "${WORK}/sentinel.txt" 2>&1; then check "sentinel preserved"; else check "FAIL sentinel check failed"; STATUS=1; SENTINEL_PRESERVED=0; fi
fi
"${CTL}" manifest-public -manifest "${MANIFEST}" -out "${WORK}/manifest.public.json"

# SQL reopen: restart the product on the same database and read the catalog back.
kill -TERM "${SERVER_PID}" 2>/dev/null || true
for _ in $(seq 1 40); do kill -0 "${SERVER_PID}" 2>/dev/null || break; sleep 0.25; done
wait "${SERVER_PID}" 2>/dev/null || true
SERVER_PID=""
if start_product "${WORK}/api-addr-reopen" "${WORK}/orchestrator-reopen.log"; then
  REOPEN_CONNECTION_KEY="$(node -e 'const fs=require("fs");const i=JSON.parse(fs.readFileSync(process.argv[1],"utf8"));process.stdout.write(i.aws?.connectionName ?? i.connection?.name ?? "")' "${PRIVATE}/scenario-input.json")"
  ORCH_REPO_ROOT="${REPO}" node "${E2E}/terraform-v2-reopen.mjs" "${API}" "${SCENARIO}" "${APPLICATION_KEY}" "${REOPEN_CONNECTION_KEY}" | tee -a "${WORK}/checks.txt" || STATUS=1
else
  check "FAIL the product did not restart on the same database"; STATUS=1
fi

# Evidence rules: secret scan of every public text file first (its verdict is an assertion), then assertions and phases.
SECRET_FILES="${PRIVATE}/app-secrets.txt,${PRIVATE}/console.url,${PRIVATE}/state.url,${PRIVATE}/owner.url,${PRIVATE}/state.key,${PRIVATE}/root-header,${PRIVATE}/connection-vault-token,${PRIVATE}/output-vault-token,${PRIVATE}/connection.kubeconfig"
if [[ "${IS_AWS}" == 1 ]]; then SECRET_FILES+=",${PRIVATE}/aws-input.json,$(cfg awsAccessKeyFile)"; fi
if [[ "${SS_MODE}" == run-owned ]]; then
  SECRET_FILES+=",${PRIVATE}/store-root-header,${SECRET_STORE_TOKEN}"
elif [[ "${SS_MODE}" == shared ]]; then
  SECRET_FILES+=",${SECRET_STORE_TOKEN}"
fi
if SCAN="$("${CTL}" scan -public "${WORK}" -secrets "${SECRET_FILES}" 2>&1)"; then
  check "${SCAN}"
  SCAN_PASS=true
else
  check "FAIL secret scan: ${SCAN}"; STATUS=1; SCAN_PASS=false
fi
if [[ "${IS_AWS}" == 1 ]]; then
  # Preflight verdicts (read-only, taken before the first create) and the preservation proof of the pre-existing service-linked roles.
  verdict() { printf '%s' "${AWS_PREFLIGHT}" | node -e 'let s="";process.stdin.on("data",d=>s+=d).on("end",()=>{const c=JSON.parse(s);const names=process.argv.slice(1);process.stdout.write(names.every(n=>c.some(x=>x.name===n&&x.ok))?"true":"false")})' "$@"; }
  "${CTL}" assertion -file "${WORK}/assertions.json" -id aws.preflight-account-region-principal -phase PREPARE -source private-read -pass="$(verdict sts-account sts-principal region-fixed)" -detail "STS account and principal equal the configured ones; region is us-east-1"
  "${CTL}" assertion -file "${WORK}/assertions.json" -id aws.egress-32 -phase PREPARE -source private-read -pass="$(verdict 'executor-egress-/32')" -detail "the executor egress allowlist is one explicit IPv4 /32"
  "${CTL}" assertion -file "${WORK}/assertions.json" -id aws.pod-capacity -phase PREPARE -source private-read -pass="$(verdict pod-capacity)" -detail "node type pod budget covers system and acceptance pods"
  if SLR_OUT="$("${CTL}" aws-slr -verify -config "${CONFIG}" -snapshot "${PRIVATE}/slr-snapshot.json" 2>&1)"; then SLR_PASS=true; else SLR_PASS=false; check "FAIL ${SLR_OUT}"; STATUS=1; KEEP_PRIVATE=1; fi
  "${CTL}" assertion -file "${WORK}/assertions.json" -id aws.preexisting-preserved -phase CLEANUP -source private-read -pass="${SLR_PASS}" -detail "pre-existing EKS/EKS node group/RDS/ElastiCache service-linked roles keep the same ARN, role id, trust policy and attached policies"
fi
if [[ "${SUPERVISOR_CLEAN}" == 1 && "${PHYSICAL_ABSENT}" == 1 && "${SENTINEL_PRESERVED}" == 1 && "${KEEP_PRIVATE:-0}" != 1 && ("${IS_AWS}" != 1 || "${SLR_PASS}" == true) ]]; then
  # Scenario assertions may fail while cleanup is fully proven. Keep result.txt
  # and STATUS honest; the clean marker is solely the independent cleanup proof.
  CLEANUP_PROVEN=1
fi
"${CTL}" assertion -file "${WORK}/assertions.json" -id evidence.no-secret-public -phase CLEANUP -source private-read -pass="${SCAN_PASS}" -detail "public text evidence scanned for the Console/State/Owner logins, state key, Vault tokens and the Connection kubeconfig"
if [[ -s "${WORK}/assertions.json" && -s "${WORK}/phases.json" ]]; then
  "${CTL}" check-evidence -scenario "${SCENARIO}" -marks "${WORK}/phases.json" -assertions "${WORK}/assertions.json" | tee -a "${WORK}/checks.txt" || STATUS=1
else
  check "FAIL assertions.json or phases.json is missing"; STATUS=1
fi

CAPTIONED="${WORK}/${SCENARIO}-captioned.mp4"
W="${SCREEN%x*}" H="${SCREEN#*x}"
if [[ -s "${RAW}" && -s "${WORK}/captions.srt" ]]; then
  node "${E2E}/render-captions.mjs" "${WORK}/captions.srt" "${WORK}/captions.ass" "${W}" "$((H + STRIP))"
  ffmpeg -hide_banner -loglevel error -n -i "${RAW}" -vf "pad=${W}:$((H + STRIP)):0:0:black,ass=${WORK}/captions.ass" \
    -r 15 -c:v libx264 -preset veryfast -crf 24 -pix_fmt yuv420p -movflags +faststart "${CAPTIONED}" || { echo "caption burn-in failed" >&2; STATUS=1; }
fi
"${CTL}" scan -public "${WORK}" -secrets "${SECRET_FILES}" > /dev/null || STATUS=1

if [[ "${STATUS}" != 0 ]]; then
  echo "scenario ${SCENARIO} failed; video, phases, assertions and checks kept in ${WORK}" >&2
  exit "${STATUS}"
fi
[[ "$(<"${WORK}/result.txt")" == PASS ]] || { echo "result.txt is not PASS" >&2; exit 1; }
LAST_MARK=cleanup
video_validate "${RAW}" "${SCREEN}" "${MIN_SECONDS}" 4 "${LAST_MARK}"
ffmpeg -hide_banner -v error -xerror -i "${CAPTIONED}" -f null - 2> "${WORK}/decode-captioned.log" \
  || { echo "captioned video does not decode; see ${WORK}/decode-captioned.log" >&2; exit 1; }
[[ ! -s "${WORK}/decode-captioned.log" ]] || { echo "captioned video decode errors" >&2; exit 1; }
[[ "$(ffprobe -v error -select_streams v:0 -show_entries stream=width,height -of csv=p=0 "${CAPTIONED}")" == "${W},$((H + STRIP))" ]] \
  || { echo "captioned video has unexpected size" >&2; exit 1; }
echo "captioned=${CAPTIONED}"
