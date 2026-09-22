#!/usr/bin/env bash
# Internal happy path on an existing kind cluster.
#
# The script discovers the cluster, builds and loads the acceptance images, runs
# the UC-06/UC-08 integration test, verifies the Web Console against the same
# state and then deletes every object it created. It never creates or deletes a
# kind cluster and never touches a namespace outside the run id.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
WORK="${WORK_DIR:-$(mktemp -d)}"
RUN_ID="${RUN_ID:-kind-$(date -u +%Y%m%d%H%M%S)-$RANDOM}"
NAMESPACE="acceptance-${RUN_ID}"
STATE_PATH="${WORK}/state.json"
API_PID=""
DEPLOYMENT_ID_FILE="${WORK}/deployment-id"
EVIDENCE="${WORK}/evidence"
mkdir -p "${EVIDENCE}"

CLUSTER="${KIND_CLUSTER:-$(kind get clusters | head -1)}"
if [[ -z "${CLUSTER}" ]]; then
  echo "no kind cluster found" >&2
  exit 1
fi
CONTEXT="kind-${CLUSTER}"

cleanup() {
  local status=$?
  if [[ -n "${API_PID}" ]]; then
    kill "${API_PID}" 2>/dev/null || true
  fi
  if [[ "${KEEP_NAMESPACE:-0}" == "1" && "${status}" != "0" ]]; then
    echo "--- keeping namespace ${NAMESPACE} for diagnosis (KEEP_NAMESPACE=1)"
    kubectl --context "${CONTEXT}" get pods -n "${NAMESPACE}" -o wide || true
    exit "${status}"
  fi
  echo "--- cleanup: namespace ${NAMESPACE}"
  kubectl --context "${CONTEXT}" delete namespace "${NAMESPACE}" --ignore-not-found --wait=true >/dev/null 2>&1 || true
  echo "--- cleanup: objects still carrying run id ${RUN_ID}"
  kubectl --context "${CONTEXT}" get all,pvc,secret,namespace -A -l "orchestrator.io/run-id=${RUN_ID}" 2>/dev/null | tee "${EVIDENCE}/cleanup-check.txt" || true
  kubectl --context "${CONTEXT}" get namespace "${NAMESPACE}" >/dev/null 2>&1 && echo "WARNING: namespace ${NAMESPACE} still exists" || echo "namespace ${NAMESPACE} is gone"
  exit "${status}"
}
trap cleanup EXIT

echo "=== kind cluster: ${CLUSTER} (context ${CONTEXT})"
kubectl --context "${CONTEXT}" version -o json | head -20 > "${EVIDENCE}/kubectl-version.json"
kubectl --context "${CONTEXT}" get nodes -o wide | tee "${EVIDENCE}/nodes.txt"

echo "=== building acceptance images (tag ${RUN_ID})"
"${ROOT}/test/integration/build-images.sh" "${RUN_ID}" "${WORK}/bin"

echo "=== loading images into kind"
for workload in frontend backend worker; do
  kind load docker-image "acceptance-${workload}:${RUN_ID}" --name "${CLUSTER}"
done
docker image inspect postgres:16-alpine >/dev/null 2>&1 || docker pull postgres:16-alpine
# A multi-platform image cannot be loaded directly, so load a single-platform
# archive. If that fails the kubelet pulls the image itself.
if docker save postgres:16-alpine -o "${WORK}/postgres.tar"; then
  kind load image-archive "${WORK}/postgres.tar" --name "${CLUSTER}" || echo "postgres image not preloaded; the kubelet will pull it"
  rm -f "${WORK}/postgres.tar"
fi

echo "=== starting the orchestrator in kubernetes mode"
cd "${ROOT}"
go build -o "${WORK}/orchestrator" ./cmd/orchestrator
"${WORK}/orchestrator" -addr 127.0.0.1:0 -addr-file "${WORK}/api-addr" -ui-dir "${ROOT}/../frontend/dist" \
  -adapters kubernetes -state "${STATE_PATH}" -namespace "${NAMESPACE}" \
  -kube-context "${CONTEXT}" -cluster "${CLUSTER}" -run-id "${RUN_ID}" \
  -frontend-image "acceptance-frontend:${RUN_ID}" \
  -backend-image "acceptance-backend:${RUN_ID}" \
  -worker-image "acceptance-worker:${RUN_ID}" \
  > "${EVIDENCE}/orchestrator.log" 2>&1 &
API_PID=$!
for _ in $(seq 1 60); do
  [[ -f "${WORK}/api-addr" ]] && break
  sleep 0.5
done
API_ADDR="$(cat "${WORK}/api-addr")"
for _ in $(seq 1 60); do
  curl -sf "http://${API_ADDR}/api/v1/healthz" > /dev/null && break
  sleep 0.5
done
echo "api=${API_ADDR}"

echo "=== deploying backend, worker and frontend over the HTTP API"
go run ./test/integration/deployctl -api "http://${API_ADDR}" \
  -application acceptance -environment dev \
  -actor kind-verify -workloads backend,worker,frontend \
  | tee "${EVIDENCE}/deployments.txt"

echo "=== verifying the internal happy path"
KIND_CONTEXT="${CONTEXT}" RUN_ID="${RUN_ID}" STATE_PATH="${STATE_PATH}" NAMESPACE="${NAMESPACE}" \
  DEPLOYMENT_ID_FILE="${DEPLOYMENT_ID_FILE}" \
  go test -tags integration -count=1 -timeout 20m -v -run TestKindInternalVerification ./test/integration/ 2>&1 \
  | tee "${EVIDENCE}/go-integration.log"

echo "=== cluster evidence"
kubectl --context "${CONTEXT}" get all -n "${NAMESPACE}" | tee "${EVIDENCE}/resources.txt"
kubectl --context "${CONTEXT}" get pods -n "${NAMESPACE}" -o wide | tee "${EVIDENCE}/pods.txt"
kubectl --context "${CONTEXT}" get pvc -n "${NAMESPACE}" | tee "${EVIDENCE}/pvc.txt"
kubectl --context "${CONTEXT}" get deployments -n "${NAMESPACE}" \
  -o jsonpath='{range .items[*]}{.metadata.name}{"\t"}{.spec.template.spec.containers[*].resources}{"\n"}{end}' \
  | tee "${EVIDENCE}/container-resources.txt"
kubectl --context "${CONTEXT}" logs -n "${NAMESPACE}" deployment/worker --tail=20 | tee "${EVIDENCE}/worker.log"
kubectl --context "${CONTEXT}" get secret -n "${NAMESPACE}" -o name | tee "${EVIDENCE}/secrets.txt"

DEPLOYMENT_ID="$(cat "${DEPLOYMENT_ID_FILE}")"
echo "=== verifying the Web Console against deployment ${DEPLOYMENT_ID}"
curl -s "http://${API_ADDR}/api/v1/deployments/${DEPLOYMENT_ID}" > "${EVIDENCE}/deployment-view.json"
curl -s -o /dev/null -w "ui:%{http_code}\n" "http://${API_ADDR}/ui/deployments/${DEPLOYMENT_ID}" | tee "${EVIDENCE}/ui-status.txt"
(
  cd "${ROOT}/../frontend"
  ORCHESTRATOR_LIVE_URL="http://${API_ADDR}" ORCHESTRATOR_LIVE_DEPLOYMENT_ID="${DEPLOYMENT_ID}" \
    npx vitest run src/test/live-console.test.tsx
) | tee "${EVIDENCE}/console-test.log"
kill "${API_PID}" 2>/dev/null || true

echo "=== internal happy path passed; run id ${RUN_ID}; evidence in ${EVIDENCE}"
