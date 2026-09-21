#!/usr/bin/env bash
# Cloud happy path on real AWS, driven through the orchestrator HTTP API.
#
# Flow: preflight identity -> price the run -> create temporary ECR repositories
# -> build and push the acceptance images -> start the orchestrator binary in
# aws mode -> POST three deployments to /api/v1/deployments -> verify cluster,
# job flow and Web Console -> clean up and prove nothing is left.
#
# Cleanup runs from an EXIT trap, so an interrupted run still tears down.
set -uo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
WORK="${WORK_DIR:-$(mktemp -d)}"
RUN_ID="${RUN_ID:-aws-$(date -u +%Y%m%d%H%M%S)}"
TERRAFORM_ROOT="${WORK}/terraform"
EVIDENCE="${WORK}/evidence"
STATE_PATH="${WORK}/state.json"
DEPLOYMENT_ID_FILE="${WORK}/deployment-id"
KUBECONFIG_FILE="${WORK}/kubeconfig-pointer"
NAMESPACE="acceptance-${RUN_ID}"
export TF_PLUGIN_CACHE_DIR="${TF_PLUGIN_CACHE_DIR:-${HOME}/.terraform.d/plugin-cache}"
mkdir -p "${EVIDENCE}" "${TERRAFORM_ROOT}" "${TF_PLUGIN_CACHE_DIR}"

ECR_REPOSITORIES=""
API_PID=""
CLEANUP_DONE=0

cleanup() {
  local status=$?
  if [[ "${CLEANUP_DONE}" == "1" ]]; then
    exit "${status}"
  fi
  CLEANUP_DONE=1
  echo "=== cleanup (exit status ${status})"
  if [[ -n "${API_PID}" ]]; then
    kill "${API_PID}" 2>/dev/null || true
  fi
  local kubeconfig=""
  if [[ -f "${KUBECONFIG_FILE}" ]]; then
    kubeconfig="$(head -1 "${KUBECONFIG_FILE}")"
  fi
  RUN_ID="${RUN_ID}" TERRAFORM_ROOT="${TERRAFORM_ROOT}" AWS_REGION="${REGION}" \
    KUBECONFIG_PATH="${kubeconfig}" NAMESPACE="${NAMESPACE}" \
    ECR_REPOSITORIES="${ECR_REPOSITORIES}" EVIDENCE_DIR="${EVIDENCE}" \
    bash "${ROOT}/test/integration/aws-cleanup.sh"
  local cleanup_status=$?
  echo "=== cleanup exit ${cleanup_status}; evidence in ${EVIDENCE}"
  if [[ "${status}" == "0" ]]; then
    exit "${cleanup_status}"
  fi
  exit "${status}"
}

set -e

# 1. Identity and region, without printing any credential.
REGION="${AWS_REGION:-${AWS_DEFAULT_REGION:-$(aws configure get region)}}"
if [[ -z "${REGION}" ]]; then
  echo "no AWS region configured" >&2
  exit 1
fi
export AWS_REGION="${REGION}" AWS_DEFAULT_REGION="${REGION}"
IDENTITY="$(aws sts get-caller-identity --output json)"
ACCOUNT_ID="$(echo "${IDENTITY}" | sed -n 's/.*"Account": "\([0-9]*\)".*/\1/p')"
PRINCIPAL_ARN="$(echo "${IDENTITY}" | sed -n 's/.*"Arn": "\([^"]*\)".*/\1/p')"
if [[ -z "${ACCOUNT_ID}" ]]; then
  echo "could not resolve the AWS account id" >&2
  exit 1
fi
OWNER_TAG="${OWNER_TAG:-$(basename "${PRINCIPAL_ARN}")}"
EXPIRES_AT="$(date -u -d '+2 hours' +%FT%TZ)"
ECR_REGISTRY="${ACCOUNT_ID}.dkr.ecr.${REGION}.amazonaws.com"
{
  echo "run_id=${RUN_ID}"
  echo "account_id=${ACCOUNT_ID}"
  echo "principal_arn=${PRINCIPAL_ARN}"
  echo "region=${REGION}"
  echo "namespace=${NAMESPACE}"
  echo "expires_at=${EXPIRES_AT}"
  echo "started_at=$(date -u +%FT%TZ)"
} | tee "${EVIDENCE}/preflight.txt"

trap cleanup EXIT

# 2. Price the run and pick the cheapest workable configuration.
echo "=== pricing"
go run ./test/integration/costreport -region "${REGION}" -hours 2 \
  -shell-out "${WORK}/cost.env" -json-out "${EVIDENCE}/cost-estimate.json" | tee "${EVIDENCE}/cost-estimate.md"
# shellcheck disable=SC1090
source "${WORK}/cost.env"
echo "eks_version=${EKS_VERSION:-1.34} node=${NODE_INSTANCE_TYPE}/${NODE_CAPACITY_TYPE} arch=${NODE_ARCH}" \
  | tee -a "${EVIDENCE}/preflight.txt"

# 3. Temporary ECR repositories, tagged with the run id.
echo "=== creating temporary ECR repositories"
for workload in frontend backend worker; do
  repo="orchestrator-verification/acceptance-${workload}-${RUN_ID}"
  aws ecr create-repository --region "${REGION}" --repository-name "${repo}" \
    --image-tag-mutability MUTABLE \
    --tags "Key=project,Value=orchestrator" "Key=owner,Value=${OWNER_TAG}" \
           "Key=environment,Value=dev" "Key=run-id,Value=${RUN_ID}" \
           "Key=expires-at,Value=${EXPIRES_AT}" "Key=managed-by,Value=orchestrator-verification" \
    --query 'repository.repositoryUri' --output text >> "${EVIDENCE}/ecr.txt"
  ECR_REPOSITORIES="${ECR_REPOSITORIES} ${repo}"
done
cat "${EVIDENCE}/ecr.txt"

# 4. Build and push the acceptance images for the node architecture.
echo "=== building and pushing acceptance images"
ARCH="${NODE_ARCH}" "${ROOT}/test/integration/build-images.sh" "${RUN_ID}" "${WORK}/bin"
aws ecr get-login-password --region "${REGION}" | docker login --username AWS --password-stdin "${ECR_REGISTRY}" > /dev/null
for workload in frontend backend worker; do
  repo="orchestrator-verification/acceptance-${workload}-${RUN_ID}"
  image="${ECR_REGISTRY}/${repo}:${RUN_ID}"
  docker tag "acceptance-${workload}:${RUN_ID}" "${image}"
  docker push --quiet "${image}"
  echo "${image}" >> "${EVIDENCE}/images.txt"
done
FRONTEND_IMAGE="${ECR_REGISTRY}/orchestrator-verification/acceptance-frontend-${RUN_ID}:${RUN_ID}"
BACKEND_IMAGE="${ECR_REGISTRY}/orchestrator-verification/acceptance-backend-${RUN_ID}:${RUN_ID}"
WORKER_IMAGE="${ECR_REGISTRY}/orchestrator-verification/acceptance-worker-${RUN_ID}:${RUN_ID}"

# 5. Terraform validation before any apply.
echo "=== terraform validate"
for module in vpc eks aurora; do
  dir="${WORK}/validate/${module}"
  mkdir -p "${dir}"
  cp "${ROOT}/internal/adapters/terraform/modules/${module}/main.tf" "${dir}/"
  (cd "${dir}" && terraform init -backend=false -input=false -no-color > /dev/null && terraform validate -no-color)
done | tee "${EVIDENCE}/terraform-validate.txt"

# 6. Start the orchestrator exactly as an operator would.
echo "=== starting the orchestrator in aws mode"
go build -o "${WORK}/orchestrator" ./cmd/orchestrator
"${WORK}/orchestrator" -addr 127.0.0.1:0 -addr-file "${WORK}/api-addr" -ui-dir frontend/dist \
  -adapters aws -state "${STATE_PATH}" -region "${REGION}" -account-id "${ACCOUNT_ID}" \
  -run-id "${RUN_ID}" -owner "${OWNER_TAG}" -cloud-namespace "${NAMESPACE}" \
  -terraform-root "${TERRAFORM_ROOT}" \
  -frontend-image "${FRONTEND_IMAGE}" -backend-image "${BACKEND_IMAGE}" -worker-image "${WORKER_IMAGE}" \
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
curl -s "http://${API_ADDR}/api/v1/applications" | tee "${EVIDENCE}/applications.json" > /dev/null
echo "api=${API_ADDR}" | tee -a "${EVIDENCE}/preflight.txt"

# 7. Cloud happy path over HTTP: the same request the Deploy page sends.
echo "=== deploying backend, worker and frontend over the HTTP API"
go run ./test/integration/deployctl -api "http://${API_ADDR}" \
  -application acceptance-cloud -environment dev \
  -actor aws-verify -workloads backend,worker,frontend \
  | tee "${EVIDENCE}/deployments.txt"

# 8. Verify the persisted state, the cluster and the job flow.
echo "=== verifying the cloud run"
RUN_ID="${RUN_ID}" STATE_PATH="${STATE_PATH}" NAMESPACE="${NAMESPACE}" \
  DEPLOYMENT_ID_FILE="${DEPLOYMENT_ID_FILE}" KUBECONFIG_FILE="${KUBECONFIG_FILE}" \
  go test -tags integration -count=1 -timeout 40m -v -run TestAWSCloudVerification ./test/integration/ 2>&1 \
  | tee "${EVIDENCE}/go-integration.log"

KUBECONFIG_PATH="$(head -1 "${KUBECONFIG_FILE}")"
DEPLOYMENT_ID="$(cat "${DEPLOYMENT_ID_FILE}")"

echo "=== cluster evidence"
kubectl --kubeconfig "${KUBECONFIG_PATH}" get nodes -o wide | tee "${EVIDENCE}/eks-nodes.txt"
kubectl --kubeconfig "${KUBECONFIG_PATH}" get all -n "${NAMESPACE}" | tee "${EVIDENCE}/eks-resources.txt"
kubectl --kubeconfig "${KUBECONFIG_PATH}" logs -n "${NAMESPACE}" deployment/worker --tail=20 | tee "${EVIDENCE}/worker.log"
aws eks describe-cluster --region "${REGION}" --name "acceptance-cloud-${RUN_ID}" \
  --query 'cluster.{name:name,version:version,status:status}' --output json | tee "${EVIDENCE}/eks-cluster.json"
aws rds describe-db-clusters --region "${REGION}" \
  --query "DBClusters[?contains(DBClusterIdentifier,'${RUN_ID}')].{id:DBClusterIdentifier,engine:EngineVersion,status:Status,serverless:ServerlessV2ScalingConfiguration}" \
  --output json | tee "${EVIDENCE}/aurora.json"

# 9. Web Console against the real cloud deployment.
echo "=== verifying the Web Console"
curl -s "http://${API_ADDR}/api/v1/deployments/${DEPLOYMENT_ID}" > "${EVIDENCE}/deployment-view.json"
curl -s -o /dev/null -w "ui:%{http_code}\n" "http://${API_ADDR}/ui/deployments/${DEPLOYMENT_ID}" | tee "${EVIDENCE}/ui-status.txt"
(
  cd frontend
  ORCHESTRATOR_LIVE_URL="http://${API_ADDR}" ORCHESTRATOR_LIVE_DEPLOYMENT_ID="${DEPLOYMENT_ID}" \
    npx vitest run src/test/live-console.test.tsx
) | tee "${EVIDENCE}/console-test.log"

echo "finished_at=$(date -u +%FT%TZ)" | tee -a "${EVIDENCE}/preflight.txt"
echo "=== cloud happy path passed; run id ${RUN_ID}"
