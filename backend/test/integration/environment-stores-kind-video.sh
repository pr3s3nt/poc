#!/usr/bin/env bash
# Live, recorded ADR-012 verification on the existing kind-idp-internal cluster
# (real Kubernetes adapters, real Vault KV v2, real Vault Secrets Operator, real
# PostgreSQL 16; no fakes or mocks).
#
# Run-owned infrastructure only:
#   - three Vault dev containers on the `kind` Docker network: two workload
#     Secret Stores (each with its own Kubernetes auth mount trusting this
#     cluster) and one platform credential store for store tokens;
#   - a reviewer ServiceAccount + ClusterRoleBinding labelled with the run id
#     (Vault's TokenReview identity);
#   - the application namespaces created by the product (label-checked removal).
# A Platform Engineer registers both stores through the UI with SCOPED tokens
# (never root); a Developer selects a store and a deployment connection per
# Environment, copies a secret to the second store and rolls it out through VSO,
# sees a stale transition Preview rejected from a second tab, then migrates the
# environment (MIGRATE_POSTGRES) into a second target generation. The known job
# row is served from the destination over the public Ingress, the source is
# retained quiesced, survives a backend restart, and is deleted only by an
# explicit cleanup action.
#
# LIMIT: both logical Connections use the SAME physical kind cluster and
# credential; the generations are isolated by namespace/resource/state identity.
# This proves logical-target isolation, not two physical clusters, DNS cutover or
# AWS/Aurora. Evidence stays outside Git; no token, secret value or dump content
# is written to the evidence directory (checked at the end).
#
# Output: /tmp/poc-environment-stores-review/live/<run id>/
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
REPO="$(cd "${ROOT}/.." && pwd)"
RUN_ID="envstores-$(date -u +%Y%m%d%H%M%S)-${RANDOM}"
CONTEXT="kind-idp-internal"
SCREEN="${ORCH_VIDEO_SCREEN:-1440x900}"
VAULT_IMAGE="${ORCH_VIDEO_VAULT_IMAGE:-hashicorp/vault:1.20}"
VIDEO_NAME="environment-stores-kind-${RUN_ID}.mp4"
OUT_BASE="${ORCH_RESULT_DIR:-/tmp/poc-environment-stores-review/live}"
MODE="${ORCH_RUN_MODE:-video}" # video | probe (REST-only infrastructure check, no browser)
CONTAINERS=()
PIDS=()
BACKEND_PID=""
BROWSER_PGID=""
NODE_PID=""
REVIEWER_NS="orch-${RUN_ID}-reviewer"

WORK="${OUT_BASE}/${RUN_ID}"
[[ ! -e "${WORK}" ]] || { echo "${WORK} already exists" >&2; exit 1; }
mkdir -p "${OUT_BASE}"
mkdir -m 0700 "${WORK}"
PRIVATE="${WORK}/private"
mkdir -m 0700 "${PRIVATE}"
# shellcheck source=video-lib.sh
source "${ROOT}/test/integration/video-lib.sh"

namespace_lookup() { kubectl --context "${CONTEXT}" get namespace "$1" --ignore-not-found -o name; }
note() { echo "$*" | tee -a "${WORK}/checks.txt"; }
synthetic_token() { head -c 32 /dev/urandom | od -An -tx1 | tr -d ' \n'; }

stop_browser_group() {
  [[ -n "${BROWSER_PGID}" ]] || return 0
  kill -TERM -- "-${BROWSER_PGID}" 2>/dev/null || true
  for _ in $(seq 1 20); do kill -0 -- "-${BROWSER_PGID}" 2>/dev/null || break; sleep 0.1; done
  kill -KILL -- "-${BROWSER_PGID}" 2>/dev/null || true
  [[ -z "${NODE_PID}" ]] || { wait "${NODE_PID}" 2>/dev/null || true; }
  NODE_PID=""
  if ps -eo pgid=,stat= | awk -v group="${BROWSER_PGID}" '$1 == group && $2 !~ /^Z/ { alive=1 } END { exit !alive }'; then
    echo "cleanup: recording process group ${BROWSER_PGID} still running" >&2
    return 1
  fi
  echo "cleanup: recording process group ${BROWSER_PGID} stopped"
  BROWSER_PGID=""
}

stop_backend() {
  [[ -n "${BACKEND_PID}" ]] || return 0
  kill "${BACKEND_PID}" 2>/dev/null || true
  for _ in $(seq 1 50); do kill -0 "${BACKEND_PID}" 2>/dev/null || break; sleep 0.1; done
  kill -KILL "${BACKEND_PID}" 2>/dev/null || true
  wait "${BACKEND_PID}" 2>/dev/null || true
  BACKEND_PID=""
}

cleanup() {
  local status=$?
  local pid name ns
  stop_browser_group || status=1
  stop_backend
  for pid in "${PIDS[@]}"; do kill "${pid}" 2>/dev/null || true; done
  for pid in "${PIDS[@]}"; do wait "${pid}" 2>/dev/null || true; done
  # Application namespaces: only those carrying this run's application label.
  local app=""
  [[ -s "${WORK}/run.json" ]] && app="$(jq -r .applicationId "${WORK}/run.json" 2>/dev/null || true)"
  [[ -n "${app}" ]] || app="$(<"${WORK}/application-id" 2>/dev/null || true)"
  if [[ -n "${app}" && "${app}" =~ ^[a-z0-9-]+$ ]]; then
    for ns in $(kubectl --context "${CONTEXT}" get namespace -l "orchestrator.io/application=${app}" -o name 2>/dev/null | sed 's#namespace/##'); do
      kubectl --context "${CONTEXT}" delete namespace "${ns}" --ignore-not-found --wait=true --timeout=300s >/dev/null || status=1
    done
    if [[ -n "$(kubectl --context "${CONTEXT}" get namespace -l "orchestrator.io/application=${app}" -o name 2>/dev/null)" ]]; then
      echo "cleanup: application namespaces still exist" >&2; status=1
    else
      echo "cleanup: no namespace labelled orchestrator.io/application=${app} remains" | tee -a "${WORK}/cleanup.txt"
    fi
  fi
  # Reviewer identity (run-owned).
  kubectl --context "${CONTEXT}" delete clusterrolebinding "orch-${RUN_ID}-reviewer" --ignore-not-found >/dev/null 2>&1 || status=1
  kubectl --context "${CONTEXT}" delete namespace "${REVIEWER_NS}" --ignore-not-found --wait=true --timeout=120s >/dev/null 2>&1 || status=1
  if [[ -z "$(namespace_lookup "${REVIEWER_NS}" 2>/dev/null)" ]] && ! kubectl --context "${CONTEXT}" get clusterrolebinding "orch-${RUN_ID}-reviewer" >/dev/null 2>&1; then
    echo "cleanup: reviewer namespace and ClusterRoleBinding removed" | tee -a "${WORK}/cleanup.txt"
  else
    echo "cleanup: reviewer identity still present" >&2; status=1
  fi
  for name in "${CONTAINERS[@]}"; do docker rm -f "${name}" >/dev/null 2>&1 || true; done
  for name in "${CONTAINERS[@]}"; do
    if docker container inspect "${name}" >/dev/null 2>&1; then echo "WARNING: container ${name} still present" >&2; status=1
    else echo "cleanup: container ${name} removed" | tee -a "${WORK}/cleanup.txt"; fi
  done
  rm -rf "${PRIVATE}"
  rm -f "${WORK}/state.json"
  [[ ! -e "${PRIVATE}" ]] && echo "cleanup: private credential directory removed" >> "${WORK}/cleanup.txt"
  [[ -s "${WORK}/${VIDEO_NAME}" ]] && echo "video=${WORK}/${VIDEO_NAME}"
  echo "run-id=${RUN_ID} evidence=${WORK} status=${status}"
  exit "${status}"
}
trap cleanup EXIT
trap 'exit 129' HUP
trap 'exit 130' INT
trap 'exit 143' TERM

# ---------------------------------------------------------------- preflight
CURRENT_BEFORE="$(kubectl config current-context)"
video_require_tools
command -v docker >/dev/null; command -v setsid >/dev/null; command -v jq >/dev/null; command -v node >/dev/null
if [[ "${MODE}" == "video" ]]; then
  video_require_fresh_dist "${REPO}"
  XDOTOOL="$(video_xdotool)"
fi
kubectl config get-contexts -o name | grep -qx "${CONTEXT}"
kubectl --context "${CONTEXT}" version -o json >/dev/null
docker network inspect kind >/dev/null
kubectl --context "${CONTEXT}" -n vault-secrets-operator-system rollout status deployment/vault-secrets-operator-controller-manager --timeout=60s >/dev/null
kubectl --context "${CONTEXT}" -n traefik rollout status deployment/traefik --timeout=60s >/dev/null
note "preflight: context ${CONTEXT}, kind network, VSO and Traefik ready; AWS/DO contexts untouched (every command passes --context)"
umask 077

UPLOAD="${PRIVATE}/upload.kubeconfig"
kubectl config view --raw --minify --flatten --context "${CONTEXT}" > "${UPLOAD}"
chmod 0600 "${UPLOAD}"
HOSTCFG="${PRIVATE}/backend-host.kubeconfig"
cp "${UPLOAD}" "${HOSTCFG}"
HOSTUSER="$(kubectl --kubeconfig "${HOSTCFG}" config view --minify -o jsonpath='{.users[0].name}')"
kubectl --kubeconfig "${HOSTCFG}" config unset "users.${HOSTUSER}.client-key-data" >/dev/null
kubectl --kubeconfig "${HOSTCFG}" config unset "users.${HOSTUSER}.client-certificate-data" >/dev/null
kubectl --kubeconfig "${HOSTCFG}" config set-credentials "${HOSTUSER}" --token="$(synthetic_token)" >/dev/null
if KUBECONFIG="${HOSTCFG}" kubectl --context "${CONTEXT}" get namespace default -o name >/dev/null 2>&1; then
  echo "backend host credential is unexpectedly valid" >&2; exit 1
fi
note "preflight: backend host-context credential is rejected, so every cluster call must use the uploaded Connection credential"

# ------------------------------------------------- Kubernetes auth identity
kubectl --context "${CONTEXT}" create namespace "${REVIEWER_NS}" >/dev/null
kubectl --context "${CONTEXT}" label namespace "${REVIEWER_NS}" "orchestrator.io/run-id=${RUN_ID}" >/dev/null
kubectl --context "${CONTEXT}" -n "${REVIEWER_NS}" create serviceaccount reviewer >/dev/null
kubectl --context "${CONTEXT}" create clusterrolebinding "orch-${RUN_ID}-reviewer" --clusterrole=system:auth-delegator --serviceaccount="${REVIEWER_NS}:reviewer" >/dev/null
kubectl --context "${CONTEXT}" -n "${REVIEWER_NS}" create token reviewer --duration=4h > "${PRIVATE}/reviewer.jwt"
kubectl config view --raw --minify --flatten --context "${CONTEXT}" -o jsonpath='{.clusters[0].cluster.certificate-authority-data}' | base64 -d > "${PRIVATE}/cluster-ca.pem"
CONTROL_PLANE="$(docker ps --filter 'name=idp-internal-control-plane' --format '{{.Names}}' | head -1)"
[[ -n "${CONTROL_PLANE}" ]]
K8S_HOST="https://${CONTROL_PLANE}:6443"

# ------------------------------------------------------------- Vault dev nodes
# start_vault <label> <auth-mount>: sets <LABEL>_HOST (published) and <LABEL>_CLUSTER
# (reachable from Pods over the kind network), a root header file and enables the
# Kubernetes auth mount when one is requested.
start_vault() {
  local label="$1" auth="$2" name root port ip
  name="orch-${RUN_ID}-${label}"
  root="$(synthetic_token)"
  printf 'X-Vault-Token: %s\n' "${root}" > "${PRIVATE}/${label}-root-header"
  printf 'VAULT_DEV_ROOT_TOKEN_ID=%s\nVAULT_DEV_LISTEN_ADDRESS=0.0.0.0:8200\n' "${root}" > "${PRIVATE}/${label}-vault.env"
  unset root
  docker run -d --rm --name "${name}" --label "orchestrator.run-id=${RUN_ID}" --cap-add IPC_LOCK --network kind \
    --env-file "${PRIVATE}/${label}-vault.env" -p 127.0.0.1::8200 "${VAULT_IMAGE}" server -dev >/dev/null
  CONTAINERS+=("${name}")
  port="$(docker port "${name}" 8200/tcp | head -1 | sed 's/.*://')"
  ip="$(docker inspect -f '{{(index .NetworkSettings.Networks "kind").IPAddress}}' "${name}")"
  eval "${label}_HOST=http://127.0.0.1:${port}"
  eval "${label}_CLUSTER=http://${ip}:8200"
  for _ in $(seq 1 60); do curl -fsS "http://127.0.0.1:${port}/v1/sys/health" >/dev/null 2>&1 && break; sleep 0.5; done
  curl -fsS "http://127.0.0.1:${port}/v1/sys/health" >/dev/null
  if [[ -n "${auth}" ]]; then
    curl -fsS -H @"${PRIVATE}/${label}-root-header" -X POST "http://127.0.0.1:${port}/v1/sys/auth/${auth}" -d '{"type":"kubernetes"}' >/dev/null
    jq -n --arg host "${K8S_HOST}" --rawfile ca "${PRIVATE}/cluster-ca.pem" --rawfile jwt "${PRIVATE}/reviewer.jwt" \
      '{kubernetes_host:$host, kubernetes_ca_cert:$ca, token_reviewer_jwt:$jwt}' \
      | curl -fsS -H @"${PRIVATE}/${label}-root-header" -X POST "http://127.0.0.1:${port}/v1/auth/${auth}/config" --data-binary @- >/dev/null
  fi
}
# scoped_token <label> <policy-name> <policy-hcl-file> <out-file>
scoped_token() {
  local label="$1" policy="$2" file="$3" out="$4" port
  port="$(eval "echo \${${label}_HOST}" | sed 's/.*://')"
  jq -n --rawfile p "${file}" '{policy:$p}' | curl -fsS -H @"${PRIVATE}/${label}-root-header" -X PUT "http://127.0.0.1:${port}/v1/sys/policies/acl/${policy}" --data-binary @- >/dev/null
  curl -fsS -H @"${PRIVATE}/${label}-root-header" -X POST "http://127.0.0.1:${port}/v1/auth/token/create" \
    -d "{\"policies\":[\"${policy}\"],\"no_default_policy\":true,\"ttl\":\"4h\"}" \
    | node -e 'let s="";process.stdin.on("data",d=>s+=d).on("end",()=>{const t=JSON.parse(s).auth?.client_token;if(!t)process.exit(1);process.stdout.write(t)})' > "${out}"
  [[ -s "${out}" ]]
}
store_policy() { # <auth-mount> <file>
  cat > "$2" <<EOF
path "auth/token/lookup-self" { capabilities = ["read"] }
path "sys/capabilities-self" { capabilities = ["update"] }
path "sys/internal/ui/mounts/*" { capabilities = ["read"] }
path "secret/data/orchestrator/apps/*" { capabilities = ["create", "read", "update"] }
path "secret/metadata/orchestrator/apps/*" { capabilities = ["read", "list", "delete"] }
path "sys/policies/acl/orch-*" { capabilities = ["create", "update", "read"] }
path "auth/$1/role/orch-*" { capabilities = ["create", "update", "read"] }
path "auth/$1/config" { capabilities = ["read"] }
EOF
}

start_vault STORE_A kubernetes
start_vault STORE_B k8s-b
start_vault CREDS ""
store_policy kubernetes "${PRIVATE}/policy-a.hcl"
store_policy k8s-b "${PRIVATE}/policy-b.hcl"
scoped_token STORE_A orch-workload-store "${PRIVATE}/policy-a.hcl" "${PRIVATE}/store-a-token"
scoped_token STORE_B orch-workload-store "${PRIVATE}/policy-b.hcl" "${PRIVATE}/store-b-token"
cat > "${PRIVATE}/policy-creds.hcl" <<'EOF'
path "secret/data/orchestrator/connections/*" { capabilities = ["create", "read"] }
path "secret/metadata/orchestrator/connections/*" { capabilities = ["read", "list", "delete"] }
EOF
scoped_token CREDS orch-connection-credentials "${PRIVATE}/policy-creds.hcl" "${PRIVATE}/creds-token"
note "vault: store A ${STORE_A_CLUSTER} (auth mount kubernetes), store B ${STORE_B_CLUSTER} (auth mount k8s-b), credential store ${CREDS_HOST}; backend tokens are scoped, never root"

# ------------------------------------------------------------- build and start
cd "${ROOT}"
"${ROOT}/test/integration/build-images.sh" "${RUN_ID}" "${WORK}/bin"
for workload in frontend backend; do kind load docker-image "acceptance-${workload}:${RUN_ID}" --name idp-internal; done
go build -o "${WORK}/orchestrator" ./cmd/orchestrator

API_PORT="$(node -e 'const s=require("net").createServer();s.listen(0,"127.0.0.1",()=>{console.log(s.address().port);s.close()})')"
start_backend() {
  local label="$1"
  rm -f "${WORK}/api-addr"
  env -u ORCHESTRATOR_DATABASE_URL_FILE -u ORCHESTRATOR_VAULT_ADDR -u ORCHESTRATOR_VAULT_TOKEN_FILE \
    -u ORCHESTRATOR_VAULT_AGENT_ADDR -u ORCHESTRATOR_CONNECTION_CREDENTIAL_STORE -u ORCHESTRATOR_CONNECTION_VAULT_ADDR \
    -u ORCHESTRATOR_CONNECTION_VAULT_TOKEN_FILE -u TF_PLUGIN_CACHE_DIR -u AWS_PROFILE -u AWS_REGION -u AWS_DEFAULT_REGION \
    -u AWS_ACCESS_KEY_ID -u AWS_SECRET_ACCESS_KEY -u AWS_SESSION_TOKEN \
    KUBECONFIG="${HOSTCFG}" \
    "${WORK}/orchestrator" -addr "127.0.0.1:${API_PORT}" -addr-file "${WORK}/api-addr" -state "${WORK}/state.json" -database-url-file "" \
    -adapters kubernetes -kube-context "${CONTEXT}" -cluster idp-internal -run-id "${RUN_ID}" -vault-delivery vso \
    -connection-credential-store vault -connection-vault-address "${CREDS_HOST}" \
    -connection-vault-token-file "${PRIVATE}/creds-token" -connection-vault-mount secret \
    -ui-dir "${REPO}/frontend/dist" >> "${WORK}/orchestrator-${label}.log" 2>&1 &
  BACKEND_PID=$!
  for _ in $(seq 1 80); do [[ -s "${WORK}/api-addr" ]] && break; sleep 0.5; done
  [[ -s "${WORK}/api-addr" ]] || { echo "backend did not start; see ${WORK}/orchestrator-${label}.log" >&2; return 1; }
  for _ in $(seq 1 40); do curl -fsS "http://127.0.0.1:${API_PORT}/api/v1/healthz" >/dev/null 2>&1 && return 0; sleep 0.5; done
  echo "backend is not healthy" >&2
  return 1
}
start_backend first
API="http://127.0.0.1:${API_PORT}"

# ---------------------------- setup through REST (not part of the shown flow)
SOURCE_NAME="Source cluster"; DEST_NAME="Destination cluster"
SOURCE_KEY="source-cluster"; DEST_KEY="destination-cluster"
signin() { curl -fsS -c "${PRIVATE}/jar-$1" -H 'Content-Type: application/json' -d "{\"username\":\"$1\",\"password\":\"test-password\"}" "${API}/api/v1/auth/sign-in" >/dev/null; }
signin platform-engineer
for pair in "${SOURCE_NAME}:${SOURCE_KEY}" "${DEST_NAME}:${DEST_KEY}"; do
  name="${pair%%:*}"; key="${pair##*:}"
  jq -n --arg name "${name}" --rawfile cfg "${UPLOAD}" --arg ctx "${CONTEXT}" '{name:$name,kubeconfig:$cfg,context:$ctx}' \
    | curl -fsS -b "${PRIVATE}/jar-platform-engineer" -H 'Content-Type: application/json' --data-binary @- "${API}/api/v1/connections/kubernetes" > "${WORK}/connection-${key}.json"
  [[ "$(jq -r .key "${WORK}/connection-${key}.json")" == "${key}" && "$(jq -r .status "${WORK}/connection-${key}.json")" == "READY" ]]
  jq -n --arg key "cluster-${key}" --arg conn "${key}" \
    '{key:$key,resourceType:"k8s-cluster",driverType:"existing-cluster",connectionKey:$conn,
      driverInputs:{values:{variables:{name:"${context.connection.cluster}",kubeContext:"${context.connection.context}"}}},
      criteria:[{res_id:("connections."+$conn),class:"internal"}]}' \
    | curl -fsS -b "${PRIVATE}/jar-platform-engineer" -H 'Content-Type: application/json' --data-binary @- "${API}/api/v1/resource-definitions" >/dev/null
done
note "setup: two READY credential-backed Connections (${SOURCE_KEY}, ${DEST_KEY}) to the same physical cluster, each with its matching cluster Definition"

kubectl --context "${CONTEXT}" -n traefik port-forward svc/traefik :80 > "${WORK}/traefik-port-forward.log" 2>&1 &
PIDS+=($!)
TRAEFIK_PORT=""
for _ in $(seq 1 40); do
  TRAEFIK_PORT="$(sed -n 's/^Forwarding from 127\.0\.0\.1:\([0-9][0-9]*\) -> 8000$/\1/p;s/^Forwarding from 127\.0\.0\.1:\([0-9][0-9]*\) -> 80$/\1/p' "${WORK}/traefik-port-forward.log" | head -n 1)"
  [[ -n "${TRAEFIK_PORT}" ]] && break; sleep 0.5
done
[[ -n "${TRAEFIK_PORT}" ]]

if [[ "${MODE}" == "probe" ]]; then
  # REST-only check of the infrastructure: register a store with the scoped token,
  # copy nothing, and prove the Vault side is usable. No browser, no application.
  jq -n --arg b "${STORE_A_HOST}" --arg w "${STORE_A_CLUSTER}" --rawfile t "${PRIVATE}/store-a-token" \
    '{name:"Probe A",backendAddress:$b,workloadAddress:$w,mount:"secret",authMount:"kubernetes",token:($t|rtrimstr("\n"))}' \
    | curl -sS -b "${PRIVATE}/jar-platform-engineer" -H 'Content-Type: application/json' --data-binary @- "${API}/api/v1/secret-stores" | tee "${WORK}/probe-store-a.json" | jq -c 'del(.verification)'
  [[ "$(jq -r .status "${WORK}/probe-store-a.json")" == "READY" ]]
  note "probe: store A verified READY with the scoped token (kubernetesAuth=$(jq -r .verification.kubernetesAuth "${WORK}/probe-store-a.json"))"
  exit 0
fi

# --------------------------------------------------------------------- video
video_start_xvfb "${SCREEN}"
setsid env DISPLAY="${VIDEO_DISPLAY}" ORCH_E2E_SCREEN="${SCREEN}" ORCH_E2E_XDOTOOL="${XDOTOOL}" \
  ORCH_E2E_URL="${API}" ORCH_E2E_RUN_ID="${RUN_ID}" ORCH_E2E_KUBE_CONTEXT="${CONTEXT}" ORCH_E2E_EVIDENCE_DIR="${WORK}" \
  ORCH_E2E_VIDEO_NAME="${VIDEO_NAME}" ORCH_E2E_TRAEFIK_PORT="${TRAEFIK_PORT}" \
  ORCH_E2E_BROWSER_ARGS="--host-resolver-rules=MAP staging.es$(echo "${RUN_ID}" | tr -cd '0-9' | tail -c 6).example.com 127.0.0.1" \
  ORCH_E2E_STORE_A_TOKEN_FILE="${PRIVATE}/store-a-token" ORCH_E2E_STORE_B_TOKEN_FILE="${PRIVATE}/store-b-token" \
  ORCH_E2E_STORE_A_BACKEND="${STORE_A_HOST}" ORCH_E2E_STORE_B_BACKEND="${STORE_B_HOST}" \
  ORCH_E2E_STORE_A_WORKLOAD="${STORE_A_CLUSTER}" ORCH_E2E_STORE_B_WORKLOAD="${STORE_B_CLUSTER}" \
  ORCH_E2E_SOURCE_KEY="${SOURCE_KEY}" ORCH_E2E_DEST_KEY="${DEST_KEY}" ORCH_E2E_SOURCE_NAME="${SOURCE_NAME}" ORCH_E2E_DEST_NAME="${DEST_NAME}" \
  node "${REPO}/frontend/test/e2e/environment-stores-kind-human.mjs" > "${WORK}/playwright.log" 2>&1 &
NODE_PID=$!
BROWSER_PGID=${NODE_PID}
BROWSER_DEADLINE=$((SECONDS + 3000))
while kill -0 "${NODE_PID}" 2>/dev/null; do
  (( SECONDS < BROWSER_DEADLINE )) || { echo "browser flow exceeded 3000 seconds" >&2; exit 1; }
  if [[ -e "${WORK}/restart-request" ]]; then
    rm -f "${WORK}/restart-request"
    stop_backend
    start_backend restarted
    note "restart: API restarted on the same state file and port"
    : > "${WORK}/restart-done"
  fi
  sleep 0.5
done
browser_status=0
wait "${NODE_PID}" || browser_status=$?
NODE_PID=""
cat "${WORK}/playwright.log"
(( browser_status == 0 )) || exit "${browser_status}"
stop_browser_group
sleep 1

# ------------------------------------------------------- observer assertions
APP="$(jq -r .applicationId "${WORK}/run.json")"
echo "${APP}" > "${WORK}/application-id"
SRC_NS="$(jq -r .sourceNamespace "${WORK}/run.json")"
DST_NS="$(jq -r .destinationNamespace "${WORK}/run.json")"
[[ -z "$(namespace_lookup "${SRC_NS}")" ]]
note "k8s: source namespace ${SRC_NS} is gone after the explicit cleanup"
kubectl --context "${CONTEXT}" -n "${DST_NS}" get deployment,statefulset,service,ingress,vaultconnection,vaultauth,vaultstaticsecret -o wide > "${WORK}/k8s-destination.txt"
for deployment in backend frontend; do
  kubectl --context "${CONTEXT}" -n "${DST_NS}" rollout status "deployment/${deployment}" --timeout=60s >/dev/null
  note "k8s: deployment/${deployment} available in ${DST_NS}"
done
[[ -z "$(kubectl --context "${CONTEXT}" -n "${DST_NS}" get pod --no-headers | grep -v -E ' (Running|Completed) ' || true)" ]]
HOST="$(jq -r .stagingHost "${WORK}/run.json")"
curl -fsS -H "Host: ${HOST}" "http://127.0.0.1:${TRAEFIK_PORT}/api/checks" > "${WORK}/http-checks.json"
[[ "$(jq -c .checks "${WORK}/http-checks.json")" == '{"environment":true,"secret":true,"database":true}' ]]
note "http: destination served /api/checks through Traefik with Host ${HOST}"
signin developer
curl -fsS -b "${PRIVATE}/jar-developer" "${API}/api/v1/applications/${APP}/environments/staging/connection-transitions" > "${WORK}/transitions.json"
jq -e '.transitions | length == 1 and .[0].status == "SUCCEEDED" and .[0].authority == "DESTINATION" and .[0].sourceState == "CLEANED" and .[0].mode == "MIGRATE_POSTGRES" and .[0].destination.generation == 1' "${WORK}/transitions.json" >/dev/null
curl -fsS -b "${PRIVATE}/jar-developer" "${API}/api/v1/applications/${APP}/environments/staging/operations" > "${WORK}/operations.json"
jq -e '[.operations[] | select(.status == "ACTIVE" or .status == "INTERRUPTED" or .status == "RECOVERING")] | length == 0' "${WORK}/operations.json" >/dev/null
note "api: one SUCCEEDED MIGRATE_POSTGRES transition (generation 1, destination authoritative, source CLEANED); no operation is holding the environment"
node -e '
  const state = JSON.parse(require("fs").readFileSync(process.argv[1], "utf8"));
  const app = process.argv[2];
  const env = state.environments?.[`${app}/staging`];
  const fail = (m) => { console.error(m); process.exit(1); };
  if (!env || env.targetGeneration !== 1 || env.connectionKey !== process.argv[3] || env.activeOperationId) fail("persisted staging binding is not the committed generation 1");
  const stores = Object.values(state.secretStores ?? {}).map((s) => s.name);
  if (!stores.includes("Alpha vault") || !stores.includes("Beta vault")) fail("both stores must be persisted");
  if (!String(env.secretStoreKey).includes("beta")) fail("staging must select the second store");
  const instances = Object.values(state.workloadInstances ?? {}).filter((w) => w.environmentKey === `${app}/staging`);
  if (!instances.some((w) => w.generation === 1) ) fail("no generation 1 workload instance");
  if (instances.some((w) => w.generation === 1 && w.targetRef?.connection !== process.argv[3])) fail("generation 1 instances must reference the destination Connection");
  console.log(JSON.stringify({ generation: env.targetGeneration, connection: env.connectionKey, store: env.secretStoreKey, instances: instances.map((w) => ({ generation: w.generation ?? 0, workload: w.workloadId, status: w.status })) }));
' "${WORK}/state.json" "${APP}" "${DEST_KEY}" | tee "${WORK}/persisted.json"
# Nothing sensitive in the evidence: scoped tokens, the reviewer JWT and the uploaded credential.
node -e '
  const fs = require("fs");
  const [priv, ...files] = process.argv.slice(1);
  const values = [];
  for (const name of ["store-a-token", "store-b-token", "creds-token", "reviewer.jwt"]) { const t = fs.readFileSync(`${priv}/${name}`, "utf8").trim(); if (t.length >= 12) values.push(t); }
  values.push(...[...fs.readFileSync(`${priv}/upload.kubeconfig`, "utf8").matchAll(/^\s*(?:token|client-key-data|client-certificate-data):\s*(\S+)\s*$/gm)].map((m) => m[1]).filter((v) => v.length >= 12));
  for (const file of files) {
    if (!fs.existsSync(file)) continue;
    const text = fs.readFileSync(file, "utf8");
    if (values.some((v) => text.includes(v))) { console.error(`secret material found in ${file}`); process.exit(1); }
  }
  console.log(`no-secret-in-artifacts files=${files.length} values=${values.length}`);
' "${PRIVATE}" "${WORK}/orchestrator-first.log" "${WORK}/orchestrator-restarted.log" "${WORK}/playwright.log" "${WORK}/run.json" "${WORK}/marks.json" "${WORK}/state.json" "${WORK}/k8s-destination.txt" "${WORK}/checks.txt" "${WORK}/transitions.json" "${WORK}/operations.json" "${WORK}/persisted.json" "${WORK}/http-checks.json" | tee -a "${WORK}/checks.txt"
[[ "$(kubectl config current-context)" == "${CURRENT_BEFORE}" ]]
note "current kubectl context unchanged: ${CURRENT_BEFORE}"
jq -n --arg run "${RUN_ID}" --arg app "${APP}" --arg src "${SRC_NS}" --arg dst "${DST_NS}" \
  '{runId:$run, applicationId:$app, sourceNamespace:$src, destinationNamespace:$dst, limit:"both logical Connections point at the same physical kind cluster"}' > "${WORK}/summary.json"
stop_backend
video_validate "${WORK}/${VIDEO_NAME}" "${SCREEN}" "${ORCH_VIDEO_MIN_SECONDS:-420}" 30 cleanup-done
