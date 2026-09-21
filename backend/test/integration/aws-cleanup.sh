#!/usr/bin/env bash
# Cloud cleanup for one verification run. It never uses a wildcard: every target
# comes from the Terraform state of this run or from the run id tag.
#
# Required: RUN_ID, TERRAFORM_ROOT, AWS_REGION.
# Optional: KUBECONFIG_PATH, NAMESPACE, ECR_REPOSITORIES (space separated).
set -uo pipefail

RUN_ID="${RUN_ID:?RUN_ID is required}"
TERRAFORM_ROOT="${TERRAFORM_ROOT:?TERRAFORM_ROOT is required}"
REGION="${AWS_REGION:?AWS_REGION is required}"
EVIDENCE="${EVIDENCE_DIR:-${TERRAFORM_ROOT}/evidence}"
mkdir -p "${EVIDENCE}"
REPORT="${EVIDENCE}/cleanup-report.txt"
: > "${REPORT}"

say() { echo "$*" | tee -a "${REPORT}"; }

say "=== cleanup for run ${RUN_ID} in ${REGION} at $(date -u +%FT%TZ)"

# 1. Kubernetes workloads and any Service that could create a cloud resource.
if [[ -n "${KUBECONFIG_PATH:-}" && -f "${KUBECONFIG_PATH}" && -n "${NAMESPACE:-}" ]]; then
  say "--- deleting Kubernetes namespace ${NAMESPACE}"
  kubectl --kubeconfig "${KUBECONFIG_PATH}" get svc -A -o jsonpath='{range .items[?(@.spec.type=="LoadBalancer")]}{.metadata.namespace}/{.metadata.name}{"\n"}{end}' \
    >> "${REPORT}" 2>/dev/null
  kubectl --kubeconfig "${KUBECONFIG_PATH}" delete namespace "${NAMESPACE}" --ignore-not-found --timeout=5m >> "${REPORT}" 2>&1
fi

# 2. Temporary ECR repositories.
for repo in ${ECR_REPOSITORIES:-}; do
  say "--- deleting ECR repository ${repo}"
  aws ecr delete-repository --region "${REGION}" --repository-name "${repo}" --force >> "${REPORT}" 2>&1
done

# 3. Terraform destroy in reverse creation order, using this run's state only.
MANIFEST="${TERRAFORM_ROOT}/workspaces.txt"
if [[ -f "${MANIFEST}" ]]; then
  mapfile -t WORKSPACES < <(tac "${MANIFEST}")
  for entry in "${WORKSPACES[@]}"; do
    [[ -z "${entry}" ]] && continue
    module="${entry%% *}"
    dir="${entry#* }"
    [[ -d "${dir}" ]] || continue
    say "--- terraform destroy ${module} (${dir})"
    for attempt in 1 2 3; do
      if (cd "${dir}" && TF_IN_AUTOMATION=1 AWS_REGION="${REGION}" terraform destroy -auto-approve -input=false -no-color \
            -var-file=terraform.tfvars.json >> "${REPORT}" 2>&1); then
        say "    destroy ok (${module})"
        break
      fi
      say "    destroy attempt ${attempt} failed for ${module}; retrying"
      sleep $((attempt * 30))
    done
  done
else
  say "--- no Terraform workspace manifest at ${MANIFEST}"
fi

# 4. Verify with the AWS API; terraform exit code alone is not evidence.
say "=== verification queries for run ${RUN_ID}"
LEFTOVER=0

check() {
  local label="$1"; shift
  local output
  output="$("$@" 2>&1)"
  if [[ -n "${output}" && "${output}" != "None" && "${output}" != "[]" ]]; then
    say "LEFTOVER ${label}: ${output}"
    LEFTOVER=1
  else
    say "clean ${label}"
  fi
}

TAGF="Name=tag:run-id,Values=${RUN_ID}"
check "vpc" aws ec2 describe-vpcs --region "${REGION}" --filters "${TAGF}" --query 'Vpcs[].VpcId' --output text
check "subnets" aws ec2 describe-subnets --region "${REGION}" --filters "${TAGF}" --query 'Subnets[].SubnetId' --output text
check "security-groups" aws ec2 describe-security-groups --region "${REGION}" --filters "${TAGF}" --query 'SecurityGroups[].GroupId' --output text
check "nat-gateways" aws ec2 describe-nat-gateways --region "${REGION}" --filter "${TAGF}" --query 'NatGateways[?State!=`deleted`].NatGatewayId' --output text
check "internet-gateways" aws ec2 describe-internet-gateways --region "${REGION}" --filters "${TAGF}" --query 'InternetGateways[].InternetGatewayId' --output text
check "ec2-instances" aws ec2 describe-instances --region "${REGION}" --filters "${TAGF}" "Name=instance-state-name,Values=pending,running,stopping,stopped" --query 'Reservations[].Instances[].InstanceId' --output text
check "ebs-volumes" aws ec2 describe-volumes --region "${REGION}" --filters "${TAGF}" --query 'Volumes[].VolumeId' --output text
check "elastic-ips" aws ec2 describe-addresses --region "${REGION}" --filters "${TAGF}" --query 'Addresses[].AllocationId' --output text
check "route-tables" aws ec2 describe-route-tables --region "${REGION}" --filters "${TAGF}" --query 'RouteTables[].RouteTableId' --output text

check "eks-clusters" aws eks list-clusters --region "${REGION}" --query "clusters[?contains(@, '${RUN_ID}')]" --output text
check "rds-clusters" aws rds describe-db-clusters --region "${REGION}" --query "DBClusters[?contains(DBClusterIdentifier, '${RUN_ID}')].DBClusterIdentifier" --output text
check "rds-instances" aws rds describe-db-instances --region "${REGION}" --query "DBInstances[?contains(DBInstanceIdentifier, '${RUN_ID}')].DBInstanceIdentifier" --output text
check "db-subnet-groups" aws rds describe-db-subnet-groups --region "${REGION}" --query "DBSubnetGroups[?contains(DBSubnetGroupName, '${RUN_ID}')].DBSubnetGroupName" --output text
check "ecr-repositories" aws ecr describe-repositories --region "${REGION}" --query "repositories[?contains(repositoryName, '${RUN_ID}')].repositoryName" --output text
check "load-balancers-v2" aws elbv2 describe-load-balancers --region "${REGION}" --query 'LoadBalancers[].LoadBalancerName' --output text
check "load-balancers-classic" aws elb describe-load-balancers --region "${REGION}" --query 'LoadBalancerDescriptions[].LoadBalancerName' --output text
check "iam-roles" aws iam list-roles --query "Roles[?contains(RoleName, '${RUN_ID}')].RoleName" --output text

if [[ "${LEFTOVER}" == "1" ]]; then
  say "=== CLEANUP INCOMPLETE for run ${RUN_ID}; see ${REPORT}"
  exit 2
fi
say "=== cleanup verified: no resource carries run id ${RUN_ID}"
