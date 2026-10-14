#!/usr/bin/env bash
# I04-08 local browser scenario: a Platform Engineer deletes an UNREFERENCED
# resource together with an opt-in dependency through the Web Console, a Developer
# is denied, recorded headed with Vietnamese captions below the browser.
#   resource-delete-local.sh --headed --captions vi --evidence NEW_PATH
#
# What is REAL: the Web Console bundle, the product HTTP handlers and services, the
# REAL tfexec cleanup engine and encrypted Terraform state backend driving the
# pinned terraform 1.9.8 binary, a disposable PostgreSQL database (Console and
# State logins; a reopen check restarts the server on it) and a run-owned
# disposable Vault dev server holding the Connection credential objects (scoped
# token, KV v2). score-k8s is not used: the scenario renders no Workload.
# What is FAKE: the resources (terraform_data objects of one hand-built module in a
# local state: no provider, cloud or cluster), the provider binder and readiness
# check, and the topology, which the launcher arranges before the browser starts
# (the Application itself is created through the product API); one destroy-time
# local-exec fails on purpose for the partial-failure step. A PASS here is NOT live
# proof of V2-A01/A02/A03; no cloud, cluster or retained database is touched and
# nothing is deployed.
#
# Required environment:
#   ORCH_I0408_PG_ADMIN_URL_FILE  0600 file with the administrator URL of a
#                                 DISPOSABLE PostgreSQL server you own. The run
#                                 creates one database and three roles with
#                                 random passwords and drops exactly those.
#   ORCH_I0408_TERRAFORM          path of the pinned terraform 1.9.8 binary; its
#                                 version and SHA-256 are compared with
#                                 internal/adapters/terraform/registry/release.json
#                                 and the verified copy is the one that runs.
#   ORCH_I0408_VAULT_BIN          vault binary (>= 1.15) for `vault server -dev`,
#                                 or leave unset to use docker with
#                                 ORCH_I0408_VAULT_IMAGE (default hashicorp/vault:1.20).
# Optional: ORCH_VIDEO_SCREEN (1440x900), ORCH_VIDEO_MIN_SECONDS (60).
#
# NEW_PATH must not exist and must be outside the repository; it is kept, also
# on failure. Output: I04-08-raw.mp4, I04-08-captioned.mp4, captions.srt/.ass,
# marks.json, assertions.txt, result.txt, run.json, checks.txt, scenario.json,
# outcome.json, orchestrator.log, orchestrator-reopen.log, vault-health.json,
# cleanup.txt. Credentials (the raw Vault dev-server output holds its root token,
# the state key and the Terraform work directory) live in NEW_PATH/private (0700),
# are removed on exit and never in the evidence; every retained text file is
# scanned for them.
set -euo pipefail
umask 077
# xdotool types the Vietnamese option names of native selects; it needs a UTF-8 locale.
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
[[ "${HEADED}" == 1 && -n "${CAPTIONS}" && -n "${EVIDENCE}" ]] || usage
[[ "${CAPTIONS}" == vi ]] || { echo "unsupported captions ${CAPTIONS}; only vi" >&2; exit 2; }
[[ -n "${ORCH_I0408_PG_ADMIN_URL_FILE:-}" && -f "${ORCH_I0408_PG_ADMIN_URL_FILE}" ]] || { echo "ORCH_I0408_PG_ADMIN_URL_FILE must name the administrator URL file of a disposable PostgreSQL server" >&2; exit 2; }
command -v node >/dev/null || { echo "node is required" >&2; exit 1; }
TERRAFORM_SRC="${ORCH_I0408_TERRAFORM:-}"
[[ -n "${TERRAFORM_SRC}" && -x "${TERRAFORM_SRC}" ]] || { echo "set ORCH_I0408_TERRAFORM to the pinned terraform 1.9.8 binary" >&2; exit 2; }
RELEASE="${ROOT}/internal/adapters/terraform/registry/release.json"
PIN_VERSION="$(node -e 'process.stdout.write(JSON.parse(require("fs").readFileSync(process.argv[1],"utf8")).terraform.version)' "${RELEASE}")"
PIN_SHA="$(node -e 'process.stdout.write(JSON.parse(require("fs").readFileSync(process.argv[1],"utf8")).terraform.binarySha256)' "${RELEASE}")"
[[ "$(sha256sum "${TERRAFORM_SRC}" | cut -d' ' -f1)" == "${PIN_SHA}" ]] || { echo "ORCH_I0408_TERRAFORM does not match the pinned SHA-256 of release.json" >&2; exit 2; }
"${TERRAFORM_SRC}" version | head -n 1 | grep -q "^Terraform v${PIN_VERSION}\$" || { echo "ORCH_I0408_TERRAFORM is not terraform ${PIN_VERSION}" >&2; exit 2; }
node "${E2E}/resource-delete-local.mjs" --check I04-08 || exit 2

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
RUN_ID="i0408-$(date -u +%Y%m%d%H%M%S)-${RANDOM}"
SCREEN="${ORCH_VIDEO_SCREEN:-1440x900}"
MIN_SECONDS="${ORCH_VIDEO_MIN_SECONDS:-60}"
STRIP=96
VAULT_IMAGE="${ORCH_I0408_VAULT_IMAGE:-hashicorp/vault:1.20}"
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
  # The product server first.
  if [[ -n "${SERVER_PID}" ]] && kill -0 "${SERVER_PID}" 2>/dev/null; then
    kill -TERM "${SERVER_PID}" 2>/dev/null || true
    for _ in $(seq 1 40); do kill -0 "${SERVER_PID}" 2>/dev/null || break; sleep 0.25; done
    kill -KILL "${SERVER_PID}" 2>/dev/null || true
    wait "${SERVER_PID}" 2>/dev/null || true
    note "resdelserver stopped pid=${SERVER_PID}"
  fi
  if [[ "${PROVISIONED}" == 1 ]]; then
    if "${WORK}/resdelserver" drop -admin-url-file "${ORCH_I0408_PG_ADMIN_URL_FILE}" -manifest "${PRIVATE}/manifest.json" >> "${WORK}/cleanup.txt" 2>&1; then note "disposable database and roles dropped"; else note "FAILED to drop the disposable database; manifest kept in ${PRIVATE}"; DROP_FAILED=1; status=1; fi
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

# Test-only launcher (real PostgreSQL/Vault/tfexec engine over fake terraform_data resources) and the Console login database.
(cd "${ROOT}" && go build -o "${WORK}/resdelserver" ./test/integration/resdelserver)
"${WORK}/resdelserver" provision -admin-url-file "${ORCH_I0408_PG_ADMIN_URL_FILE}" -out-dir "${PRIVATE}" | tee -a "${WORK}/cleanup.txt"
PROVISIONED=1
# State encryption key (random, 0600) and the pinned Terraform copy that runs. The
# launcher keeps its artifacts and encrypted recovery files in a private work dir.
mkdir -m 0700 "${PRIVATE}/bin" "${PRIVATE}/tfwork"
cp "${TERRAFORM_SRC}" "${PRIVATE}/bin/terraform"
chmod 0500 "${PRIVATE}/bin/terraform"
[[ "$(sha256sum "${PRIVATE}/bin/terraform" | cut -d' ' -f1)" == "${PIN_SHA}" ]] || { echo "the copied terraform changed" >&2; exit 1; }
head -c 32 /dev/urandom | od -An -tx1 | tr -d ' \n' > "${PRIVATE}/state.key"

# Run-owned Vault dev server, KV v2 mount "secret", scoped token for the
# Connection credential namespace only (create/read objects, no overwrite).
synthetic_token() { head -c 32 /dev/urandom | od -An -tx1 | tr -d ' \n'; }
free_port() { node -e 'const s=require("net").createServer();s.listen(0,"127.0.0.1",()=>{console.log(s.address().port);s.close()})'; }
ROOT_TOKEN="$(synthetic_token)"
printf 'X-Vault-Token: %s\n' "${ROOT_TOKEN}" > "${PRIVATE}/root-header"
VAULT_PORT="$(free_port)"
VAULT_ADDR_LOCAL="http://127.0.0.1:${VAULT_PORT}"
VAULT_BIN="${ORCH_I0408_VAULT_BIN:-}"
if [[ -n "${VAULT_BIN}" ]]; then
  [[ -x "${VAULT_BIN}" ]] || { echo "ORCH_I0408_VAULT_BIN is not executable" >&2; exit 1; }
  VAULT_DEV_ROOT_TOKEN_ID="${ROOT_TOKEN}" "${VAULT_BIN}" server -dev -dev-listen-address "127.0.0.1:${VAULT_PORT}" -dev-no-store-token > "${PRIVATE}/vault.log" 2>&1 &
  VAULT_PID=$!
else
  command -v docker >/dev/null || { echo "set ORCH_I0408_VAULT_BIN or install docker for the run-owned Vault" >&2; exit 1; }
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
start_server() {
  # $1 addr file, $2 log, remaining: extra launcher flags. Inherited database,
  # Vault, AWS, kube and Terraform configuration is dropped; no host credential can reach it.
  local addr_file="$1" log="$2"; shift 2
  local unset_tf=()
  local name
  while IFS= read -r name; do unset_tf+=(-u "${name}"); done < <(env | sed -n 's/^\(TF_[A-Za-z0-9_]*\)=.*/\1/p')
  env -u ORCHESTRATOR_DATABASE_URL_FILE -u ORCHESTRATOR_CONNECTION_CREDENTIAL_STORE -u ORCHESTRATOR_CONNECTION_VAULT_ADDR -u ORCHESTRATOR_CONNECTION_VAULT_TOKEN_FILE \
    -u ORCHESTRATOR_POSTGRES_TEST_URL -u KUBECONFIG -u TF_PLUGIN_CACHE_DIR -u AWS_PROFILE -u AWS_REGION -u AWS_DEFAULT_REGION \
    -u AWS_ACCESS_KEY_ID -u AWS_SECRET_ACCESS_KEY -u AWS_SESSION_TOKEN "${unset_tf[@]}" \
    "${WORK}/resdelserver" -addr 127.0.0.1:0 -addr-file "${addr_file}" -ui-dir "${REPO}/frontend/dist" \
    -database-url-file "${PRIVATE}/console.url" -state-database-url-file "${PRIVATE}/state.url" \
    -terraform-state-key-file "${PRIVATE}/state.key" -terraform "${PRIVATE}/bin/terraform" -work-dir "${PRIVATE}/tfwork" \
    -connection-vault-address "${VAULT_ADDR_LOCAL}" -connection-vault-token-file "${PRIVATE}/connection-vault-token" -connection-vault-mount secret \
    -profile test -run-id "${RUN_ID}" "$@" > "${log}" 2>&1 &
}
start_server "${WORK}/api-addr" "${WORK}/orchestrator.log" -seed -scenario-file "${WORK}/scenario.json"
SERVER_PID=$!
for _ in $(seq 1 480); do [[ -s "${WORK}/api-addr" ]] && break; kill -0 "${SERVER_PID}" 2>/dev/null || break; sleep 0.25; done
[[ -s "${WORK}/api-addr" ]] || { echo "backend did not start; see ${WORK}/orchestrator.log" >&2; exit 1; }
API="http://$(<"${WORK}/api-addr")"
for _ in $(seq 1 40); do curl -fsS "${API}/api/v1/healthz" >/dev/null 2>&1 && break; sleep 0.25; done
curl -fsS "${API}/api/v1/healthz" >/dev/null
printf '{"scenario":"I04-08","runId":"%s","engine":"REAL tfexec cleanup engine, pinned terraform %s","resources":"FAKE terraform_data objects (no provider, cloud or cluster)","binder":"FAKE","postgres":"disposable","vault":"run-owned dev server","captions":"%s"}\n' "${RUN_ID}" "${PIN_VERSION}" "${CAPTIONS}" > "${WORK}/run.json"

video_start_xvfb "${SCREEN}"
RAW="${WORK}/I04-08-raw.mp4"
STATUS=0
: > "${WORK}/playwright.log"
tail -n +1 -f "${WORK}/playwright.log" &
PIDS+=($!)
DISPLAY="${VIDEO_DISPLAY}" ORCH_E2E_SCREEN="${SCREEN}" ORCH_E2E_XDOTOOL="${XDOTOOL}" ORCH_E2E_PRIVATE_DIR="${PRIVATE}" ORCH_E2E_RESDEL="${WORK}/resdelserver" \
  ORCH_E2E_URL="${API}" ORCH_E2E_RUN_ID="${RUN_ID}" ORCH_E2E_EVIDENCE_DIR="${WORK}" \
  setsid node "${E2E}/resource-delete-local.mjs" --scenario I04-08 --headed --captions "${CAPTIONS}" --evidence "${WORK}" \
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
# Every retained text file is scanned for the Vault root and scoped tokens and
# the disposable SQL credentials. Values are compared in node only; output names
# the checks, never a value.
node -e '
  const fs = require("fs");
  const path = require("path");
  const [work, priv] = process.argv.slice(1);
  const read = (file) => (fs.existsSync(file) ? fs.readFileSync(file, "utf8") : "");
  const values = new Set();
  for (const file of ["root-header", "scoped-header"]) { const m = read(path.join(priv, file)).match(/X-Vault-Token:\s*(\S+)/); if (m) values.add(m[1]); }
  const scoped = read(path.join(priv, "connection-vault-token")).trim(); if (scoped) values.add(scoped);
  const stateKey = read(path.join(priv, "state.key")).trim(); if (stateKey) values.add(stateKey);
  for (const name of ["console.url", "state.url", "owner.url"]) { try { const u = new URL(read(path.join(priv, name)).trim()); if (u.password) values.add(decodeURIComponent(u.password)); } catch { /* absent */ } }
  try { const u = new URL(read(process.env.ORCH_I0408_PG_ADMIN_URL_FILE).trim()); if (u.password) values.add(decodeURIComponent(u.password)); } catch { /* absent */ }
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
    for (const marker of [/AKIA[0-9A-Z]{16}/, /-----BEGIN [A-Z ]*PRIVATE KEY-----/, /vault:\/\/[^\s"]+/, /"serial"\s*:\s*\d+/]) if (marker.test(text)) { console.error(`private marker ${marker} found in ${path.relative(work, full)}`); process.exit(1); }
    for (const v of values) if (text.includes(v)) { console.error(`credential material found in ${path.relative(work, full)}`); process.exit(1); }
  } };
  walk(work);
  if (values.size < 4) { console.error("scan had too few values to be meaningful"); process.exit(1); }
  console.log(`no-credential-in-retained-text files=${scanned} values=${values.size} (Vault root and scoped tokens, the state encryption key, SQL credentials of all three logins)`);
' "${WORK}" "${PRIVATE}" | tee -a "${WORK}/checks.txt" || STATUS=1

# SQL reopen: stop the server, start a new process on the same PostgreSQL
# database and read the registered Type, classes and Definitions back through
# the public API as a Platform Engineer. Nothing is written.
kill -TERM "${SERVER_PID}" 2>/dev/null || true
for _ in $(seq 1 40); do kill -0 "${SERVER_PID}" 2>/dev/null || break; sleep 0.25; done
wait "${SERVER_PID}" 2>/dev/null || true
SERVER_PID=""
rm -f "${WORK}/api-addr-reopen"
start_server "${WORK}/api-addr-reopen" "${WORK}/orchestrator-reopen.log"
SERVER_PID=$!
for _ in $(seq 1 120); do [[ -s "${WORK}/api-addr-reopen" ]] && break; kill -0 "${SERVER_PID}" 2>/dev/null || break; sleep 0.25; done
if [[ -s "${WORK}/api-addr-reopen" ]]; then
  REOPEN_API="http://$(<"${WORK}/api-addr-reopen")"
  node -e '
    const base = process.argv[1];
    const scenario = JSON.parse(require("fs").readFileSync(process.argv[2], "utf8"));
    const outcome = JSON.parse(require("fs").readFileSync(process.argv[3], "utf8"));
    (async () => {
      const signIn = await fetch(`${base}/api/v1/auth/sign-in`, { method: "POST", headers: { "content-type": "application/json" }, body: JSON.stringify({ username: "platform-engineer", password: "test-password" }) });
      if (!signIn.ok) throw new Error("sign-in after reopen failed");
      const cookie = (signIn.headers.getSetCookie?.() ?? []).map((value) => value.split(";")[0]).join("; ");
      const scope = `${base}/api/v1/applications/${scenario.applicationId}/environments/staging`;
      const get = async (path) => (await fetch(`${scope}${path}`, { headers: { cookie } })).json();
      const failures = [];
      const list = (await get("/resources")).resources ?? [];
      const byId = Object.fromEntries(list.map((item) => [item.id, item.status]));
      for (const [key, id] of Object.entries(scenario.resources)) {
        const want = outcome.afterSecond[key];
        if (byId[id] !== want) failures.push(`${key} is ${byId[id]} after the restart, was ${want}`);
      }
      if (outcome.afterSecond.root1 !== "DESTROYED" || outcome.afterSecond.ordersNet !== "DESTROYED") failures.push("the first cleanup did not destroy its two confirmed resources");
      if (["sessions", "sharedNet", "liveCache", "auditNet"].some((key) => outcome.afterSecond[key] === "DESTROYED")) failures.push("an unconfirmed resource was destroyed");
      const first = (await get(`/resource-cleanups/${outcome.firstOperation}`)).cleanup;
      if (!first || first.status !== "SUCCEEDED" || first.allVerified !== true || first.authorizationMode !== "USER_CONFIRMED" || first.resources?.length !== 2) failures.push("the first cleanup differs after the restart");
      const second = (await get(`/resources/${scenario.resources.root2}`)).resource?.cleanup;
      const secondOp = second ? (await get(`/resource-cleanups/${second.operationId}`)).cleanup : undefined;
      if (!secondOp || secondOp.status !== "FAILED" || secondOp.allVerified === true) failures.push("the partial-failure cleanup differs after the restart");
      else if (secondOp.resources.some((item) => item.verified === true)) failures.push("a resource of the failed cleanup is reported verified after the restart");
      if (failures.length) { console.error(failures.join("; ")); process.exit(1); }
      console.log(`sql-reopen ok: ${list.length} resources, statuses equal the end of the run, first cleanup SUCCEEDED and verified for exactly its 2 confirmed resources, second cleanup FAILED and unverified, read back after a server restart on the same PostgreSQL database`);
    })().catch((error) => { console.error(error.message); process.exit(1); });
  ' "${REOPEN_API}" "${WORK}/scenario.json" "${WORK}/outcome.json" | tee -a "${WORK}/checks.txt" || STATUS=1
else
  check "FAIL the server did not restart on the same database; see ${WORK}/orchestrator-reopen.log"; STATUS=1
fi
kill -TERM "${SERVER_PID}" 2>/dev/null || true
for _ in $(seq 1 40); do kill -0 "${SERVER_PID}" 2>/dev/null || break; sleep 0.25; done
wait "${SERVER_PID}" 2>/dev/null || true
SERVER_PID=""

# Captions below the browser area, never over the UI; also after a failure.
CAPTIONED="${WORK}/I04-08-captioned.mp4"
W="${SCREEN%x*}" H="${SCREEN#*x}"
if [[ -s "${RAW}" && -s "${WORK}/captions.srt" ]]; then
  node "${E2E}/render-captions.mjs" "${WORK}/captions.srt" "${WORK}/captions.ass" "${W}" "$((H + STRIP))"
  ffmpeg -hide_banner -loglevel error -n -i "${RAW}" -vf "pad=${W}:$((H + STRIP)):0:0:black,ass=${WORK}/captions.ass" \
    -r 15 -c:v libx264 -preset veryfast -crf 24 -pix_fmt yuv420p -movflags +faststart "${CAPTIONED}" || { echo "caption burn-in failed" >&2; STATUS=1; }
fi

if [[ "${STATUS}" != 0 ]]; then
  echo "scenario I04-08 failed; video, marks, assertions and checks kept in ${WORK}" >&2
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
