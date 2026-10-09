#!/usr/bin/env bash
# Live, recorded UC-01 per-Environment Connection selection (ADR-011) on the
# existing kind-idp-internal cluster (real Kubernetes adapters, no fakes or
# mocks).
#
# A Platform Engineer uploads the same kubeconfig twice (two READY nondefault
# logical Connections; no cluster Definition is registered, ADR-013); a Developer creates an UNCONFIGURED Application, sees Preview
# refuse it, sets staging and production to different Connections in Environment
# Settings (editable: kept after a refresh and a backend restart, an empty
# Environment may be rebound, and once staging has runtime resources a different
# destination requires a transition), deploys the diagnostic acceptance workloads to staging with
# PostgreSQL and UC-12 references, and the browser checks the deployed app.
# Every shown mutation is a UI action.
#
# LIMIT: both logical Connections point at the same physical kind cluster and
# credential, so this proves independent logical bindings and execution on the
# selected one, not two physical clusters. Production is bound but not
# deployed, so no production namespace or workload exists.
#
# Identity: the uploaded Connections and the seeded default point at the same
# physical cluster and credential. To prove the uploaded credential drives
# execution, the backend process runs with a private KUBECONFIG whose host
# context carries an invalid token: any fallback to the host context fails.
#
# Safety: existing kind-idp-internal only; the current context is never
# changed; every command passes --context. Only this run's namespace (label
# checked) is deleted. The Connection credentials go to a run-owned Vault dev
# container. The workload Secret Store is a second run-owned Vault dev container
# on the kind network with its own Kubernetes auth mount (trusting this cluster
# through a run-owned reviewer ServiceAccount) and a SCOPED token, registered as
# an ordinary store; the persistent vault-uc12 and its token, policy and roles
# are never used or changed. Credential files live in a private directory that is
# removed on exit. Evidence stays outside Git.
#
# Optional ORCH_E2E_STAGING_CONNECTION_NAME names the uploaded staging
# Connection exactly (for example k8s-4f; its key is the slug of the name). The
# default is the dynamic "Staging kind <suffix>" name. The production Connection
# name always stays dynamic.
#
# Output: /tmp/poc-environment-connection-review/<run id>/
# (environment-connection-kind-<run id>.mp4, marks.json, ffprobe.json,
# frames/, checks.txt, run.json, persisted-binding.json, logs).
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
REPO="$(cd "${ROOT}/.." && pwd)"
RUN_ID="envconn-kind-$(date -u +%Y%m%d%H%M%S)-${RANDOM}"
CONTEXT="kind-idp-internal"
SCREEN="${ORCH_VIDEO_SCREEN:-1440x900}"
VAULT_IMAGE="${ORCH_VIDEO_VAULT_IMAGE:-hashicorp/vault:1.20}"
VIDEO_NAME="environment-connection-kind-${RUN_ID}.mp4"
OUT_BASE="${ORCH_RESULT_DIR:-/tmp/poc-environment-connection-review}"
VAULT_CONTAINER=""
STORE_CONTAINER=""
REVIEWER_NS="orch-${RUN_ID}-reviewer"
PIDS=()
BACKEND_PID=""
BROWSER_PGID=""
NODE_PID=""

STAGING_CONNECTION_NAME="${ORCH_E2E_STAGING_CONNECTION_NAME:-}"
if [[ -n "${STAGING_CONNECTION_NAME}" && ! "${STAGING_CONNECTION_NAME}" =~ ^[A-Za-z0-9]([A-Za-z0-9 -]{0,60}[A-Za-z0-9])?$ ]]; then
  echo "ORCH_E2E_STAGING_CONNECTION_NAME must be 1-62 letters, digits, spaces or hyphens" >&2
  exit 1
fi

WORK="${OUT_BASE}/${RUN_ID}"
[[ ! -e "${WORK}" ]] || { echo "${WORK} already exists" >&2; exit 1; }
mkdir -p "${OUT_BASE}"
mkdir -m 0700 "${WORK}"
PRIVATE="${WORK}/private"
mkdir -m 0700 "${PRIVATE}"
# shellcheck source=video-lib.sh
source "${ROOT}/test/integration/video-lib.sh"

# namespace_lookup <name>: prints namespace/<name> when it exists, nothing
# when the API server confirms it is absent; fails on any lookup error.
namespace_lookup() {
  kubectl --context "${CONTEXT}" get namespace "$1" --ignore-not-found -o name
}

# Only the session launched by this runner is terminated, including descendants.
stop_browser_group() {
  [[ -n "${BROWSER_PGID}" ]] || return 0
  kill -TERM -- "-${BROWSER_PGID}" 2>/dev/null || true
  for _ in $(seq 1 20); do
    kill -0 -- "-${BROWSER_PGID}" 2>/dev/null || break
    sleep 0.1
  done
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
  for _ in $(seq 1 50); do
    kill -0 "${BACKEND_PID}" 2>/dev/null || break
    sleep 0.1
  done
  kill -KILL "${BACKEND_PID}" 2>/dev/null || true
  wait "${BACKEND_PID}" 2>/dev/null || true
  BACKEND_PID=""
}

cleanup() {
  local status=$?
  local pid
  stop_browser_group || status=1
  stop_backend
  for pid in "${PIDS[@]}"; do kill "${pid}" 2>/dev/null || true; done
  for pid in "${PIDS[@]}"; do wait "${pid}" 2>/dev/null || true; done
  if [[ -s "${WORK}/namespace" ]]; then
    local namespace app_id actual present
    namespace="$(<"${WORK}/namespace")"
    if [[ "${namespace}" =~ ^app-([a-z0-9-]+)-staging$ ]]; then
      app_id="${BASH_REMATCH[1]}"
      if ! present="$(namespace_lookup "${namespace}")"; then
        echo "cleanup: namespace ${namespace} lookup failed" >&2
        status=1
      elif [[ -n "${present}" ]]; then
        # Delete only a namespace proven to be this run's: managed-by, application,
        # environment and the deployment run id the UI preview returned (the
        # executor stamps that pending-<hash> id, not the harness RUN_ID).
        local expected_run=""
        [[ -s "${WORK}/deployment-run-id" ]] && expected_run="$(<"${WORK}/deployment-run-id")"
        if [[ ! "${expected_run}" =~ ^pending-[0-9a-f]{16}$ ]]; then
          echo "namespace ${namespace} retained: no observed deployment run id" >&2
          status=1
        elif ! actual="$(kubectl --context "${CONTEXT}" get namespace "${namespace}" -o jsonpath='{.metadata.labels.app\.kubernetes\.io/managed-by}|{.metadata.labels.orchestrator\.io/application}|{.metadata.labels.orchestrator\.io/environment}|{.metadata.labels.orchestrator\.io/run-id}')"; then
          echo "cleanup: namespace ${namespace} label lookup failed" >&2
          status=1
        elif [[ "${actual}" == "orchestrator|${app_id}|staging|${expected_run}" ]]; then
          kubectl --context "${CONTEXT}" delete namespace "${namespace}" --ignore-not-found --wait=true --timeout=300s >/dev/null || status=1
        else
          echo "namespace ${namespace} retained: managed-by/application/environment/deployment-run-id label mismatch" >&2
          status=1
        fi
      fi
      if ! present="$(namespace_lookup "${namespace}")"; then
        echo "cleanup: namespace ${namespace} absence not verified: lookup failed" >&2
        status=1
      elif [[ -n "${present}" ]]; then
        echo "cleanup: namespace ${namespace} still exists" >&2
        status=1
      else
        echo "cleanup: namespace ${namespace} absent (successful API lookup)" | tee "${WORK}/cleanup.txt"
      fi
    fi
  fi
  if [[ -s "${WORK}/run.json" ]]; then
    local production_ns present_production
    production_ns="app-$(jq -r .applicationId "${WORK}/run.json")-production"
    if present_production="$(namespace_lookup "${production_ns}")" && [[ -z "${present_production}" ]]; then
      echo "cleanup: production namespace ${production_ns} absent (never created)" >> "${WORK}/cleanup.txt"
    else
      echo "cleanup: production namespace ${production_ns} unexpectedly present or lookup failed" >&2
      status=1
    fi
  fi
  # Run-owned reviewer identity: the global binding is deleted only when its run
  # label, roleRef and single subject are exactly what this run created.
  if [[ -e "${WORK}/reviewer-created" ]]; then
    local binding="orch-${RUN_ID}-reviewer" proof
    if proof="$(timeout 60 kubectl --context "${CONTEXT}" get clusterrolebinding "${binding}" --ignore-not-found -o jsonpath='{.metadata.labels.orchestrator\.io/run-id}|{.roleRef.kind}/{.roleRef.name}|{range .subjects[*]}{.kind}/{.namespace}/{.name};{end}' 2>/dev/null)"; then
      if [[ -z "${proof}" ]]; then
        :
      elif [[ "${proof}" == "${RUN_ID}|ClusterRole/system:auth-delegator|ServiceAccount/${REVIEWER_NS}/reviewer;" ]]; then
        timeout 60 kubectl --context "${CONTEXT}" delete clusterrolebinding "${binding}" --ignore-not-found >/dev/null 2>&1 || status=1
      else
        echo "cleanup: clusterrolebinding ${binding} does not match this run; left untouched" >&2; status=1
      fi
    else
      echo "cleanup: clusterrolebinding ${binding} lookup failed" >&2; status=1
    fi
    if [[ "$(timeout 60 kubectl --context "${CONTEXT}" get namespace "${REVIEWER_NS}" --ignore-not-found -o jsonpath='{.metadata.labels.orchestrator\.io/run-id}' 2>/dev/null || echo lookup-failed)" == "${RUN_ID}" ]]; then
      timeout 150 kubectl --context "${CONTEXT}" delete namespace "${REVIEWER_NS}" --ignore-not-found --wait=true --timeout=120s >/dev/null 2>&1 || status=1
    fi
    # Absence needs a successful --ignore-not-found read that returns nothing; a
    # lookup error is never treated as "removed".
    local ns_left crb_left
    if ns_left="$(timeout 60 kubectl --context "${CONTEXT}" get namespace "${REVIEWER_NS}" --ignore-not-found -o name 2>/dev/null)" \
      && crb_left="$(timeout 60 kubectl --context "${CONTEXT}" get clusterrolebinding "${binding}" --ignore-not-found -o name 2>/dev/null)"; then
      if [[ -z "${ns_left}" && -z "${crb_left}" ]]; then
        echo "cleanup: reviewer namespace and ClusterRoleBinding absent (successful API lookups)" >> "${WORK}/cleanup.txt"
      else
        echo "cleanup: reviewer identity still present" >&2; status=1
      fi
    else
      echo "cleanup: reviewer identity absence not verified: lookup failed" >&2; status=1
    fi
  fi
  if [[ -n "${STORE_CONTAINER}" ]]; then
    docker rm -f "${STORE_CONTAINER}" >/dev/null 2>&1 || true
    if docker container inspect "${STORE_CONTAINER}" >/dev/null 2>&1; then echo "WARNING: Vault container ${STORE_CONTAINER} is still present" >&2; status=1
    else echo "cleanup: Vault container ${STORE_CONTAINER} removed" >> "${WORK}/cleanup.txt"; fi
  fi
  if [[ -n "${VAULT_CONTAINER}" ]]; then docker rm -f "${VAULT_CONTAINER}" >/dev/null 2>&1 || true; fi
  rm -rf "${PRIVATE}"
  rm -f "${WORK}/state.json"
  if [[ -n "${VAULT_CONTAINER}" ]] && docker container inspect "${VAULT_CONTAINER}" >/dev/null 2>&1; then
    echo "WARNING: Vault container ${VAULT_CONTAINER} is still present" >&2
    status=1
  elif [[ -n "${VAULT_CONTAINER}" ]]; then
    echo "cleanup: Vault container ${VAULT_CONTAINER} removed" | tee -a "${WORK}/cleanup.txt"
  fi
  [[ ! -e "${PRIVATE}" ]] || { echo "WARNING: private credential directory remains" >&2; status=1; }
  [[ ! -e "${PRIVATE}" ]] && echo "cleanup: private credential directory removed" >> "${WORK}/cleanup.txt"
  [[ -s "${WORK}/${VIDEO_NAME}" ]] && echo "video=${WORK}/${VIDEO_NAME}"
  echo "run-id=${RUN_ID} evidence=${WORK} status=${status}"
  exit "${status}"
}
trap cleanup EXIT
trap 'exit 129' HUP
trap 'exit 130' INT
trap 'exit 143' TERM

synthetic_token() { head -c 32 /dev/urandom | od -An -tx1 | tr -d ' \n'; }
note() { echo "$*" | tee -a "${WORK}/checks.txt"; }

CURRENT_BEFORE="$(kubectl config current-context)"
[[ -d "${REPO}/frontend/node_modules/@playwright/test" ]]
video_require_tools
video_require_fresh_dist "${REPO}"
XDOTOOL="$(video_xdotool)"
command -v docker >/dev/null
command -v setsid >/dev/null
command -v jq >/dev/null
docker network inspect kind >/dev/null
kubectl config get-contexts -o name | grep -qx "${CONTEXT}"
kubectl --context "${CONTEXT}" version -o json >/dev/null
umask 077

# Upload file: selected context only, flattened, written straight to a
# private 0600 file and never printed.
UPLOAD="${PRIVATE}/upload.kubeconfig"
kubectl config view --raw --minify --flatten --context "${CONTEXT}" > "${UPLOAD}"
grep -q 'client-key-data:\|token:' "${UPLOAD}"
chmod 0600 "${UPLOAD}"

# Backend host kubeconfig: same cluster and CA, but an invalid token for the
# host context, so a host-context fallback cannot succeed.
HOSTCFG="${PRIVATE}/backend-host.kubeconfig"
cp "${UPLOAD}" "${HOSTCFG}"
chmod 0600 "${HOSTCFG}"
HOSTUSER="$(kubectl --kubeconfig "${HOSTCFG}" config view --minify -o jsonpath='{.users[0].name}')"
kubectl --kubeconfig "${HOSTCFG}" config unset "users.${HOSTUSER}.client-key-data" >/dev/null
kubectl --kubeconfig "${HOSTCFG}" config unset "users.${HOSTUSER}.client-certificate-data" >/dev/null
kubectl --kubeconfig "${HOSTCFG}" config set-credentials "${HOSTUSER}" --token="$(synthetic_token)" >/dev/null
if KUBECONFIG="${HOSTCFG}" kubectl --context "${CONTEXT}" get namespace default -o name >/dev/null 2>&1; then
  echo "backend host credential is unexpectedly valid" >&2
  exit 1
fi
note "preflight: backend host-context credential rejected by the cluster (expected)"
kubectl --kubeconfig "${UPLOAD}" get namespace default -o name >/dev/null
note "preflight: uploaded kubeconfig credential accepted by the cluster"
WHO_HOST="$(kubectl --context "${CONTEXT}" auth whoami -o jsonpath='{.status.userInfo.username}' 2>/dev/null || echo unknown)"
WHO_UPLOAD="$(kubectl --kubeconfig "${UPLOAD}" auth whoami -o jsonpath='{.status.userInfo.username}' 2>/dev/null || echo unknown)"
note "identity: default host context user=${WHO_HOST}; uploaded kubeconfig user=${WHO_UPLOAD}"

kubectl --context "${CONTEXT}" -n vault-secrets-operator-system rollout status deployment/vault-secrets-operator-controller-manager --timeout=60s >/dev/null
note "preflight: Vault Secrets Operator ready (the persistent vault-uc12 is not used)"

# Run-owned Kubernetes auth identity: Vault's TokenReview reviewer.
# Created with its run-id label in one request, so cleanup can always prove ownership.
: > "${WORK}/reviewer-created"
kubectl --context "${CONTEXT}" create -f - >/dev/null <<MANIFEST
apiVersion: v1
kind: Namespace
metadata:
  name: ${REVIEWER_NS}
  labels:
    orchestrator.io/run-id: ${RUN_ID}
MANIFEST
kubectl --context "${CONTEXT}" -n "${REVIEWER_NS}" create serviceaccount reviewer >/dev/null
kubectl --context "${CONTEXT}" create clusterrolebinding "orch-${RUN_ID}-reviewer" --clusterrole=system:auth-delegator --serviceaccount="${REVIEWER_NS}:reviewer" >/dev/null
kubectl --context "${CONTEXT}" label clusterrolebinding "orch-${RUN_ID}-reviewer" "orchestrator.io/run-id=${RUN_ID}" >/dev/null
kubectl --context "${CONTEXT}" -n "${REVIEWER_NS}" create token reviewer --duration=4h > "${PRIVATE}/reviewer.jwt"
kubectl config view --raw --minify --flatten --context "${CONTEXT}" -o jsonpath='{.clusters[0].cluster.certificate-authority-data}' | base64 -d > "${PRIVATE}/cluster-ca.pem"
CONTROL_PLANE="$(docker ps --filter 'name=idp-internal-control-plane' --format '{{.Names}}' | head -1)"
[[ -n "${CONTROL_PLANE}" ]]
K8S_HOST="https://${CONTROL_PLANE}:6443"

# Run-owned workload Secret Store: Vault dev container on the kind network (Pods
# reach it by IP) with a Kubernetes auth mount and a SCOPED token, never root.
STORE_CONTAINER="orch-${RUN_ID}-store"
STORE_ROOT="$(synthetic_token)"
printf 'X-Vault-Token: %s\n' "${STORE_ROOT}" > "${PRIVATE}/store-root-header"
printf 'VAULT_DEV_ROOT_TOKEN_ID=%s\nVAULT_DEV_LISTEN_ADDRESS=0.0.0.0:8200\n' "${STORE_ROOT}" > "${PRIVATE}/store-vault.env"
unset STORE_ROOT
docker run -d --rm --name "${STORE_CONTAINER}" --label "orchestrator.run-id=${RUN_ID}" --cap-add IPC_LOCK --network kind \
  --env-file "${PRIVATE}/store-vault.env" -p 127.0.0.1::8200 "${VAULT_IMAGE}" server -dev >/dev/null
STORE_PORT="$(docker port "${STORE_CONTAINER}" 8200/tcp | head -1 | sed 's/.*://')"
STORE_IP="$(docker inspect -f '{{(index .NetworkSettings.Networks "kind").IPAddress}}' "${STORE_CONTAINER}")"
STORE_HOST="http://127.0.0.1:${STORE_PORT}"
STORE_CLUSTER="http://${STORE_IP}:8200"
for _ in $(seq 1 60); do curl -fsS "${STORE_HOST}/v1/sys/health" >/dev/null 2>&1 && break; sleep 0.5; done
curl -fsS "${STORE_HOST}/v1/sys/health" >/dev/null
curl -fsS -H @"${PRIVATE}/store-root-header" -X POST "${STORE_HOST}/v1/sys/auth/kubernetes" -d '{"type":"kubernetes"}' >/dev/null
jq -n --arg host "${K8S_HOST}" --rawfile ca "${PRIVATE}/cluster-ca.pem" --rawfile jwt "${PRIVATE}/reviewer.jwt" \
  '{kubernetes_host:$host, kubernetes_ca_cert:$ca, token_reviewer_jwt:$jwt}' \
  | curl -fsS -H @"${PRIVATE}/store-root-header" -X POST "${STORE_HOST}/v1/auth/kubernetes/config" --data-binary @- >/dev/null
cat > "${PRIVATE}/store-policy.hcl" <<'POLICY'
path "auth/token/lookup-self" { capabilities = ["read"] }
path "sys/capabilities-self" { capabilities = ["update"] }
path "sys/internal/ui/mounts/*" { capabilities = ["read"] }
path "secret/data/orchestrator/apps/*" { capabilities = ["create", "read", "update"] }
path "secret/metadata/orchestrator/apps/*" { capabilities = ["read", "list", "delete"] }
path "sys/policies/acl/orch-*" { capabilities = ["create", "update", "read"] }
path "auth/kubernetes/role/orch-*" { capabilities = ["create", "update", "read"] }
path "auth/kubernetes/config" { capabilities = ["read"] }
POLICY
jq -n --rawfile p "${PRIVATE}/store-policy.hcl" '{policy:$p}' \
  | curl -fsS -H @"${PRIVATE}/store-root-header" -X PUT "${STORE_HOST}/v1/sys/policies/acl/orch-workload-store" --data-binary @- >/dev/null
curl -fsS -H @"${PRIVATE}/store-root-header" -X POST "${STORE_HOST}/v1/auth/token/create" \
  -d '{"policies":["orch-workload-store"],"no_default_policy":true,"ttl":"4h"}' \
  | node -e 'let s="";process.stdin.on("data",d=>s+=d).on("end",()=>{const t=JSON.parse(s).auth?.client_token;if(!t)process.exit(1);process.stdout.write(t)})' \
  > "${PRIVATE}/store-token"
[[ -s "${PRIVATE}/store-token" ]]
# Early auth preflight (the call behind CheckWorkloadAuth), status only: a store
# that cannot verify workload auth fails here in seconds, not after the deploy.
printf 'X-Vault-Token: %s\n' "$(<"${PRIVATE}/store-token")" > "${PRIVATE}/store-header"
auth_status="$(curl -sS -o /dev/null -w '%{http_code}' -H @"${PRIVATE}/store-header" "${STORE_HOST}/v1/auth/kubernetes/config")"
[[ "${auth_status}" == "200" ]] || { echo "preflight: scoped store token cannot read auth/kubernetes/config (HTTP ${auth_status})" >&2; exit 1; }
note "preflight: run-owned workload store ${STORE_CLUSTER} with Kubernetes auth; scoped token reads auth/kubernetes/config (HTTP 200)"

# Run-owned Vault dev container for Connection credentials.
VAULT_CONTAINER="orch-${RUN_ID}"
ROOT_TOKEN="$(synthetic_token)"
printf 'X-Vault-Token: %s\n' "${ROOT_TOKEN}" > "${PRIVATE}/root-header"
printf 'VAULT_DEV_ROOT_TOKEN_ID=%s\nVAULT_DEV_LISTEN_ADDRESS=0.0.0.0:8200\n' "${ROOT_TOKEN}" > "${PRIVATE}/vault.env"
unset ROOT_TOKEN
docker run -d --rm --name "${VAULT_CONTAINER}" --label "orchestrator.run-id=${RUN_ID}" --cap-add IPC_LOCK \
  --env-file "${PRIVATE}/vault.env" -p 127.0.0.1::8200 "${VAULT_IMAGE}" server -dev >/dev/null
CONN_PORT="$(docker port "${VAULT_CONTAINER}" 8200/tcp | head -1 | sed 's/.*://')"
CONN_VAULT="http://127.0.0.1:${CONN_PORT}"
for _ in $(seq 1 60); do curl -fsS "${CONN_VAULT}/v1/sys/health" >/dev/null 2>&1 && break; sleep 0.5; done
curl -fsS "${CONN_VAULT}/v1/sys/health" >/dev/null
curl -fsS -H @"${PRIVATE}/root-header" -X PUT "${CONN_VAULT}/v1/sys/policies/acl/orch-connection-credentials" \
  -d '{"policy":"path \"secret/data/orchestrator/connections/*\" { capabilities = [\"create\", \"read\"] }\npath \"secret/metadata/orchestrator/connections/*\" { capabilities = [\"read\", \"list\", \"delete\"] }\n"}' >/dev/null
curl -fsS -H @"${PRIVATE}/root-header" -X POST "${CONN_VAULT}/v1/auth/token/create" \
  -d '{"policies":["orch-connection-credentials"],"no_default_policy":true,"ttl":"3h"}' \
  | node -e 'let s="";process.stdin.on("data",d=>s+=d).on("end",()=>{const t=JSON.parse(s).auth?.client_token;if(!t)process.exit(1);process.stdout.write(t)})' \
  > "${PRIVATE}/connection-vault-token"
[[ -s "${PRIVATE}/connection-vault-token" ]]

cd "${ROOT}"
"${ROOT}/test/integration/build-images.sh" "${RUN_ID}" "${WORK}/bin"
for workload in frontend backend; do
  kind load docker-image "acceptance-${workload}:${RUN_ID}" --name idp-internal
done
go build -o "${WORK}/orchestrator" ./cmd/orchestrator

API_PORT="$(node -e 'const s=require("net").createServer();s.listen(0,"127.0.0.1",()=>{console.log(s.address().port);s.close()})')"
# start_backend <label>: the same flags and JSON state every time, so a restart
# reads the persisted Environment bindings back.
start_backend() {
  local label="$1"
  rm -f "${WORK}/api-addr"
  env -u ORCHESTRATOR_DATABASE_URL_FILE -u ORCHESTRATOR_VAULT_ADDR -u ORCHESTRATOR_VAULT_TOKEN_FILE \
    -u ORCHESTRATOR_VAULT_AGENT_ADDR -u ORCHESTRATOR_CONNECTION_CREDENTIAL_STORE -u ORCHESTRATOR_CONNECTION_VAULT_ADDR \
    -u ORCHESTRATOR_CONNECTION_VAULT_TOKEN_FILE -u TF_PLUGIN_CACHE_DIR -u AWS_PROFILE -u AWS_REGION -u AWS_DEFAULT_REGION \
    -u AWS_ACCESS_KEY_ID -u AWS_SECRET_ACCESS_KEY -u AWS_SESSION_TOKEN \
    KUBECONFIG="${HOSTCFG}" \
    "${WORK}/orchestrator" -addr "127.0.0.1:${API_PORT}" -addr-file "${WORK}/api-addr" -state "${WORK}/state.json" -database-url-file "" \
    -adapters kubernetes -kube-context "${CONTEXT}" -cluster idp-internal -run-id "${RUN_ID}" \
    -vault-delivery vso \
    -connection-credential-store vault -connection-vault-address "${CONN_VAULT}" \
    -connection-vault-token-file "${PRIVATE}/connection-vault-token" -connection-vault-mount secret \
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
# Register the run-owned store as an ordinary verified Secret Store (REST setup,
# not part of the shown flow) with the scoped token; it must verify READY with
# working Kubernetes auth before any browser or deploy work starts.
curl -fsS -c "${PRIVATE}/jar-pe" -H 'Content-Type: application/json' -d '{"username":"platform-engineer","password":"test-password"}' "${API}/api/v1/auth/sign-in" >/dev/null
store_status="$(jq -n --arg b "${STORE_HOST}" --arg w "${STORE_CLUSTER}" --rawfile t "${PRIVATE}/store-token"   '{name:"Run store",backendAddress:$b,workloadAddress:$w,mount:"secret",authMount:"kubernetes",token:($t|rtrimstr("\n"))}'   | curl -sS -o "${WORK}/secret-store.json" -w '%{http_code}' -b "${PRIVATE}/jar-pe" -H 'Content-Type: application/json' --data-binary @- "${API}/api/v1/secret-stores")"
if [[ "${store_status}" != 2* ]] || [[ "$(jq -r '.status // empty' "${WORK}/secret-store.json" 2>/dev/null)" != "READY" ]]; then
  # Fixed text plus allowlisted fields only; the raw reply (which may echo input) is discarded.
  echo "secret store registration failed: HTTP ${store_status} status=$(jq -r '.status // "unknown"' "${WORK}/secret-store.json" 2>/dev/null | head -c 40) code=$(jq -r '.code // "none"' "${WORK}/secret-store.json" 2>/dev/null | head -c 40)" >&2
  rm -f "${WORK}/secret-store.json"
  exit 1
fi
STORE_KEY="$(jq -r .key "${WORK}/secret-store.json")"
note "store: ${STORE_KEY} registered READY (kubernetesAuth=$(jq -r '.verification.kubernetesAuth // "unknown"' "${WORK}/secret-store.json"))"
# Keep only an allowlisted projection (no credential reference or verification detail).
jq '{key, name, status, authMount, mount, kubernetesAuth: .verification.kubernetesAuth}' "${WORK}/secret-store.json" > "${WORK}/secret-store.safe.json" && mv "${WORK}/secret-store.safe.json" "${WORK}/secret-store.json"
video_require_secret_store "${API}" "${STORE_KEY}"

video_start_xvfb "${SCREEN}"
setsid env DISPLAY="${VIDEO_DISPLAY}" ORCH_E2E_SCREEN="${SCREEN}" ORCH_E2E_XDOTOOL="${XDOTOOL}" \
  ORCH_E2E_URL="${API}" ORCH_E2E_RUN_ID="${RUN_ID}" ORCH_E2E_KUBE_CONTEXT="${CONTEXT}" \
  ORCH_E2E_NAMESPACE_FILE="${WORK}/namespace" ORCH_E2E_EVIDENCE_DIR="${WORK}" \
  ORCH_E2E_KUBECONFIG_FILE="${UPLOAD}" ORCH_E2E_VIDEO_NAME="${VIDEO_NAME}" \
  ORCH_E2E_STAGING_CONNECTION_NAME="${STAGING_CONNECTION_NAME}" \
  node "${REPO}/frontend/test/e2e/application-connection-kind-human.mjs" > "${WORK}/playwright.log" 2>&1 &
NODE_PID=$!
BROWSER_PGID=${NODE_PID}
BROWSER_DEADLINE=$((SECONDS + 1200))
# The browser flow asks the runner to restart the API (same flags, state file
# and port) once both bindings are saved, then continues after a refresh.
while kill -0 "${NODE_PID}" 2>/dev/null; do
  (( SECONDS < BROWSER_DEADLINE )) || { echo "browser flow exceeded 1200 seconds" >&2; exit 1; }
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

# Observer assertions on the live cluster and persisted state.
NS="$(<"${WORK}/namespace")"
APP="$(jq -r .applicationId "${WORK}/run.json")"
STAGING_KEY="$(jq -r .stagingKey "${WORK}/run.json")"
PRODUCTION_KEY="$(jq -r .productionKey "${WORK}/run.json")"
[[ "${STAGING_KEY}" =~ ^[a-z0-9-]+$ && "${STAGING_KEY}" != "internal-cluster" ]]
[[ "${PRODUCTION_KEY}" =~ ^[a-z0-9-]+$ && "${PRODUCTION_KEY}" != "internal-cluster" && "${PRODUCTION_KEY}" != "${STAGING_KEY}" ]]
kubectl --context "${CONTEXT}" get namespace "${NS}" -o jsonpath='{.metadata.labels}' > "${WORK}/namespace-labels.json"
DEPLOY_RUN_ID="$(<"${WORK}/deployment-run-id")"
[[ "${DEPLOY_RUN_ID}" =~ ^pending-[0-9a-f]{16}$ && "$(jq -r .deploymentRunId "${WORK}/run.json")" == "${DEPLOY_RUN_ID}" ]]
[[ "$(kubectl --context "${CONTEXT}" get namespace "${NS}" -o jsonpath='{.metadata.labels.orchestrator\.io/run-id}')" == "${DEPLOY_RUN_ID}" ]]
note "k8s: namespace ${NS} carries the observed deployment run id ${DEPLOY_RUN_ID} (not the harness id)"
kubectl --context "${CONTEXT}" -n "${NS}" get deployment,statefulset,service,pod -o wide > "${WORK}/k8s-workloads.txt"
for deployment in backend frontend; do
  kubectl --context "${CONTEXT}" -n "${NS}" rollout status "deployment/${deployment}" --timeout=60s >/dev/null
  [[ "$(kubectl --context "${CONTEXT}" -n "${NS}" get "deployment/${deployment}" -o jsonpath='{.status.availableReplicas}')" -ge 1 ]]
  note "k8s: deployment/${deployment} available in ${NS}"
done
# PostgreSQL (StatefulSet) Ready and every PVC Bound.
mapfile -t STATEFULSETS < <(kubectl --context "${CONTEXT}" -n "${NS}" get statefulset -o name)
(( ${#STATEFULSETS[@]} >= 1 ))
for sts in "${STATEFULSETS[@]}"; do
  kubectl --context "${CONTEXT}" -n "${NS}" rollout status "${sts}" --timeout=120s >/dev/null
  [[ "$(kubectl --context "${CONTEXT}" -n "${NS}" get "${sts}" -o jsonpath='{.status.readyReplicas}')" -ge 1 ]]
  note "k8s: ${sts} Ready in ${NS}"
done
PVC_PHASES="$(kubectl --context "${CONTEXT}" -n "${NS}" get pvc -o jsonpath='{range .items[*]}{.metadata.name}={.status.phase}{"\n"}{end}')"
[[ -n "${PVC_PHASES}" ]]
if grep -v '=Bound$' <<<"${PVC_PHASES}" | grep -q .; then echo "a PVC in ${NS} is not Bound" >&2; exit 1; fi
note "k8s: PVCs Bound in ${NS}: $(tr '\n' ' ' <<<"${PVC_PHASES}")"
kubectl --context "${CONTEXT}" -n "${NS}" get service backend frontend -o name | sed 's/^/k8s: /' | tee -a "${WORK}/checks.txt"
[[ -z "$(kubectl --context "${CONTEXT}" -n "${NS}" get pod --no-headers | grep -v -E ' (Running|Completed) ' || true)" ]]
note "k8s: all pods in ${NS} Running/Completed"
PRODUCTION_NS="app-${APP}-production"
[[ -z "$(namespace_lookup "${PRODUCTION_NS}")" ]]
note "k8s: production namespace ${PRODUCTION_NS} does not exist (production is bound but not deployed)"

# Persisted Environment bindings, Application and Active Resources (names/keys only).
node -e '
  const [path, app, stagingKey, productionKey] = process.argv.slice(1);
  const state = JSON.parse(require("fs").readFileSync(path, "utf8"));
  const application = state.applications?.[app];
  const staging = state.environments?.[`${app}/staging`];
  const production = state.environments?.[`${app}/production`];
  const fail = (message) => { console.error(message); process.exit(1); };
  if (!application || application.connectionKey || application.executionProfile || application.region) fail("the Application must carry no execution target");
  const check = (env, key, label) => {
    if (!env || env.connectionKey !== key || env.executionProfile !== "internal-k8s" || env.runtimeStatus !== "READY" || env.infrastructureScope !== "ENVIRONMENT") fail(`${label} binding is not the selected Environment binding`);
  };
  check(staging, stagingKey, "staging");
  check(production, productionKey, "production");
  const active = Object.entries(state.activeResources ?? {}).filter(([k]) => k.includes(app));
  if (!active.length) fail("no staging Active Resources");
  if (active.some(([, v]) => v.connectionKey !== stagingKey)) fail("an Active Resource is not bound to the staging Connection");
  // ADR-013: the Environment Connection backs the implicit cluster through the
  // system-owned builtin-existing-cluster Definition; no cluster Definition was authored.
  // resourceDefinitions is the JSON store collection (seeded Definitions): it must be
  // present and non-empty, or the authored-Definition check below would prove nothing.
  if (!state.resourceDefinitions || !Object.keys(state.resourceDefinitions).length) fail("resourceDefinitions collection missing or empty: the Definition assertion would be vacuous");
  const authoredClusters = Object.values(state.resourceDefinitions).filter((d) => d.driverType === "existing-cluster" && d.key !== "builtin-existing-cluster");
  if (authoredClusters.length) fail("an authored cluster Definition exists");
  const builtinUsed = active.filter(([, v]) => v.definitionKey === "builtin-existing-cluster");
  if (!builtinUsed.length) fail("builtin-existing-cluster was not used by an Active Resource");
  if (builtinUsed.some(([, v]) => v.connectionKey !== stagingKey)) fail("builtin-existing-cluster is not bound to the staging Connection");
  const everything = Object.entries(state.activeResources ?? {});
  if (everything.some(([k, v]) => k.includes(`connections.${productionKey}`) || v.connectionKey === productionKey)) fail("production Connection executed something");
  const instances = Object.values(state.workloadInstances ?? {}).filter((w) => JSON.stringify(w).includes(app));
  if (!instances.length || instances.some((w) => w.targetRef?.connection !== stagingKey)) fail("workload TargetRef is not the staging Connection");
  const deployments = Object.values(state.deployments ?? {}).filter((d) => d.applicationKey === app);
  if (!deployments.length || deployments.some((d) => d.environmentKey !== "staging" || d.executionProfile !== "internal-k8s")) fail("unexpected deployments");
  console.log(JSON.stringify({
    application: { connectionKey: application.connectionKey ?? "", executionProfile: application.executionProfile ?? "" },
    environments: { staging: { connectionKey: staging.connectionKey, version: staging.version, scope: staging.infrastructureScope }, production: { connectionKey: production.connectionKey, version: production.version, scope: production.infrastructureScope } },
    activeResources: active.map(([k, v]) => ({ id: k.split("|")[1], connectionKey: v.connectionKey, definitionKey: v.definitionKey })),
    builtinClusterResources: builtinUsed.length,
    authoredClusterDefinitions: authoredClusters.length,
    definitionsInStore: Object.keys(state.resourceDefinitions).length,
    workloadTargetConnections: [...new Set(instances.map((w) => w.targetRef.connection))],
    deployments: deployments.map((d) => ({ environment: d.environmentKey, status: d.status })),
  }));
' "${WORK}/state.json" "${APP}" "${STAGING_KEY}" "${PRODUCTION_KEY}" | tee "${WORK}/persisted-binding.json"
note "persisted: Application unbound; staging=${STAGING_KEY} production=${PRODUCTION_KEY} (rebound from the default); every Active Resource and workload TargetRef bound to ${STAGING_KEY}; builtin-existing-cluster used, no authored cluster Definition"

# Final API read-back after the restart: both bindings are served and a repeat set is refused.
curl -fsS -c "${PRIVATE}/jar" -H 'Content-Type: application/json' -d '{"username":"developer","password":"test-password"}' "${API}/api/v1/auth/sign-in" >/dev/null
curl -fsS -b "${PRIVATE}/jar" "${API}/api/v1/applications/${APP}" > "${WORK}/application-view.json"
[[ "$(jq -r '.application.environments[] | select(.key=="staging") | .connectionKey' "${WORK}/application-view.json")" == "${STAGING_KEY}" ]]
[[ "$(jq -r '.application.environments[] | select(.key=="production") | .connectionKey' "${WORK}/application-view.json")" == "${PRODUCTION_KEY}" ]]
[[ "$(jq -r '[.application.environments[] | select(.configured == true)] | length' "${WORK}/application-view.json")" == "2" ]]
# Read-only-in-effect probes (each is refused or a no-op and must leave the bindings
# unchanged): staging has runtime resources, so a different destination at the
# CURRENT version is refused with RUNTIME_EXISTS; production has none, so only a
# stale version is refused (STALE_VERSION). No permanent lock is expected.
env_field() { jq -r --arg env "$1" --arg field "$2" '.application.environments[] | select(.key == $env) | .[$field]' "${WORK}/application-view.json"; }
STAGING_VERSION="$(env_field staging version)"
PRODUCTION_VERSION="$(env_field production version)"
(( PRODUCTION_VERSION > 1 ))
status="$(curl -s -o "${WORK}/probe-staging.json" -w '%{http_code}' -b "${PRIVATE}/jar" -X PUT -H 'Content-Type: application/json' \
  -d "{\"connectionKey\":\"internal-cluster\",\"expectedVersion\":${STAGING_VERSION}}" "${API}/api/v1/applications/${APP}/environments/staging/connection")"
[[ "${status}" == "409" && "$(jq -r .code "${WORK}/probe-staging.json")" == "RUNTIME_EXISTS" ]]
status="$(curl -s -o "${WORK}/probe-production.json" -w '%{http_code}' -b "${PRIVATE}/jar" -X PUT -H 'Content-Type: application/json' \
  -d "{\"connectionKey\":\"internal-cluster\",\"expectedVersion\":1}" "${API}/api/v1/applications/${APP}/environments/production/connection")"
[[ "${status}" == "409" && "$(jq -r .code "${WORK}/probe-production.json")" == "STALE_VERSION" ]]
curl -fsS -b "${PRIVATE}/jar" "${API}/api/v1/applications/${APP}" > "${WORK}/application-view-after-probes.json"
[[ "$(jq -S '[.application.environments[] | {key, connectionKey, version}]' "${WORK}/application-view.json")" == "$(jq -S '[.application.environments[] | {key, connectionKey, version}]' "${WORK}/application-view-after-probes.json")" ]]
note "api: both bindings served after restart; staging refuses a direct change (409 RUNTIME_EXISTS, transition required), production refuses a stale version (409 STALE_VERSION); refused probes left every binding and version unchanged"
# The uploaded credential must not appear in any evidence file.
EXTRA_SECRET_FILES="${PRIVATE}/store-token:${PRIVATE}/connection-vault-token:${PRIVATE}/reviewer.jwt" node -e '
  const fs = require("fs");
  const [upload, ...files] = process.argv.slice(1);
  const values = [...fs.readFileSync(upload, "utf8").matchAll(/^\s*(?:token|client-key-data|client-certificate-data):\s*(\S+)\s*$/gm)].map((m) => m[1]).filter((v) => v.length >= 12);
  // Run-owned tokens and the reviewer JWT must not appear either (whole-file values).
  for (const extra of (process.env.EXTRA_SECRET_FILES || "").split(":").filter(Boolean)) {
    if (fs.existsSync(extra)) { const v = fs.readFileSync(extra, "utf8").trim(); if (v.length >= 12) values.push(v); }
  }
  for (const file of files) {
    if (!fs.existsSync(file)) continue;
    const text = fs.readFileSync(file, "utf8");
    if (values.some((v) => text.includes(v))) { console.error(`credential material found in ${file}`); process.exit(1); }
  }
  console.log(`no-credential-in-artifacts files=${files.length} values=${values.length}`);
' "${UPLOAD}" "${WORK}/orchestrator-first.log" "${WORK}/orchestrator-restarted.log" "${WORK}/playwright.log" "${WORK}/run.json" "${WORK}/marks.json" "${WORK}/state.json" "${WORK}/k8s-workloads.txt" "${WORK}/checks.txt" "${WORK}/application-view.json" "${WORK}/persisted-binding.json" | tee -a "${WORK}/checks.txt"
[[ "$(kubectl config current-context)" == "${CURRENT_BEFORE}" ]]
note "current kubectl context unchanged: ${CURRENT_BEFORE}"
# Record the run for the result report (no credential material).
jq -n --arg run "${RUN_ID}" --arg app "${APP}" --arg staging "${STAGING_KEY}" --arg production "${PRODUCTION_KEY}" --arg ns "${NS}" \
  '{runId:$run, applicationId:$app, stagingConnection:$staging, productionConnection:$production, stagingNamespace:$ns, clusterDefinition:"builtin-existing-cluster", authoredClusterDefinitions:0, limit:"both logical Connections point at the same physical kind cluster"}' > "${WORK}/summary.json"

stop_backend
video_validate "${WORK}/${VIDEO_NAME}" "${SCREEN}" "${ORCH_VIDEO_MIN_SECONDS:-420}" 31 job-submitted
