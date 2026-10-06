#!/usr/bin/env bash
# Human-paced UC-04 kubeconfig upload video: isolated local backend, private
# temporary directory, headed Chromium on a private Xvfb display recorded by
# ffmpeg. Every shown step is a UI action; no route mocks, no API-created
# Connection. See orchestrator_docs/operations/local.md (UC-04 kubeconfig
# review recording).
#
# Usage: uc04-kubeconfig-video-local.sh [--kind]
#   default  SIMULATED evidence: a synthetic kubeconfig (unreachable
#            endpoint, random token) and the explicit non-durable memory
#            credential store. No cluster or Vault is contacted; Check and
#            save is rejected by the real read-only verifier.
#   --kind   Live read-only verification of the existing kind-idp-internal
#            context only. Its flattened selected config is written straight
#            to a private 0600 file (never printed) and uploaded with one
#            synthetic extra context. The credential goes to a run-owned
#            Vault dev container (test-only root token, a scoped token for
#            orchestrator/connections only, consumed by file path). Executor
#            adapters stay fake: no Kubernetes object is created, and the
#            platform Vault, its policies and workloads are not touched.
#
# Requires Xvfb, ffmpeg, ffprobe, kubectl, node and (for --kind) docker; the
# Vault image (ORCH_VIDEO_VAULT_IMAGE, default hashicorp/vault:1.20) may be
# pulled once.
#
# Output: ${ORCH_VIDEO_DIR:-/tmp/<run id>.XXXXXX}/uc04-kubeconfig[-kind]-review.mp4
# plus run.json, marks.json, ffprobe.json, frames/*.png and checks.txt. The
# private credential directory, temporary state and the run's Vault
# container are removed on exit, also on failure; nothing is committed.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
REPO="$(cd "${ROOT}/.." && pwd)"
KIND_CONTEXT=""
case "${1:-}" in
  "") ;;
  --kind) KIND_CONTEXT="kind-idp-internal" ;;
  *) echo "usage: $0 [--kind]" >&2; exit 2 ;;
esac
RUN_ID="uc04-kubeconfig-${KIND_CONTEXT:+kind-}video-$(date -u +%Y%m%d%H%M%S)-${RANDOM}"
SCREEN="${ORCH_VIDEO_SCREEN:-1440x900}"
MIN_SECONDS="${ORCH_VIDEO_MIN_SECONDS:-60}"
VIDEO_NAME="uc04-kubeconfig${KIND_CONTEXT:+-kind}-review.mp4"
VAULT_IMAGE="${ORCH_VIDEO_VAULT_IMAGE:-hashicorp/vault:1.20}"
VAULT_CONTAINER=""
MIN_MARKS=8
[[ -z "${KIND_CONTEXT}" ]] || MIN_MARKS=10
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
chmod 0700 "${WORK}"
PRIVATE="${WORK}/private"
mkdir -m 0700 "${PRIVATE}"
# shellcheck source=video-lib.sh
source "${ROOT}/test/integration/video-lib.sh"

cleanup() {
  local status=$?
  local pid
  for pid in "${PIDS[@]}"; do kill "${pid}" 2>/dev/null || true; done
  for pid in "${PIDS[@]}"; do wait "${pid}" 2>/dev/null || true; done
  # Only this run's container, found by its unique name.
  if [[ -n "${VAULT_CONTAINER}" ]]; then docker rm -f "${VAULT_CONTAINER}" >/dev/null 2>&1 || true; fi
  rm -rf "${PRIVATE}"
  rm -f "${WORK}/state.json"
  if [[ -n "${VAULT_CONTAINER}" ]] && docker container inspect "${VAULT_CONTAINER}" >/dev/null 2>&1; then
    echo "WARNING: Vault container ${VAULT_CONTAINER} is still present" >&2
    status=1
  fi
  [[ ! -e "${PRIVATE}" ]] || { echo "WARNING: private credential directory remains: ${PRIVATE}" >&2; status=1; }
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
command -v kubectl >/dev/null || { echo "kubectl is required" >&2; exit 1; }
XDOTOOL="$(video_xdotool)"
umask 077

# Credential values are generated or copied only into PRIVATE and are never
# echoed. kubectl config writes below edit only these private files.
UNSUPPORTED="${PRIVATE}/unsupported-exec.kubeconfig"
cat > "${UNSUPPORTED}" <<'EOF'
apiVersion: v1
kind: Config
clusters:
- name: eks-demo
  cluster:
    server: https://eks-demo.invalid
users:
- name: eks-demo-user
  user:
    exec:
      apiVersion: client.authentication.k8s.io/v1beta1
      command: aws
      args: ["eks", "get-token", "--cluster-name", "demo"]
contexts:
- name: eks-demo
  context:
    cluster: eks-demo
    user: eks-demo-user
current-context: eks-demo
EOF
synthetic_token() { head -c 32 /dev/urandom | od -An -tx1 | tr -d ' \n'; }

UPLOAD="${PRIVATE}/upload.kubeconfig"
if [[ -n "${KIND_CONTEXT}" ]]; then
  command -v docker >/dev/null || { echo "docker is required for the run-owned Vault" >&2; exit 1; }
  # Read-only preflight; the current context is neither read nor changed.
  kubectl config get-contexts -o name | grep -qx "${KIND_CONTEXT}" || { echo "context ${KIND_CONTEXT} is not configured" >&2; exit 1; }
  kubectl --context "${KIND_CONTEXT}" version -o json >/dev/null
  # Selected context only, flattened (embedded CA/client certificate), written
  # straight to the private file.
  kubectl config view --raw --minify --flatten --context "${KIND_CONTEXT}" > "${UPLOAD}"
  grep -q 'client-key-data:\|token:' "${UPLOAD}" || { echo "${KIND_CONTEXT} does not use embedded credentials" >&2; exit 1; }
  SELECT_CONTEXT="${KIND_CONTEXT}"
else
  : > "${UPLOAD}"
  SELECT_CONTEXT="lab-primary"
  kubectl config --kubeconfig "${UPLOAD}" set-cluster lab-primary --server=https://127.0.0.1:9 >/dev/null
  kubectl config --kubeconfig "${UPLOAD}" set-credentials lab-primary-user --token="$(synthetic_token)" >/dev/null
  kubectl config --kubeconfig "${UPLOAD}" set-context lab-primary --cluster=lab-primary --user=lab-primary-user >/dev/null
fi
# The one-context file is uploaded first to show automatic context selection.
SINGLE="${PRIVATE}/single-context.kubeconfig"
cp "${UPLOAD}" "${SINGLE}"
# A second, synthetic context makes the multiple-context choice explicit.
kubectl config --kubeconfig "${UPLOAD}" set-cluster demo-unreachable --server=https://127.0.0.1:9 >/dev/null
kubectl config --kubeconfig "${UPLOAD}" set-credentials demo-unreachable-user --token="$(synthetic_token)" >/dev/null
kubectl config --kubeconfig "${UPLOAD}" set-context demo-unreachable --cluster=demo-unreachable --user=demo-unreachable-user >/dev/null
chmod 0600 "${UPLOAD}" "${SINGLE}" "${UNSUPPORTED}"

STORE_FLAGS=(-connection-credential-store memory)
STATE_FLAGS=()
if [[ -n "${KIND_CONTEXT}" ]]; then
  VAULT_CONTAINER="orch-${RUN_ID}"
  ROOT_TOKEN="$(synthetic_token)"
  printf 'X-Vault-Token: %s\n' "${ROOT_TOKEN}" > "${PRIVATE}/root-header"
  printf 'VAULT_DEV_ROOT_TOKEN_ID=%s\nVAULT_DEV_LISTEN_ADDRESS=0.0.0.0:8200\n' "${ROOT_TOKEN}" > "${PRIVATE}/vault.env"
  unset ROOT_TOKEN
  docker run -d --rm --name "${VAULT_CONTAINER}" --label "orchestrator.run-id=${RUN_ID}" --cap-add IPC_LOCK \
    --env-file "${PRIVATE}/vault.env" -p 127.0.0.1::8200 "${VAULT_IMAGE}" server -dev >/dev/null
  VAULT_PORT="$(docker port "${VAULT_CONTAINER}" 8200/tcp | head -1 | sed 's/.*://')"
  VAULT_ADDR_LOCAL="http://127.0.0.1:${VAULT_PORT}"
  for _ in $(seq 1 60); do curl -fsS "${VAULT_ADDR_LOCAL}/v1/sys/health" >/dev/null 2>&1 && break; sleep 0.5; done
  curl -fsS "${VAULT_ADDR_LOCAL}/v1/sys/health" >/dev/null
  # Scoped policy on the dev server's KV v2 mount "secret": Connection
  # credential objects only.
  curl -fsS -H @"${PRIVATE}/root-header" -X PUT "${VAULT_ADDR_LOCAL}/v1/sys/policies/acl/orch-connection-credentials" \
    -d '{"policy":"path \"secret/data/orchestrator/connections/*\" { capabilities = [\"create\", \"read\"] }\npath \"secret/metadata/orchestrator/connections/*\" { capabilities = [\"read\", \"list\", \"delete\"] }\n"}' >/dev/null
  curl -fsS -H @"${PRIVATE}/root-header" -X POST "${VAULT_ADDR_LOCAL}/v1/auth/token/create" \
    -d '{"policies":["orch-connection-credentials"],"no_default_policy":true,"ttl":"2h"}' \
    | node -e 'let s="";process.stdin.on("data",d=>s+=d).on("end",()=>{const t=JSON.parse(s).auth?.client_token;if(!t)process.exit(1);process.stdout.write(t)})' \
    > "${PRIVATE}/connection-vault-token"
  [[ -s "${PRIVATE}/connection-vault-token" ]] || { echo "scoped Vault token was not created" >&2; exit 1; }
  STORE_FLAGS=(-connection-credential-store vault -connection-vault-address "${VAULT_ADDR_LOCAL}"
    -connection-vault-token-file "${PRIVATE}/connection-vault-token" -connection-vault-mount secret)
  STATE_FLAGS=(-state "${WORK}/state.json")
fi

(cd "${ROOT}" && go build -o "${WORK}/orchestrator" ./cmd/orchestrator)
# Isolated backend: inherited database/Vault/Terraform/AWS/kube defaults are
# dropped and the corresponding flags are explicitly blank. KUBECONFIG is
# unset so only the uploaded credential can reach a cluster during upload
# verification.
env -u ORCHESTRATOR_DATABASE_URL_FILE -u ORCHESTRATOR_VAULT_ADDR -u ORCHESTRATOR_VAULT_TOKEN_FILE \
  -u ORCHESTRATOR_VAULT_AGENT_ADDR -u ORCHESTRATOR_CONNECTION_CREDENTIAL_STORE -u ORCHESTRATOR_CONNECTION_VAULT_ADDR \
  -u ORCHESTRATOR_CONNECTION_VAULT_TOKEN_FILE -u TF_PLUGIN_CACHE_DIR -u AWS_PROFILE -u AWS_REGION -u AWS_DEFAULT_REGION \
  -u AWS_ACCESS_KEY_ID -u AWS_SECRET_ACCESS_KEY -u AWS_SESSION_TOKEN -u KUBECONFIG \
  "${WORK}/orchestrator" -addr 127.0.0.1:0 -addr-file "${WORK}/api-addr" "${STATE_FLAGS[@]}" \
  -database-url-file "" -vault-address "" -vault-token-file "" -vault-agent-address "" "${STORE_FLAGS[@]}" \
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
  ORCH_E2E_KIND_CONTEXT="${KIND_CONTEXT}" ORCH_E2E_KUBECONFIG_FILE="${UPLOAD}" ORCH_E2E_SINGLE_CONTEXT_FILE="${SINGLE}" \
  ORCH_E2E_UNSUPPORTED_FILE="${UNSUPPORTED}" ORCH_E2E_SELECT_CONTEXT="${SELECT_CONTEXT}" \
  node "${REPO}/frontend/test/e2e/uc04-kubeconfig-video-local.mjs" 2>&1 | tee "${WORK}/playwright.log"

# Post-run checks. Credential values are compared inside node only; output
# lists check names and counts, never values.
node -e '
  const fs = require("fs");
  const [upload, ...files] = process.argv.slice(1);
  const values = [...fs.readFileSync(upload, "utf8").matchAll(/^\s*(?:token|client-key-data|client-certificate-data):\s*(\S+)\s*$/gm)].map((m) => m[1]).filter((v) => v.length >= 12);
  for (const file of files) {
    if (!fs.existsSync(file)) continue;
    const text = fs.readFileSync(file, "utf8");
    if (values.some((v) => text.includes(v))) { console.error(`credential material found in ${file}`); process.exit(1); }
  }
  console.log(`no-credential-in-artifacts files=${files.length} values=${values.length}`);
' "${UPLOAD}" "${WORK}/orchestrator.log" "${WORK}/playwright.log" "${WORK}/run.json" "${WORK}/marks.json" "${WORK}/state.json" | tee "${WORK}/checks.txt"
if [[ -n "${KIND_CONTEXT}" ]]; then
  KEY="$(node -e 'console.log(require(process.argv[1]).connection.key)' "${WORK}/run.json")"
  [[ "${KEY}" =~ ^[a-z0-9-]+$ ]] || { echo "invalid connection key in run.json" >&2; exit 1; }
  grep -q '"authenticationType": "KUBECONFIG"' "${WORK}/state.json" || { echo "state has no KUBECONFIG connection" >&2; exit 1; }
  # Exactly one immutable credential object exists for the new Connection.
  printf 'X-Vault-Token: %s\n' "$(<"${PRIVATE}/connection-vault-token")" > "${PRIVATE}/scoped-header"
  COUNT="$(curl -fsS -H @"${PRIVATE}/scoped-header" -X LIST \
    "${VAULT_ADDR_LOCAL}/v1/secret/metadata/orchestrator/connections/acme/${KEY}/credentials" \
    | node -e 'let s="";process.stdin.on("data",d=>s+=d).on("end",()=>console.log(JSON.parse(s).data.keys.length))')"
  [[ "${COUNT}" == "1" ]] || { echo "expected one credential object, found ${COUNT}" >&2; exit 1; }
  echo "vault-credential-objects=${COUNT} connection=${KEY}" | tee -a "${WORK}/checks.txt"
  OBJECT_PATH="orchestrator/connections/acme/${KEY}/credentials/$(curl -fsS -H @"${PRIVATE}/scoped-header" -X LIST \
    "${VAULT_ADDR_LOCAL}/v1/secret/metadata/orchestrator/connections/acme/${KEY}/credentials" \
    | node -e 'let s="";process.stdin.on("data",d=>s+=d).on("end",()=>process.stdout.write(JSON.parse(s).data.keys[0]))')"
  [[ "${OBJECT_PATH}" =~ ^orchestrator/connections/acme/[a-z0-9-]+/credentials/[A-Za-z0-9_-]+$ ]] || { echo "unexpected credential object path" >&2; exit 1; }
  # One version only, and the stored object is the normalized selected
  # context: the kind credential is present, the synthetic extra context is
  # not. Compared inside node; only booleans are printed.
  VERSIONS="$(curl -fsS -H @"${PRIVATE}/scoped-header" "${VAULT_ADDR_LOCAL}/v1/secret/metadata/${OBJECT_PATH}" \
    | node -e 'let s="";process.stdin.on("data",d=>s+=d).on("end",()=>{const m=JSON.parse(s).data;console.log(`${m.current_version} ${Object.keys(m.versions).length}`)})')"
  [[ "${VERSIONS}" == "1 1" ]] || { echo "credential object has unexpected versions: ${VERSIONS}" >&2; exit 1; }
  curl -fsS -H @"${PRIVATE}/scoped-header" "${VAULT_ADDR_LOCAL}/v1/secret/data/${OBJECT_PATH}" | node -e '
    const fs = require("fs");
    let s = ""; process.stdin.on("data", (d) => s += d).on("end", () => {
      const data = JSON.parse(s).data.data;
      const stored = Buffer.from(data.value, "base64").toString("utf8");
      const single = fs.readFileSync(process.argv[1], "utf8");
      const values = [...single.matchAll(/^\s*(?:token|client-key-data|client-certificate-data):\s*(\S+)\s*$/gm)].map((m) => m[1]);
      const checks = {
        encoding: data.encoding === "base64",
        selectedContext: stored.includes(process.argv[2]),
        syntheticContextAbsent: !stored.includes("demo-unreachable"),
        credentialPresent: values.length > 0 && values.every((v) => stored.includes(v)),
      };
      const failed = Object.entries(checks).filter(([, ok]) => !ok).map(([name]) => name);
      if (failed.length) { console.error(`stored credential check failed: ${failed.join(",")}`); process.exit(1); }
      console.log(`vault-object-normalized ${Object.keys(checks).join(",")}=true versions=1`);
    });
  ' "${SINGLE}" "${KIND_CONTEXT}" | tee -a "${WORK}/checks.txt"
  # Immutable: the scoped token cannot overwrite the existing object.
  OVERWRITE="$(curl -sS -o /dev/null -w '%{http_code}' -H @"${PRIVATE}/scoped-header" -X POST \
    "${VAULT_ADDR_LOCAL}/v1/secret/data/${OBJECT_PATH}" -d '{"data":{"encoding":"base64","value":"eA=="}}')"
  [[ "${OVERWRITE}" == "403" ]] || { echo "scoped overwrite returned ${OVERWRITE}, expected 403" >&2; exit 1; }
  echo "vault-object-overwrite-denied status=${OVERWRITE}" | tee -a "${WORK}/checks.txt"
fi

video_validate "${WORK}/${VIDEO_NAME}" "${SCREEN}" "${MIN_SECONDS}" "${MIN_MARKS}" signed-out
