#!/usr/bin/env bash
# Live, recorded UC-01 Application-level Connection selection on the existing
# kind-idp-internal cluster (real Kubernetes adapters, no fakes or mocks).
#
# A Platform Engineer uploads a kubeconfig (new READY nondefault Connection)
# and registers the matching existing-cluster Definition; a Developer creates
# an Application selecting that Connection, deploys the diagnostic acceptance
# workloads with PostgreSQL and UC-12 references, and the browser checks the
# deployed app. Every shown mutation is a UI action.
#
# Identity: the uploaded Connection and the seeded default point at the same
# physical cluster and credential. To prove the uploaded credential drives
# execution, the backend process runs with a private KUBECONFIG whose host
# context carries an invalid token: any fallback to the host context fails.
#
# Safety: existing kind-idp-internal only; the current context is never
# changed; every command passes --context. Only this run's namespace (label
# checked) is deleted. The Connection credential goes to a run-owned Vault dev
# container; the platform Vault is used as the runbook describes (scoped token
# file consumed by path). Credential files live in a private directory that is
# removed on exit. Evidence stays outside Git.
#
# Output: /tmp/poc-application-connection-live-review/<run id>/
# (application-connection-kind-<run id>.mp4, marks.json, ffprobe.json,
# frames/, checks.txt, run.json, logs).
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
REPO="$(cd "${ROOT}/.." && pwd)"
RUN_ID="appconn-kind-$(date -u +%Y%m%d%H%M%S)-${RANDOM}"
CONTEXT="kind-idp-internal"
TOKEN_FILE="${VAULT_BACKEND_TOKEN_FILE:-/home/thanhnt1/.local/share/poc-vault/vault-uc12-backend-token}"
SCREEN="${ORCH_VIDEO_SCREEN:-1440x900}"
VAULT_IMAGE="${ORCH_VIDEO_VAULT_IMAGE:-hashicorp/vault:1.20}"
VIDEO_NAME="application-connection-kind-${RUN_ID}.mp4"
OUT_BASE="${ORCH_RESULT_DIR:-/tmp/poc-application-connection-live-review}"
VAULT_CONTAINER=""
PIDS=()

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

cleanup() {
  local status=$?
  local pid
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
        if ! actual="$(kubectl --context "${CONTEXT}" get namespace "${namespace}" -o jsonpath='{.metadata.labels.orchestrator\.io/application}')"; then
          echo "cleanup: namespace ${namespace} label lookup failed" >&2
          status=1
        elif [[ "${actual}" == "${app_id}" ]]; then
          kubectl --context "${CONTEXT}" delete namespace "${namespace}" --ignore-not-found --wait=true --timeout=300s >/dev/null || status=1
        else
          echo "namespace ${namespace} retained: application label mismatch" >&2
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
[[ -s "${TOKEN_FILE}" ]]
[[ -d "${REPO}/frontend/node_modules/@playwright/test" ]]
video_require_tools
video_require_fresh_dist "${REPO}"
XDOTOOL="$(video_xdotool)"
command -v docker >/dev/null
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

kubectl --context "${CONTEXT}" -n vault wait --for=condition=Ready pod/vault-uc12-0 --timeout=60s >/dev/null
kubectl --context "${CONTEXT}" -n vault-secrets-operator-system rollout status deployment/vault-secrets-operator-controller-manager --timeout=60s >/dev/null
kubectl --context "${CONTEXT}" -n vault port-forward svc/vault-uc12 :8200 > "${WORK}/vault-port-forward.log" 2>&1 &
VAULT_PF_PID=$!
PIDS+=("${VAULT_PF_PID}")
VAULT_PORT=""
for _ in $(seq 1 40); do
  kill -0 "${VAULT_PF_PID}" 2>/dev/null || { echo "Vault port-forward exited" >&2; exit 1; }
  VAULT_PORT="$(sed -n 's/^Forwarding from 127\.0\.0\.1:\([0-9][0-9]*\) -> 8200$/\1/p' "${WORK}/vault-port-forward.log" | head -n 1)"
  [[ -n "${VAULT_PORT}" ]] && break
  sleep 0.5
done
[[ -n "${VAULT_PORT}" ]]
PLATFORM_VAULT="http://127.0.0.1:${VAULT_PORT}"
for _ in $(seq 1 40); do curl -fsS "${PLATFORM_VAULT}/v1/sys/seal-status" > "${WORK}/seal-status.json" 2>/dev/null && break; sleep 0.5; done
[[ "$(jq -r .sealed "${WORK}/seal-status.json")" == "false" ]]

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
kill -0 "${VAULT_PF_PID}" 2>/dev/null

env -u ORCHESTRATOR_DATABASE_URL_FILE -u ORCHESTRATOR_VAULT_ADDR -u ORCHESTRATOR_VAULT_TOKEN_FILE \
  -u ORCHESTRATOR_VAULT_AGENT_ADDR -u ORCHESTRATOR_CONNECTION_CREDENTIAL_STORE -u ORCHESTRATOR_CONNECTION_VAULT_ADDR \
  -u ORCHESTRATOR_CONNECTION_VAULT_TOKEN_FILE -u TF_PLUGIN_CACHE_DIR -u AWS_PROFILE -u AWS_REGION -u AWS_DEFAULT_REGION \
  -u AWS_ACCESS_KEY_ID -u AWS_SECRET_ACCESS_KEY -u AWS_SESSION_TOKEN \
  KUBECONFIG="${HOSTCFG}" \
  "${WORK}/orchestrator" -addr 127.0.0.1:0 -addr-file "${WORK}/api-addr" -state "${WORK}/state.json" -database-url-file "" \
  -adapters kubernetes -kube-context "${CONTEXT}" -cluster idp-internal -run-id "${RUN_ID}" \
  -vault-address "${PLATFORM_VAULT}" -vault-token-file "${TOKEN_FILE}" \
  -vault-agent-address http://vault-uc12.vault.svc:8200 -vault-delivery vso \
  -connection-credential-store vault -connection-vault-address "${CONN_VAULT}" \
  -connection-vault-token-file "${PRIVATE}/connection-vault-token" -connection-vault-mount secret \
  -ui-dir "${REPO}/frontend/dist" > "${WORK}/orchestrator.log" 2>&1 &
PIDS+=($!)
for _ in $(seq 1 80); do [[ -s "${WORK}/api-addr" ]] && break; sleep 0.5; done
[[ -s "${WORK}/api-addr" ]] || { echo "backend did not start; see ${WORK}/orchestrator.log" >&2; exit 1; }
API="http://$(<"${WORK}/api-addr")"
for _ in $(seq 1 40); do curl -fsS "${API}/api/v1/healthz" >/dev/null 2>&1 && break; sleep 0.5; done
curl -fsS "${API}/api/v1/healthz" >/dev/null

video_start_xvfb "${SCREEN}"
DISPLAY="${VIDEO_DISPLAY}" ORCH_E2E_SCREEN="${SCREEN}" ORCH_E2E_XDOTOOL="${XDOTOOL}" \
  ORCH_E2E_URL="${API}" ORCH_E2E_RUN_ID="${RUN_ID}" ORCH_E2E_KUBE_CONTEXT="${CONTEXT}" \
  ORCH_E2E_NAMESPACE_FILE="${WORK}/namespace" ORCH_E2E_EVIDENCE_DIR="${WORK}" \
  ORCH_E2E_KUBECONFIG_FILE="${UPLOAD}" ORCH_E2E_VIDEO_NAME="${VIDEO_NAME}" \
  node "${REPO}/frontend/test/e2e/application-connection-kind-human.mjs" 2>&1 | tee "${WORK}/playwright.log"
[[ "${PIPESTATUS[0]}" == "0" ]]

# Observer assertions on the live cluster and persisted state.
NS="$(<"${WORK}/namespace")"
APP="$(jq -r .applicationId "${WORK}/run.json")"
KEY="$(jq -r .connectionKey "${WORK}/run.json")"
[[ "${KEY}" =~ ^[a-z0-9-]+$ && "${KEY}" != "internal-cluster" ]]
kubectl --context "${CONTEXT}" get namespace "${NS}" -o jsonpath='{.metadata.labels}' > "${WORK}/namespace-labels.json"
kubectl --context "${CONTEXT}" -n "${NS}" get deployment,statefulset,service,pod -o wide > "${WORK}/k8s-workloads.txt"
for deployment in backend frontend; do
  kubectl --context "${CONTEXT}" -n "${NS}" rollout status "deployment/${deployment}" --timeout=60s >/dev/null
  [[ "$(kubectl --context "${CONTEXT}" -n "${NS}" get "deployment/${deployment}" -o jsonpath='{.status.availableReplicas}')" -ge 1 ]]
  note "k8s: deployment/${deployment} available in ${NS}"
done
kubectl --context "${CONTEXT}" -n "${NS}" get service backend frontend -o name | sed 's/^/k8s: /' | tee -a "${WORK}/checks.txt"
[[ -z "$(kubectl --context "${CONTEXT}" -n "${NS}" get pod --no-headers | grep -v -E ' (Running|Completed) ' || true)" ]]
note "k8s: all pods in ${NS} Running/Completed"
# Persisted Application and Active Resource bindings (names/keys only).
node -e '
  const [path, app, key] = process.argv.slice(1);
  const state = JSON.parse(require("fs").readFileSync(path, "utf8"));
  const apps = Object.values(state.applications ?? {}).filter((a) => JSON.stringify(a).includes(app));
  const found = apps.find((a) => (a.id ?? a.ID ?? a.Id) === app || a.connectionKey || a.ConnectionKey);
  const conn = found && (found.connectionKey ?? found.ConnectionKey);
  const active = Object.entries(state.activeResources ?? {}).filter(([k]) => k.includes(app));
  const out = { applicationConnection: conn, activeResources: active.map(([k, v]) => ({ id: k.split("|")[1], connectionKey: v.connectionKey })) };
  console.log(JSON.stringify(out));
  if (conn !== key || !active.length || active.some(([, v]) => v.connectionKey !== key)) process.exit(1);
' "${WORK}/state.json" "${APP}" "${KEY}" | tee "${WORK}/persisted-binding.json"
note "persisted: Application and every Active Resource bound to ${KEY}"
# The uploaded credential must not appear in any evidence file.
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
' "${UPLOAD}" "${WORK}/orchestrator.log" "${WORK}/playwright.log" "${WORK}/run.json" "${WORK}/marks.json" "${WORK}/state.json" "${WORK}/k8s-workloads.txt" "${WORK}/checks.txt" | tee -a "${WORK}/checks.txt"
[[ "$(kubectl config current-context)" == "${CURRENT_BEFORE}" ]]
note "current kubectl context unchanged: ${CURRENT_BEFORE}"

video_validate "${WORK}/${VIDEO_NAME}" "${SCREEN}" "${ORCH_VIDEO_MIN_SECONDS:-300}" 18 job-submitted
