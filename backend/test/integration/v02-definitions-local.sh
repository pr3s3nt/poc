#!/usr/bin/env bash
# V02 Resource Definition local browser scenario: a Platform Engineer authors and
# edits Resource Definitions through the Web Console (Name, Driver Account and
# literal region, five-field AND/OR criteria), checks matching on the server
# (unique winner, tie, no match, unusable winner account, UNCONFIGURED
# Environment), saves a version-CAS edit that keeps the ID and Resource Type,
# resolves a stale 409, reloads and restarts the product server; recorded headed
# with Vietnamese captions below the browser.
#   v02-definitions-local.sh --headed --captions vi --evidence NEW_PATH
#
# REAL: the Web Console bundle, the product HTTP handlers and services, a
#   disposable PostgreSQL database (Console store) and a Vault KV v2 credential
#   store (run-owned `vault server -dev`, or an existing task Vault with a
#   scoped token file) that holds the two fixture accounts' credentials.
# FAKE (labelled in the captions): the AWS HTTP endpoints (deterministic awsfake
#   behind the product's real read-only verifier) that make the two fixture AWS
#   accounts READY. No Terraform runs and no AWS, cluster or Docker is
#   contacted; a PASS is NOT evidence that real AWS accepts a credential, that a
#   plan executes, or that the removed Environment account fallback is gone in a
#   live multi-account run (V02 does not run live AWS).
# API setup is limited to labelled fixtures (two accounts, two Resource Types and
# three competing Definitions) and to assertions/negative checks; every
# Definition create/edit, account/region choice, criteria edit and matching check
# of the scenario is a UI action.
#
# Required environment:
#   ORCH_V02_PG_ADMIN_URL_FILE   0600 file with the administrator URL of a
#                                DISPOSABLE PostgreSQL server you own. The run
#                                creates one database and three roles with
#                                random passwords and drops exactly those.
#   Vault, ONE of:
#     ORCH_V02_VAULT_BIN         vault binary (>= 1.15): run-owned dev server.
#     ORCH_V02_VAULT_ADDR + ORCH_V02_VAULT_TOKEN_FILE [+ ORCH_V02_VAULT_MOUNT]
#                                existing Vault; the 0600 token file must be a
#                                token scoped to orchestrator/connections/*
#                                (create+read data; read,list,delete metadata).
#                                Exact ownership: awsconnserver journals every
#                                object it writes; cleanup deletes only those
#                                (never by listing), proves each absent and fails
#                                the run if one remains.
#     ORCH_V02_VAULT_IMAGE       alternative to the binary: a docker image.
#   ORCH_V02_SCORE_K8S           path of the pinned score-k8s binary (default: score-k8s on
#                                PATH). It only makes the render bundle selectable; nothing renders.
# Optional: ORCH_V02_PORT, ORCH_V02_VAULT_MOUNT (default secret), ORCH_VIDEO_SCREEN
#   (default 1440x900), ORCH_VIDEO_MIN_SECONDS (default 240).
# Needs: go, node, curl, Xvfb, xdotool, ffmpeg/ffprobe, a built frontend/dist and
#   the Playwright Chromium (see video-lib.sh).
# Evidence (NEW_PATH, outside the repository, must not exist): V02-raw.mp4,
# V02-captioned.mp4 (captions BELOW the browser), captions.srt/.ass, marks.json,
# assertions.txt, result.txt, run.json, checks.txt, definition-writes.json,
# accounts.json, aws-calls-*.json, orchestrator-*.log, vault-health.json,
# cleanup.txt, frames/*.png. Credentials live in NEW_PATH/private (0700), removed
# on exit, and never in the evidence; every retained text file is scanned for
# them. A failed run keeps the raw/captioned video, marks, assertions and checks.
# Debug / fault checks (each makes the run exit non-zero by design):
#   ORCH_V02_INJECT_FAILURE=after-success   fail the scenario after the end;
#       video, marks, assertions and cleanup survive.
#   ORCH_V02_FAULT_DELETE=1   make the first cleanup pass unauthorised; the
#       failure must be reported (a real pass then removes the objects).
set -euo pipefail
umask 077

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
[[ "${HEADED}" == 1 && -n "${CAPTIONS}" && -n "${EVIDENCE}" ]] || usage
[[ "${CAPTIONS}" == vi ]] || { echo "unsupported captions ${CAPTIONS}; only vi" >&2; exit 2; }
[[ -n "${ORCH_V02_PG_ADMIN_URL_FILE:-}" && -f "${ORCH_V02_PG_ADMIN_URL_FILE}" ]] || { echo "ORCH_V02_PG_ADMIN_URL_FILE must name the administrator URL file of a disposable PostgreSQL server" >&2; exit 2; }
if [[ -n "${ORCH_V02_VAULT_ADDR:-}" ]]; then
  [[ -n "${ORCH_V02_VAULT_TOKEN_FILE:-}" && -f "${ORCH_V02_VAULT_TOKEN_FILE}" ]] || { echo "ORCH_V02_VAULT_TOKEN_FILE must name the scoped token file of the existing Vault" >&2; exit 2; }
elif [[ -z "${ORCH_V02_VAULT_BIN:-}" && -z "${ORCH_V02_VAULT_IMAGE:-}" ]]; then
  echo "set ORCH_V02_VAULT_BIN (run-owned dev Vault) or ORCH_V02_VAULT_ADDR with ORCH_V02_VAULT_TOKEN_FILE" >&2; exit 2
fi
SCORE_K8S="${ORCH_V02_SCORE_K8S:-$(command -v score-k8s || true)}"
[[ -n "${SCORE_K8S}" && -x "${SCORE_K8S}" ]] || { echo "set ORCH_V02_SCORE_K8S (or put score-k8s on PATH): the rendering-template steps list its render bundle; nothing is rendered" >&2; exit 2; }
command -v node >/dev/null || { echo "node is required" >&2; exit 1; }
command -v curl >/dev/null || { echo "curl is required" >&2; exit 1; }
node "${E2E}/v02-definitions-local.mjs" --check V02 || exit 2

EVIDENCE="$(realpath -m "${EVIDENCE}")"
case "${EVIDENCE}/" in "${REPO}/"*) echo "evidence path must be outside the repository" >&2; exit 2 ;; esac
if [[ -e "${EVIDENCE}" || -L "${EVIDENCE}" ]]; then
  echo "evidence path ${EVIDENCE} already exists; choose a new path" >&2
  exit 2
fi
mkdir -p "$(dirname "${EVIDENCE}")"
mkdir -m 0700 "${EVIDENCE}"
WORK="${EVIDENCE}"
PRIVATE="${WORK}/private"
mkdir -m 0700 "${PRIVATE}"
export ORCH_VIDEO_DIR="${WORK}"
RUN_ID="v02-$(date -u +%Y%m%d%H%M%S)-${RANDOM}"
SCREEN="${ORCH_VIDEO_SCREEN:-1440x900}"
MIN_SECONDS="${ORCH_VIDEO_MIN_SECONDS:-240}"
STRIP=96
VAULT_IMAGE="${ORCH_V02_VAULT_IMAGE:-hashicorp/vault:1.20}"
MOUNT="${ORCH_V02_VAULT_MOUNT:-secret}"
VAULT_CONTAINER="" VAULT_PID="" NODE_PID="" NODE_PGID="" PROVISIONED=0 DROP_FAILED=0 EXISTING_VAULT=0
VAULT_ORG="acme"
PIDS=()
# shellcheck source=video-lib.sh
source "${ROOT}/test/integration/video-lib.sh"

: > "${WORK}/cleanup.txt"
note() { printf '%s\n' "$*" >> "${WORK}/cleanup.txt"; }

stop_server() {
  local pid
  [[ -s "${PRIVATE}/server.pid" ]] || return 0
  pid="$(<"${PRIVATE}/server.pid")"
  if kill -0 "${pid}" 2>/dev/null; then
    kill -TERM "${pid}" 2>/dev/null || true
    for _ in $(seq 1 40); do kill -0 "${pid}" 2>/dev/null || break; sleep 0.25; done
    kill -KILL "${pid}" 2>/dev/null || true
    note "product server stopped pid=${pid}"
  fi
  : > "${PRIVATE}/server.pid"
}

# Exact run ownership: awsconnserver appends every credential object it writes
# ("put <ref>") or deletes ("del <ref>") to a durable journal before returning.
# Cleanup deletes only journaled objects that are still alive, never anything
# found by listing, then proves each one is absent. A foreign object under the
# same key survives; a failed deletion or a surviving object fails the run.
CLEANUP_FAILED=0
JOURNAL="${PRIVATE}/credential-journal"
# owned_objects: "<key> <uuid>" for every journaled object not deleted by the product.
owned_objects() {
  [[ -s "${JOURNAL}" ]] || return 0
  node -e '
    const fs = require("fs");
    const [journal, mount, org] = process.argv.slice(1);
    const re = new RegExp(`^[a-z0-9]+://${mount}/orchestrator/connections/${org}/([a-z0-9-]+)/credentials/([0-9a-f-]{36})$`);
    const alive = new Map();
    for (const line of fs.readFileSync(journal, "utf8").split("\n")) {
      const m = /^(put|del) (\S+)$/.exec(line);
      if (!m) continue;
      const ref = re.exec(m[2]);
      if (!ref) { console.error("journal holds a reference outside this run scope"); process.exit(3); }
      if (m[1] === "put") alive.set(m[2], `${ref[1]} ${ref[2]}`); else alive.delete(m[2]);
    }
    for (const value of alive.values()) console.log(value);
  ' "${JOURNAL}" "${MOUNT}" "${VAULT_ORG}"
}
vault_status() { # header-file, key, uuid -> http code of the object's data read
  curl -sS -o /dev/null -w '%{http_code}' -H @"$1" "${VAULT_ADDR_LOCAL}/v1/${MOUNT}/data/orchestrator/connections/${VAULT_ORG}/$2/credentials/$3"
}
# delete_owned <header-file> [label]: deletes (metadata) the alive journaled objects.
delete_owned() {
  local header="$1" label="${2:-}" key id code failed=0
  while read -r key id; do
    [[ -n "${key}" ]] || continue
    code="$(curl -sS -o /dev/null -w '%{http_code}' -H @"${header}" -X DELETE "${VAULT_ADDR_LOCAL}/v1/${MOUNT}/metadata/orchestrator/connections/${VAULT_ORG}/${key}/credentials/${id}" 2>/dev/null || echo 000)"
    if [[ "${code}" =~ ^20[04]$ ]] && [[ "$(vault_status "${header}" "${key}" "${id}" 2>/dev/null || echo 000)" == 404 ]]; then
      note "${label}run-owned vault object ${key}/${id}: deleted, absence verified"
    else
      note "${label}FAILED run-owned vault object ${key}/${id}: delete status ${code}, not proven absent"
      failed=1
    fi
  done < <(owned_objects)
  return "${failed}"
}
cleanup_vault_objects() {
  [[ -n "${VAULT_ADDR_LOCAL:-}" && -f "${PRIVATE}/scoped-header" ]] || return 0
  local count
  count="$(owned_objects | wc -l)" || { note "FAILED to read the credential journal"; CLEANUP_FAILED=1; return 0; }
  note "credential journal: ${count} run-owned object(s) still alive"
  if [[ "${ORCH_V02_FAULT_DELETE:-0}" == 1 ]]; then
    # Fault check: an unauthorised delete must be REPORTED as a failure.
    printf 'X-Vault-Token: invalid\n' > "${PRIVATE}/bad-header"
    if delete_owned "${PRIVATE}/bad-header" "(injected fault) "; then note "FAIL injected delete fault was not detected"; else note "injected delete fault detected and reported"; fi
    CLEANUP_FAILED=1
  fi
  if ! delete_owned "${PRIVATE}/scoped-header"; then CLEANUP_FAILED=1; note "run-owned object cleanup did NOT complete; journal kept in ${PRIVATE}"; fi
}

cleanup() {
  local status=$?
  local pid
  if [[ -n "${NODE_PID}" ]] && kill -0 "${NODE_PID}" 2>/dev/null; then
    kill -TERM "${NODE_PID}" 2>/dev/null || true
    for _ in $(seq 1 80); do kill -0 "${NODE_PID}" 2>/dev/null || break; sleep 0.5; done
  fi
  if [[ -n "${NODE_PGID}" ]] && kill -0 -- "-${NODE_PGID}" 2>/dev/null; then kill -KILL -- "-${NODE_PGID}" 2>/dev/null || true; fi
  if [[ -n "${NODE_PID}" ]]; then wait "${NODE_PID}" 2>/dev/null || true; fi
  stop_server
  cleanup_vault_objects
  if [[ "${CLEANUP_FAILED}" == 1 ]]; then status=1; fi
  if [[ "${PROVISIONED}" == 1 ]]; then
    if "${WORK}/awsconnserver" drop -admin-url-file "${ORCH_V02_PG_ADMIN_URL_FILE}" -manifest "${PRIVATE}/manifest.json" >> "${WORK}/cleanup.txt" 2>&1; then note "disposable database and roles dropped"; else note "FAILED to drop the disposable database; manifest kept in ${PRIVATE}"; DROP_FAILED=1; status=1; fi
  fi
  for pid in "${PIDS[@]}"; do kill "${pid}" 2>/dev/null || true; done
  for pid in "${PIDS[@]}"; do wait "${pid}" 2>/dev/null || true; done
  if [[ -n "${VAULT_PID}" ]]; then kill "${VAULT_PID}" 2>/dev/null || true; wait "${VAULT_PID}" 2>/dev/null || true; note "vault dev server stopped pid=${VAULT_PID}"; fi
  if [[ -n "${VAULT_CONTAINER}" ]]; then
    docker rm -f "${VAULT_CONTAINER}" >/dev/null 2>&1 || true
    if docker container inspect "${VAULT_CONTAINER}" >/dev/null 2>&1; then note "WARNING vault container ${VAULT_CONTAINER} still present"; status=1; else note "vault container ${VAULT_CONTAINER} removed"; fi
  fi
  if [[ "${DROP_FAILED}" == 1 || "${CLEANUP_FAILED}" == 1 ]]; then
    # Keep what a retry needs: the manifest and the object journal, never tokens.
    find "${PRIVATE}" -mindepth 1 ! -name manifest.json ! -name credential-journal -delete 2>/dev/null || true
  else
    rm -rf "${PRIVATE}"
  fi
  if [[ ! -e "${PRIVATE}" ]]; then note "private credential directory removed"; else note "kept only for a retry (database drop or vault cleanup failed): ${PRIVATE}"; fi
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

# Test-only launcher (fake AWS verification) and the Console login database.
(cd "${ROOT}" && go build -o "${WORK}/awsconnserver" ./test/integration/awsconnserver)
"${WORK}/awsconnserver" provision -admin-url-file "${ORCH_V02_PG_ADMIN_URL_FILE}" -out-dir "${PRIVATE}" | tee -a "${WORK}/cleanup.txt"
PROVISIONED=1

synthetic_token() { head -c 32 /dev/urandom | od -An -tx1 | tr -d ' \n'; }
free_port() { node -e 'const s=require("net").createServer();s.listen(0,"127.0.0.1",()=>{console.log(s.address().port);s.close()})'; }
json_field() { node -e 'let s="";process.stdin.on("data",d=>s+=d).on("end",()=>{const v=JSON.parse(s).auth?.client_token;if(!v)process.exit(1);process.stdout.write(v)})'; }

if [[ -n "${ORCH_V02_VAULT_ADDR:-}" ]]; then
  EXISTING_VAULT=1
  VAULT_ADDR_LOCAL="${ORCH_V02_VAULT_ADDR}"
  cp "${ORCH_V02_VAULT_TOKEN_FILE}" "${PRIVATE}/connection-vault-token"
  chmod 0600 "${PRIVATE}/connection-vault-token"
  note "existing task Vault in use (scoped token copied to a private 0600 file, removed at exit)"
else
  ROOT_TOKEN="$(synthetic_token)"
  printf 'X-Vault-Token: %s\n' "${ROOT_TOKEN}" > "${PRIVATE}/root-header"
  VAULT_PORT="$(free_port)"
  VAULT_ADDR_LOCAL="http://127.0.0.1:${VAULT_PORT}"
  if [[ -n "${ORCH_V02_VAULT_BIN:-}" ]]; then
    [[ -x "${ORCH_V02_VAULT_BIN}" ]] || { echo "ORCH_V02_VAULT_BIN is not executable" >&2; exit 1; }
    VAULT_DEV_ROOT_TOKEN_ID="${ROOT_TOKEN}" "${ORCH_V02_VAULT_BIN}" server -dev -dev-listen-address "127.0.0.1:${VAULT_PORT}" -dev-no-store-token > "${PRIVATE}/vault.log" 2>&1 &
    VAULT_PID=$!
  else
    command -v docker >/dev/null || { echo "set ORCH_V02_VAULT_BIN or install docker for the run-owned Vault" >&2; exit 1; }
    VAULT_CONTAINER="orch-${RUN_ID}"
    printf 'VAULT_DEV_ROOT_TOKEN_ID=%s\nVAULT_DEV_LISTEN_ADDRESS=0.0.0.0:8200\n' "${ROOT_TOKEN}" > "${PRIVATE}/vault.env"
    docker run -d --rm --name "${VAULT_CONTAINER}" --label "orchestrator.run-id=${RUN_ID}" --cap-add IPC_LOCK \
      --env-file "${PRIVATE}/vault.env" -p "127.0.0.1:${VAULT_PORT}:8200" "${VAULT_IMAGE}" server -dev >/dev/null
  fi
  unset ROOT_TOKEN
  MOUNT=secret
  for _ in $(seq 1 60); do curl -fsS "${VAULT_ADDR_LOCAL}/v1/sys/health" >/dev/null 2>&1 && break; sleep 0.5; done
  curl -fsS "${VAULT_ADDR_LOCAL}/v1/sys/health" >/dev/null || { echo "Vault did not become healthy" >&2; exit 1; }
  curl -fsS -H @"${PRIVATE}/root-header" -X PUT "${VAULT_ADDR_LOCAL}/v1/sys/policies/acl/orch-connection-credentials" \
    -d '{"policy":"path \"secret/data/orchestrator/connections/*\" { capabilities = [\"create\", \"read\"] }\npath \"secret/metadata/orchestrator/connections/*\" { capabilities = [\"read\", \"list\", \"delete\"] }\n"}' >/dev/null
  curl -fsS -H @"${PRIVATE}/root-header" -X POST "${VAULT_ADDR_LOCAL}/v1/auth/token/create" \
    -d '{"policies":["orch-connection-credentials"],"no_default_policy":true,"ttl":"3h"}' | json_field > "${PRIVATE}/connection-vault-token"
fi
[[ -s "${PRIVATE}/connection-vault-token" ]] || { echo "scoped Vault token is missing" >&2; exit 1; }
printf 'X-Vault-Token: %s\n' "$(tr -d '\n' < "${PRIVATE}/connection-vault-token")" > "${PRIVATE}/scoped-header"
curl -fsS "${VAULT_ADDR_LOCAL}/v1/sys/health" >/dev/null 2>&1 || { echo "Vault ${VAULT_ADDR_LOCAL} is not healthy" >&2; exit 1; }

SUFFIX="$(head -c 3 /dev/urandom | od -An -tx1 | tr -d ' \n')"

# Product server: started and restarted by private scripts so the browser
# scenario can restart it mid-recording. Same fixed port, same PostgreSQL and
# Vault; inherited database/Vault/AWS/kube configuration is dropped.
PORT="${ORCH_V02_PORT:-$(free_port)}"
cat > "${PRIVATE}/start-server.sh" <<SCRIPT
#!/usr/bin/env bash
set -euo pipefail
N="\$1"
cd "${WORK}"
env -u ORCHESTRATOR_DATABASE_URL_FILE -u ORCHESTRATOR_CONNECTION_CREDENTIAL_STORE -u ORCHESTRATOR_CONNECTION_VAULT_ADDR -u ORCHESTRATOR_CONNECTION_VAULT_TOKEN_FILE \\
  -u ORCHESTRATOR_POSTGRES_TEST_URL -u KUBECONFIG -u TF_PLUGIN_CACHE_DIR -u AWS_PROFILE -u AWS_REGION -u AWS_DEFAULT_REGION \\
  -u AWS_ACCESS_KEY_ID -u AWS_SECRET_ACCESS_KEY -u AWS_SESSION_TOKEN \\
  "${WORK}/awsconnserver" -addr 127.0.0.1:${PORT} -ui-dir "${REPO}/frontend/dist" \\
  -database-url-file "${PRIVATE}/console.url" -connection-vault-address "${VAULT_ADDR_LOCAL}" \\
  -connection-vault-token-file "${PRIVATE}/connection-vault-token" -connection-vault-mount "${MOUNT}" \\
  -profile test -catalog none -score-k8s "${SCORE_K8S}" -run-id "${RUN_ID}" -credential-journal "${JOURNAL}" \\
  -aws-calls-file "${WORK}/aws-calls-\${N}.json" \\
  > "${WORK}/orchestrator-\${N}.log" 2>&1 &
echo \$! > "${PRIVATE}/server.pid"
for _ in \$(seq 1 120); do
  curl -fsS "http://127.0.0.1:${PORT}/api/v1/healthz" >/dev/null 2>&1 && exit 0
  kill -0 "\$(cat "${PRIVATE}/server.pid")" 2>/dev/null || break
  sleep 0.25
done
echo "backend did not start; see ${WORK}/orchestrator-\${N}.log" >&2
exit 1
SCRIPT
cat > "${PRIVATE}/restart-server.sh" <<SCRIPT
#!/usr/bin/env bash
set -euo pipefail
pid="\$(cat "${PRIVATE}/server.pid")"
kill -TERM "\${pid}"
for _ in \$(seq 1 40); do kill -0 "\${pid}" 2>/dev/null || break; sleep 0.25; done
kill -KILL "\${pid}" 2>/dev/null || true
echo "product server restarted (old pid \${pid})" >> "${WORK}/cleanup.txt"
exec "${PRIVATE}/start-server.sh" 2
SCRIPT
chmod 0700 "${PRIVATE}/start-server.sh" "${PRIVATE}/restart-server.sh"
"${PRIVATE}/start-server.sh" 1 || { echo "backend did not start; see ${WORK}/orchestrator-1.log" >&2; exit 1; }
API="http://127.0.0.1:${PORT}"
printf '{"scenario":"V02","runId":"%s","adapters":"fake-workloads (no Terraform, no cloud)","awsTransport":"FAKE (awsfake) for the two fixture accounts","postgres":"disposable","vault":"%s","captions":"%s"}\n' "${RUN_ID}" "$([[ ${EXISTING_VAULT} == 1 ]] && echo 'existing task Vault (scoped token)' || echo 'run-owned dev server')" "${CAPTIONS}" > "${WORK}/run.json"

video_start_xvfb "${SCREEN}"
RAW="${WORK}/V02-raw.mp4"
STATUS=0
: > "${WORK}/playwright.log"
tail -n +1 -f "${WORK}/playwright.log" &
PIDS+=($!)
DISPLAY="${VIDEO_DISPLAY}" ORCH_E2E_SCREEN="${SCREEN}" ORCH_E2E_XDOTOOL="${XDOTOOL}" ORCH_E2E_PRIVATE_DIR="${PRIVATE}" \
  ORCH_E2E_RESTART_CMD="${PRIVATE}/restart-server.sh" ORCH_V02_SUFFIX="${SUFFIX}" \
  ORCH_E2E_URL="${API}" ORCH_E2E_RUN_ID="${RUN_ID}" ORCH_E2E_EVIDENCE_DIR="${WORK}" \
  setsid node "${E2E}/v02-definitions-local.mjs" --scenario V02 --headed --captions "${CAPTIONS}" --evidence "${WORK}" \
  > "${WORK}/playwright.log" 2>&1 &
NODE_PID=$!
NODE_PGID="${NODE_PID}"
wait "${NODE_PID}" || STATUS=$?
NODE_PID=""
sleep 1

check() { printf '%s\n' "$*" | tee -a "${WORK}/checks.txt"; }
: > "${WORK}/checks.txt"
curl -fsS "${VAULT_ADDR_LOCAL}/v1/sys/health" > "${WORK}/vault-health.json" 2>/dev/null || echo '{"unreachable":true}' > "${WORK}/vault-health.json"

# Vault state of this run's two fixture accounts (identified by their own keys,
# so a shared task Vault with other data is fine): the exact-ownership journal
# holds exactly one run-owned credential object per account, each decodes to the
# credential the scenario created, and the scoped token cannot overwrite.
if [[ -s "${WORK}/accounts.json" && -s "${PRIVATE}/secret-values" ]]; then
  vget() { curl -fsS -H @"${PRIVATE}/scoped-header" "$@"; }
  for KIND in aws awsSecond; do
    KEY="$(node -e 'console.log(require(process.argv[1])[process.argv[2]])' "${WORK}/accounts.json" "${KIND}")"
    [[ "${KEY}" =~ ^[a-z0-9-]+$ ]] || { check "FAIL invalid ${KIND} account key"; STATUS=1; continue; }
    IDS="$(owned_objects | awk -v k="${KEY}" '$1==k {print $2}' | tr '\n' ' ')"
    COUNT="$(wc -w <<< "${IDS}")"
    if [[ "${COUNT}" != 1 ]]; then check "FAIL ${KIND} ${KEY}: expected 1 run-owned credential object, found ${COUNT}"; STATUS=1; continue; fi
    check "vault: ${KIND} account ${KEY} holds exactly 1 run-owned object (journal-based ownership)"
    ID="${IDS%% *}"
    [[ "${ID}" =~ ^[0-9a-f-]{36}$ ]] || { check "FAIL unexpected object id"; STATUS=1; continue; }
    vget "${VAULT_ADDR_LOCAL}/v1/${MOUNT}/data/orchestrator/connections/${VAULT_ORG}/${KEY}/credentials/${ID}" | node -e '
      const fs = require("fs");
      const kind = process.argv[1];
      const [first, second] = fs.readFileSync(process.argv[2], "utf8").split("\n");
      let s = ""; process.stdin.on("data", (d) => s += d).on("end", () => {
        const b = JSON.parse(s).data;
        const text = Buffer.from(b.data.value, "base64").toString("utf8");
        const want = kind === "aws" ? first : second;
        if (b.data.encoding !== "base64" || b.metadata.version !== 1 || !text.includes(want)) { console.error(`stored credential check failed (${kind})`); process.exit(1); }
        console.log(`vault-object ok (${kind}): version 1, base64, holds the credential the scenario created (value not printed)`);
      });
    ' "${KIND}" "${PRIVATE}/secret-values" | tee -a "${WORK}/checks.txt" || STATUS=1
    OVERWRITE="$(curl -sS -o /dev/null -w '%{http_code}' -H @"${PRIVATE}/scoped-header" -X POST "${VAULT_ADDR_LOCAL}/v1/${MOUNT}/data/orchestrator/connections/${VAULT_ORG}/${KEY}/credentials/${ID}" -d '{"data":{"encoding":"base64","value":"eA=="}}')"
    if [[ "${OVERWRITE}" == "403" ]]; then check "vault: the scoped token cannot overwrite a stored ${KIND} object (status 403)"; else check "FAIL scoped overwrite of a ${KIND} object returned ${OVERWRITE}, expected 403"; STATUS=1; fi
  done
else
  check "FAIL accounts.json or secret-values missing: the scenario did not reach the end"; STATUS=1
fi

# Stop the product server last: it writes the fake call log on SIGTERM.
stop_server
for N in 1 2; do
  [[ -s "${WORK}/aws-calls-${N}.json" ]] || { check "FAIL aws-calls-${N}.json was not written"; STATUS=1; continue; }
  node -e '
    const aws = require(process.argv[1]).calls;
    const read = /^(sts POST GetCallerIdentity|ec2 POST DescribeVpcs|rds POST DescribeDBClusters|elasticache POST DescribeReplicationGroups|iam POST SimulatePrincipalPolicy|eks GET \/clusters)$/;
    const bad = aws.filter((call) => !read.test(call));
    if (bad.length) { console.error(`non read-only or unexpected fake calls: ${bad.join("; ")}`); process.exit(1); }
    console.log(`fake calls server-${process.argv[2]}: aws=${aws.length} read-only (no create/modify/delete); the Definition steps made no provider call`);
  ' "${WORK}/aws-calls-${N}.json" "${N}" | tee -a "${WORK}/checks.txt" || STATUS=1
done

# Every retained text file is scanned for fixture keys, run secrets, Vault
# tokens and the disposable SQL credentials. Values are compared in node only.
node -e '
  const fs = require("fs");
  const path = require("path");
  const [work, priv] = process.argv.slice(1);
  const read = (file) => (fs.existsSync(file) ? fs.readFileSync(file, "utf8") : "");
  const values = new Set(["AKIAFAKESUCCESS00001", "AKIAFAKEOTHERACCT001"]);
  for (const v of read(path.join(priv, "secret-values")).split("\n")) if (v) values.add(v);
  for (const file of ["root-header", "scoped-header"]) { const m = read(path.join(priv, file)).match(/X-Vault-Token:\s*(\S+)/); if (m) values.add(m[1]); }
  const scoped = read(path.join(priv, "connection-vault-token")).trim(); if (scoped) values.add(scoped);
  try { const u = new URL(read(path.join(priv, "console.url")).trim()); if (u.password) values.add(decodeURIComponent(u.password)); } catch { /* absent */ }
  try { const u = new URL(read(process.env.ORCH_V02_PG_ADMIN_URL_FILE).trim()); if (u.password) values.add(decodeURIComponent(u.password)); } catch { /* absent */ }
  for (const v of [...values]) if (v.length < 8) values.delete(v);
  const textual = /\.(log|txt|json|srt|ass|tsv|md|env)$/;
  let scanned = 0;
  const walk = (dir) => { for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
    const full = path.join(dir, entry.name);
    if (full === priv) continue;
    if (entry.isDirectory()) { walk(full); continue; }
    if (!textual.test(entry.name)) continue;
    scanned += 1;
    const text = fs.readFileSync(full, "utf8");
    for (const v of values) if (text.includes(v)) { console.error(`credential material found in ${path.relative(work, full)}`); process.exit(1); }
  } };
  walk(work);
  if (values.size < 6) { console.error("scan had too few values to be meaningful"); process.exit(1); }
  console.log(`no-credential-in-retained-text files=${scanned} values=${values.size} (fixture keys, run secrets, Vault tokens, SQL credentials)`);
' "${WORK}" "${PRIVATE}" | tee -a "${WORK}/checks.txt" || STATUS=1

# Captions below the browser area, never over the UI; also after a failure.
CAPTIONED="${WORK}/V02-captioned.mp4"
W="${SCREEN%x*}" H="${SCREEN#*x}"
if [[ -s "${RAW}" && -s "${WORK}/captions.srt" ]]; then
  node "${E2E}/render-captions.mjs" "${WORK}/captions.srt" "${WORK}/captions.ass" "${W}" "$((H + STRIP))"
  ffmpeg -hide_banner -loglevel error -n -i "${RAW}" -vf "pad=${W}:$((H + STRIP)):0:0:black,ass=${WORK}/captions.ass" \
    -r 15 -c:v libx264 -preset veryfast -crf 24 -pix_fmt yuv420p -movflags +faststart "${CAPTIONED}" || { echo "caption burn-in failed" >&2; STATUS=1; }
fi

if [[ "${STATUS}" != 0 ]]; then
  echo "scenario V02 failed; video, marks, assertions and checks kept in ${WORK}" >&2
  exit "${STATUS}"
fi
[[ "$(<"${WORK}/result.txt")" == PASS ]] || { echo "result.txt is not PASS" >&2; exit 1; }
video_validate "${RAW}" "${SCREEN}" "${MIN_SECONDS}" 15 signed-out
ffmpeg -hide_banner -v error -xerror -i "${CAPTIONED}" -f null - 2> "${WORK}/decode-captioned.log" \
  || { echo "captioned video does not decode; see ${WORK}/decode-captioned.log" >&2; exit 1; }
[[ ! -s "${WORK}/decode-captioned.log" ]] || { echo "captioned video decode errors" >&2; exit 1; }
[[ "$(ffprobe -v error -select_streams v:0 -show_entries stream=width,height -of csv=p=0 "${CAPTIONED}")" == "${W},$((H + STRIP))" ]] \
  || { echo "captioned video has unexpected size" >&2; exit 1; }
echo "captioned=${CAPTIONED}"
