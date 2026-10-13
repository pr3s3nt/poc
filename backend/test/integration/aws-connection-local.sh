#!/usr/bin/env bash
# I04-06 local browser scenario: PE/Admin AWS access-key onboarding through the
# Web Console, recorded headed with Vietnamese captions below the browser.
#   aws-connection-local.sh --headed --captions vi --evidence NEW_PATH
#
# What is REAL: the Web Console bundle, the product HTTP handlers and
# application service, a disposable PostgreSQL database (Console store), a
# run-owned disposable Vault dev server holding the Connection credential
# objects (scoped token, KV v2) and the product's awsaccess verifier with real
# SigV4 signing.
# What is FAKE: the AWS HTTP endpoints. test/integration/awsconnserver points
# the verifier's transport at an in-process deterministic fake AWS selected by
# fixture access key ID. Workload executors are fake. A PASS here is NOT live
# proof of V2-A01/A02/A03 and no AWS endpoint is contacted.
#
# Required environment:
#   ORCH_I0406_PG_ADMIN_URL_FILE  0600 file with the administrator URL of a
#                                 DISPOSABLE PostgreSQL server you own. The run
#                                 creates one database and three roles with
#                                 random passwords and drops exactly those.
#   ORCH_I0406_VAULT_BIN          vault binary (>= 1.15) for `vault server -dev`,
#                                 or leave unset to use docker with
#                                 ORCH_I0406_VAULT_IMAGE (default hashicorp/vault:1.20).
# Optional: ORCH_VIDEO_SCREEN (1440x900), ORCH_VIDEO_MIN_SECONDS (60).
#
# NEW_PATH must not exist and must be outside the repository; it is kept, also
# on failure. Output: I04-06-raw.mp4, I04-06-captioned.mp4, captions.srt/.ass,
# marks.json, assertions.txt, result.txt, run.json, checks.txt, aws-calls.json,
# orchestrator.log, vault-health.json, cleanup.txt, frames/*.png. Credentials
# (including the raw Vault dev-server output, which holds its root token and
# unseal key) live in NEW_PATH/private (0700), removed on exit, and never in
# the evidence; every retained text file is scanned for them.
# Debug only: ORCH_I0406_INJECT_FAILURE=after-success fails the scenario on
# purpose to check that video, marks and assertions survive a failure.
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
[[ -n "${ORCH_I0406_PG_ADMIN_URL_FILE:-}" && -f "${ORCH_I0406_PG_ADMIN_URL_FILE}" ]] || { echo "ORCH_I0406_PG_ADMIN_URL_FILE must name the administrator URL file of a disposable PostgreSQL server" >&2; exit 2; }
command -v node >/dev/null || { echo "node is required" >&2; exit 1; }
node "${E2E}/aws-connection-local.mjs" --check I04-06 || exit 2

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
RUN_ID="i0406-$(date -u +%Y%m%d%H%M%S)-${RANDOM}"
SCREEN="${ORCH_VIDEO_SCREEN:-1440x900}"
MIN_SECONDS="${ORCH_VIDEO_MIN_SECONDS:-60}"
STRIP=96
VAULT_IMAGE="${ORCH_I0406_VAULT_IMAGE:-hashicorp/vault:1.20}"
VAULT_CONTAINER="" VAULT_PID="" SERVER_PID="" NODE_PID="" NODE_PGID="" PROVISIONED=0 DROP_FAILED=0
PIDS=()
# shellcheck source=video-lib.sh
source "${ROOT}/test/integration/video-lib.sh"

: > "${WORK}/cleanup.txt"
note() { printf '%s\n' "$*" >> "${WORK}/cleanup.txt"; }

# Stops only what this run started and removes only what it created.
cleanup() {
  local status=$?
  local pid
  if [[ -n "${NODE_PID}" ]] && kill -0 "${NODE_PID}" 2>/dev/null; then
    kill -TERM "${NODE_PID}" 2>/dev/null || true
    for _ in $(seq 1 80); do kill -0 "${NODE_PID}" 2>/dev/null || break; sleep 0.5; done
  fi
  if [[ -n "${NODE_PGID}" ]] && kill -0 -- "-${NODE_PGID}" 2>/dev/null; then kill -KILL -- "-${NODE_PGID}" 2>/dev/null || true; fi
  if [[ -n "${NODE_PID}" ]]; then wait "${NODE_PID}" 2>/dev/null || true; fi
  # The product server first (it writes aws-calls.json on SIGTERM).
  if [[ -n "${SERVER_PID}" ]] && kill -0 "${SERVER_PID}" 2>/dev/null; then
    kill -TERM "${SERVER_PID}" 2>/dev/null || true
    for _ in $(seq 1 40); do kill -0 "${SERVER_PID}" 2>/dev/null || break; sleep 0.25; done
    kill -KILL "${SERVER_PID}" 2>/dev/null || true
    wait "${SERVER_PID}" 2>/dev/null || true
    note "awsconnserver stopped pid=${SERVER_PID}"
  fi
  if [[ "${PROVISIONED}" == 1 ]]; then
    if "${WORK}/awsconnserver" drop -admin-url-file "${ORCH_I0406_PG_ADMIN_URL_FILE}" -manifest "${PRIVATE}/manifest.json" >> "${WORK}/cleanup.txt" 2>&1; then note "disposable database and roles dropped"; else note "FAILED to drop the disposable database; manifest kept in ${PRIVATE}"; DROP_FAILED=1; status=1; fi
  fi
  for pid in "${PIDS[@]}"; do kill "${pid}" 2>/dev/null || true; done
  for pid in "${PIDS[@]}"; do wait "${pid}" 2>/dev/null || true; done
  if [[ -n "${VAULT_PID}" ]]; then kill "${VAULT_PID}" 2>/dev/null || true; wait "${VAULT_PID}" 2>/dev/null || true; note "vault dev server stopped pid=${VAULT_PID}"; fi
  if [[ -n "${VAULT_CONTAINER}" ]]; then
    docker rm -f "${VAULT_CONTAINER}" >/dev/null 2>&1 || true
    if docker container inspect "${VAULT_CONTAINER}" >/dev/null 2>&1; then note "WARNING vault container ${VAULT_CONTAINER} still present"; status=1; else note "vault container ${VAULT_CONTAINER} removed"; fi
  fi
  # Private credentials are always removed; only a failed database drop keeps
  # the manifest (database name and role names, no password) for a retry.
  if [[ "${DROP_FAILED}" == 1 ]]; then
    find "${PRIVATE}" -mindepth 1 ! -name manifest.json -delete 2>/dev/null || true
  else
    rm -rf "${PRIVATE}"
  fi
  if [[ ! -e "${PRIVATE}" ]]; then note "private credential directory removed"; else note "only manifest.json kept (database drop failed): ${PRIVATE}"; fi
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

# Test-only launcher (fake AWS transport) and the Console login database.
(cd "${ROOT}" && go build -o "${WORK}/awsconnserver" ./test/integration/awsconnserver)
"${WORK}/awsconnserver" provision -admin-url-file "${ORCH_I0406_PG_ADMIN_URL_FILE}" -out-dir "${PRIVATE}" | tee -a "${WORK}/cleanup.txt"
PROVISIONED=1

# Run-owned Vault dev server, KV v2 mount "secret", scoped token for the
# Connection credential namespace only (create/read objects, no overwrite).
synthetic_token() { head -c 32 /dev/urandom | od -An -tx1 | tr -d ' \n'; }
free_port() { node -e 'const s=require("net").createServer();s.listen(0,"127.0.0.1",()=>{console.log(s.address().port);s.close()})'; }
ROOT_TOKEN="$(synthetic_token)"
printf 'X-Vault-Token: %s\n' "${ROOT_TOKEN}" > "${PRIVATE}/root-header"
VAULT_PORT="$(free_port)"
VAULT_ADDR_LOCAL="http://127.0.0.1:${VAULT_PORT}"
VAULT_BIN="${ORCH_I0406_VAULT_BIN:-}"
if [[ -n "${VAULT_BIN}" ]]; then
  [[ -x "${VAULT_BIN}" ]] || { echo "ORCH_I0406_VAULT_BIN is not executable" >&2; exit 1; }
  VAULT_DEV_ROOT_TOKEN_ID="${ROOT_TOKEN}" "${VAULT_BIN}" server -dev -dev-listen-address "127.0.0.1:${VAULT_PORT}" -dev-no-store-token > "${PRIVATE}/vault.log" 2>&1 &
  VAULT_PID=$!
else
  command -v docker >/dev/null || { echo "set ORCH_I0406_VAULT_BIN or install docker for the run-owned Vault" >&2; exit 1; }
  VAULT_CONTAINER="orch-${RUN_ID}"
  printf 'VAULT_DEV_ROOT_TOKEN_ID=%s\nVAULT_DEV_LISTEN_ADDRESS=0.0.0.0:8200\n' "${ROOT_TOKEN}" > "${PRIVATE}/vault.env"
  docker run -d --rm --name "${VAULT_CONTAINER}" --label "orchestrator.run-id=${RUN_ID}" --cap-add IPC_LOCK \
    --env-file "${PRIVATE}/vault.env" -p "127.0.0.1:${VAULT_PORT}:8200" "${VAULT_IMAGE}" server -dev >/dev/null
fi
unset ROOT_TOKEN
for _ in $(seq 1 60); do curl -fsS "${VAULT_ADDR_LOCAL}/v1/sys/health" >/dev/null 2>&1 && break; sleep 0.5; done
curl -fsS "${VAULT_ADDR_LOCAL}/v1/sys/health" >/dev/null || { echo "Vault did not become healthy" >&2; exit 1; }
curl -fsS -H @"${PRIVATE}/root-header" -X PUT "${VAULT_ADDR_LOCAL}/v1/sys/policies/acl/orch-connection-credentials" \
  -d '{"policy":"path \"secret/data/orchestrator/connections/*\" { capabilities = [\"create\", \"read\"] }\npath \"secret/metadata/orchestrator/connections/*\" { capabilities = [\"read\", \"list\", \"delete\"] }\n"}' >/dev/null
curl -fsS -H @"${PRIVATE}/root-header" -X POST "${VAULT_ADDR_LOCAL}/v1/auth/token/create" \
  -d '{"policies":["orch-connection-credentials"],"no_default_policy":true,"ttl":"2h"}' \
  | node -e 'let s="";process.stdin.on("data",d=>s+=d).on("end",()=>{const t=JSON.parse(s).auth?.client_token;if(!t)process.exit(1);process.stdout.write(t)})' \
  > "${PRIVATE}/connection-vault-token"
[[ -s "${PRIVATE}/connection-vault-token" ]] || { echo "scoped Vault token was not created" >&2; exit 1; }
printf 'X-Vault-Token: %s\n' "$(<"${PRIVATE}/connection-vault-token")" > "${PRIVATE}/scoped-header"

# Isolated product server: inherited database/Vault/AWS/kube configuration is
# dropped; no host credential can reach it.
env -u ORCHESTRATOR_DATABASE_URL_FILE -u ORCHESTRATOR_CONNECTION_CREDENTIAL_STORE -u ORCHESTRATOR_CONNECTION_VAULT_ADDR -u ORCHESTRATOR_CONNECTION_VAULT_TOKEN_FILE \
  -u ORCHESTRATOR_POSTGRES_TEST_URL -u KUBECONFIG -u TF_PLUGIN_CACHE_DIR -u AWS_PROFILE -u AWS_REGION -u AWS_DEFAULT_REGION \
  -u AWS_ACCESS_KEY_ID -u AWS_SECRET_ACCESS_KEY -u AWS_SESSION_TOKEN \
  "${WORK}/awsconnserver" -addr 127.0.0.1:0 -addr-file "${WORK}/api-addr" -ui-dir "${REPO}/frontend/dist" \
  -database-url-file "${PRIVATE}/console.url" -connection-vault-address "${VAULT_ADDR_LOCAL}" \
  -connection-vault-token-file "${PRIVATE}/connection-vault-token" -connection-vault-mount secret \
  -profile test -run-id "${RUN_ID}" -aws-calls-file "${WORK}/aws-calls.json" \
  > "${WORK}/orchestrator.log" 2>&1 &
SERVER_PID=$!
for _ in $(seq 1 120); do [[ -s "${WORK}/api-addr" ]] && break; kill -0 "${SERVER_PID}" 2>/dev/null || break; sleep 0.25; done
[[ -s "${WORK}/api-addr" ]] || { echo "backend did not start; see ${WORK}/orchestrator.log" >&2; exit 1; }
API="http://$(<"${WORK}/api-addr")"
for _ in $(seq 1 40); do curl -fsS "${API}/api/v1/healthz" >/dev/null 2>&1 && break; sleep 0.25; done
curl -fsS "${API}/api/v1/healthz" >/dev/null
printf '{"scenario":"I04-06","runId":"%s","adapters":"fake-workloads","awsTransport":"FAKE (awsfake)","postgres":"disposable","vault":"run-owned dev server","captions":"%s"}\n' "${RUN_ID}" "${CAPTIONS}" > "${WORK}/run.json"

video_start_xvfb "${SCREEN}"
RAW="${WORK}/I04-06-raw.mp4"
STATUS=0
: > "${WORK}/playwright.log"
tail -n +1 -f "${WORK}/playwright.log" &
PIDS+=($!)
DISPLAY="${VIDEO_DISPLAY}" ORCH_E2E_SCREEN="${SCREEN}" ORCH_E2E_XDOTOOL="${XDOTOOL}" ORCH_E2E_PRIVATE_DIR="${PRIVATE}" \
  ORCH_E2E_URL="${API}" ORCH_E2E_RUN_ID="${RUN_ID}" ORCH_E2E_EVIDENCE_DIR="${WORK}" \
  setsid node "${E2E}/aws-connection-local.mjs" --scenario I04-06 --headed --captions "${CAPTIONS}" --evidence "${WORK}" \
  > "${WORK}/playwright.log" 2>&1 &
NODE_PID=$!
NODE_PGID="${NODE_PID}"
wait "${NODE_PID}" || STATUS=$?
NODE_PID=""
sleep 1

# Post-run checks. Key values are compared inside node only; output names the
# checks, never a value.
check() { printf '%s\n' "$*" | tee -a "${WORK}/checks.txt"; }
: > "${WORK}/checks.txt"
# Safe Vault diagnostics only (no token, no raw server output).
curl -fsS "${VAULT_ADDR_LOCAL}/v1/sys/health" > "${WORK}/vault-health.json" 2>/dev/null || echo '{"unreachable":true}' > "${WORK}/vault-health.json"
# Every retained text file is scanned for the AWS fixture keys, the Vault root
# and scoped tokens and the disposable SQL credentials. Values are compared in
# node only; output names the checks, never a value.
node -e '
  const fs = require("fs");
  const path = require("path");
  const [work, priv] = process.argv.slice(1);
  const read = (file) => (fs.existsSync(file) ? fs.readFileSync(file, "utf8") : "");
  const values = new Set(["AKIAFAKESUCCESS00001", "AKIAFAKEREJECTED0001", "AKIAFAKENOPERMS00001", "AKIAFAKENOSIMULATE01"]);
  for (const v of read(path.join(priv, "secret-values")).split("\n")) if (v) values.add(v);
  for (const file of ["root-header", "scoped-header"]) { const m = read(path.join(priv, file)).match(/X-Vault-Token:\s*(\S+)/); if (m) values.add(m[1]); }
  const scoped = read(path.join(priv, "connection-vault-token")).trim(); if (scoped) values.add(scoped);
  try { const u = new URL(read(path.join(priv, "console.url")).trim()); if (u.password) values.add(decodeURIComponent(u.password)); } catch { /* absent */ }
  try { const u = new URL(read(process.env.ORCH_I0406_PG_ADMIN_URL_FILE).trim()); if (u.password) values.add(decodeURIComponent(u.password)); } catch { /* absent */ }
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
  console.log(`no-credential-in-retained-text files=${scanned} values=${values.size} (AWS fixture keys, Vault root and scoped tokens, SQL credentials)`);
' "${WORK}" "${PRIVATE}" | tee -a "${WORK}/checks.txt" || STATUS=1

if [[ -s "${WORK}/connection.json" && -s "${PRIVATE}/secret-values" ]]; then
  KEY="$(node -e 'console.log(require(process.argv[1]).key)' "${WORK}/connection.json")"
  [[ "${KEY}" =~ ^[a-z0-9-]+$ ]] || { echo "invalid connection key" >&2; STATUS=1; }
  # Exactly one credential namespace exists under the organization: only the
  # successful registration left an object; every failed attempt left none.
  LISTING="$(curl -fsS -H @"${PRIVATE}/scoped-header" -X LIST "${VAULT_ADDR_LOCAL}/v1/secret/metadata/orchestrator/connections/acme" | node -e 'let s="";process.stdin.on("data",d=>s+=d).on("end",()=>console.log(JSON.parse(s).data.keys.join(",")))' || true)"
  if [[ "${LISTING}" == "${KEY}/" ]]; then check "vault: only the registered connection has a credential namespace (${KEY}); failed attempts left none"; else check "FAIL vault namespaces under the organization: ${LISTING}"; STATUS=1; fi
  OBJECT="$(curl -fsS -H @"${PRIVATE}/scoped-header" -X LIST "${VAULT_ADDR_LOCAL}/v1/secret/metadata/orchestrator/connections/acme/${KEY}/credentials" | node -e 'let s="";process.stdin.on("data",d=>s+=d).on("end",()=>{const k=JSON.parse(s).data.keys;if(k.length!==1)process.exit(1);process.stdout.write(k[0])})' || true)"
  if [[ "${OBJECT}" =~ ^[0-9a-f-]{36}$ ]]; then
    curl -fsS -H @"${PRIVATE}/scoped-header" "${VAULT_ADDR_LOCAL}/v1/secret/data/orchestrator/connections/acme/${KEY}/credentials/${OBJECT}" | node -e '
      const fs = require("fs");
      let s = ""; process.stdin.on("data", (d) => s += d).on("end", () => {
        const body = JSON.parse(s).data;
        const stored = JSON.parse(Buffer.from(body.data.value, "base64").toString("utf8"));
        const [secret] = fs.readFileSync(process.argv[1], "utf8").split("\n");
        const checks = {
          encoding: body.data.encoding === "base64",
          fields: JSON.stringify(Object.keys(stored).sort()) === JSON.stringify(["accessKeyId", "secretAccessKey"]),
          keyId: stored.accessKeyId === "AKIAFAKESUCCESS00001",
          secret: stored.secretAccessKey === secret,
          singleVersion: body.metadata.version === 1,
        };
        const failed = Object.entries(checks).filter(([, ok]) => !ok).map(([name]) => name);
        if (failed.length) { console.error(`stored credential check failed: ${failed.join(",")}`); process.exit(1); }
        console.log("vault-object ok: " + Object.keys(checks).join(",") + "=true (values not printed)");
      });
    ' "${PRIVATE}/secret-values" | tee -a "${WORK}/checks.txt" || STATUS=1
    OVERWRITE="$(curl -sS -o /dev/null -w '%{http_code}' -H @"${PRIVATE}/scoped-header" -X POST "${VAULT_ADDR_LOCAL}/v1/secret/data/orchestrator/connections/acme/${KEY}/credentials/${OBJECT}" -d '{"data":{"encoding":"base64","value":"eA=="}}')"
    if [[ "${OVERWRITE}" == "403" ]]; then check "vault: the scoped token cannot overwrite the stored object (status 403)"; else check "FAIL scoped overwrite returned ${OVERWRITE}, expected 403"; STATUS=1; fi
  else
    check "FAIL expected exactly one credential object for ${KEY}"; STATUS=1
  fi
fi

# Fake AWS call log: registration may use only read-only calls.
kill -TERM "${SERVER_PID}" 2>/dev/null || true
for _ in $(seq 1 40); do kill -0 "${SERVER_PID}" 2>/dev/null || break; sleep 0.25; done
wait "${SERVER_PID}" 2>/dev/null || true
SERVER_PID=""
if [[ -s "${WORK}/aws-calls.json" ]]; then
  node -e '
    const calls = require(process.argv[1]).calls;
    const read = /^(sts POST GetCallerIdentity|ec2 POST DescribeVpcs|rds POST DescribeDBClusters|elasticache POST DescribeReplicationGroups|iam POST SimulatePrincipalPolicy|eks GET \/clusters)$/;
    const bad = calls.filter((call) => !read.test(call));
    if (bad.length || calls.length === 0) { console.error(`non read-only or missing AWS calls: ${bad.join("; ") || "none recorded"}`); process.exit(1); }
    console.log(`fake-aws read-only calls=${calls.length} distinct=${new Set(calls).size} (no create/modify/delete)`);
  ' "${WORK}/aws-calls.json" | tee -a "${WORK}/checks.txt" || STATUS=1
else
  check "FAIL aws-calls.json was not written"; STATUS=1
fi

# Captions below the browser area, never over the UI; also after a failure.
CAPTIONED="${WORK}/I04-06-captioned.mp4"
W="${SCREEN%x*}" H="${SCREEN#*x}"
if [[ -s "${RAW}" && -s "${WORK}/captions.srt" ]]; then
  node "${E2E}/render-captions.mjs" "${WORK}/captions.srt" "${WORK}/captions.ass" "${W}" "$((H + STRIP))"
  ffmpeg -hide_banner -loglevel error -n -i "${RAW}" -vf "pad=${W}:$((H + STRIP)):0:0:black,ass=${WORK}/captions.ass" \
    -r 15 -c:v libx264 -preset veryfast -crf 24 -pix_fmt yuv420p -movflags +faststart "${CAPTIONED}" || { echo "caption burn-in failed" >&2; STATUS=1; }
fi

if [[ "${STATUS}" != 0 ]]; then
  echo "scenario I04-06 failed; video, marks, assertions and checks kept in ${WORK}" >&2
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
