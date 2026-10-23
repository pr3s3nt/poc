#!/usr/bin/env bash
# Native absence proof after Terraform destroy, from the exact saved state IDs.
set -euo pipefail
umask 077
[[ $# == 2 ]] || { echo 'Usage: absence.sh CONFIG_JSON PRIVATE_RUN_DIR' >&2; exit 2; }
config=$(realpath "$1")
run_dir=$(realpath "$2")
state="$run_dir/pre-destroy-state.json"
[[ -s $state ]] || { echo 'Original resource manifest is missing.' >&2; exit 1; }
region=$(jq -er .region "$config")
expected=$(jq -er .expected_account_id "$config")
[[ $(aws sts get-caller-identity --region "$region" --query Account --output text) == "$expected" ]]
export AWS_REGION=$region AWS_DEFAULT_REGION=$region AWS_PAGER=''
proof="$run_dir/native-absence.log"
: > "$proof"
# Only explicit NotFound codes prove absence; access/network errors fail closed.
absent() {
  local expected_code=$1
  shift
  if "$@" > "$run_dir/native-response.json" 2> "$run_dir/native-error.log"; then
    echo 'Resource still exists; recovery retained.' >&2
    return 1
  fi
  if ! grep -Eq "\($expected_code\)" "$run_dir/native-error.log"; then
    echo 'Native absence could not be proven; inspect private native-error.log.' >&2
    return 1
  fi
}
# EC2 keeps terminated-instance tombstones temporarily; volumes must disappear.
while IFS= read -r id; do
  if aws ec2 describe-instances --instance-ids "$id" --query 'Reservations[].Instances[].State.Name' --output json > "$run_dir/native-response.json" 2> "$run_dir/native-error.log"; then
    jq -e 'length > 0 and all(. == "terminated")' "$run_dir/native-response.json" >/dev/null
  else
    grep -Eq '\(InvalidInstanceID.NotFound\)' "$run_dir/native-error.log"
  fi
  echo "PASS instance $id terminated/absent" >> "$proof"
done < <(jq -r '.values.root_module.resources[]? | select(.type=="aws_instance") | .values.id' "$state")
while IFS= read -r id; do
  absent InvalidVolume.NotFound aws ec2 describe-volumes --volume-ids "$id"
  echo "PASS volume $id absent" >> "$proof"
done < <(jq -r '.values.root_module.resources[]? | select(.type=="aws_instance") | .values.root_block_device[]?.volume_id // empty' "$state")
while IFS=$'\t' read -r type id name role; do
  case "$type" in
    aws_lb) absent LoadBalancerNotFound aws elbv2 describe-load-balancers --load-balancer-arns "$id" ;;
    aws_lb_listener) absent ListenerNotFound aws elbv2 describe-listeners --listener-arns "$id" ;;
    aws_lb_target_group) absent TargetGroupNotFound aws elbv2 describe-target-groups --target-group-arns "$id" ;;
    aws_security_group) absent InvalidGroup.NotFound aws ec2 describe-security-groups --group-ids "$id" ;;
    aws_key_pair) absent InvalidKeyPair.NotFound aws ec2 describe-key-pairs --key-names "$id" ;;
    aws_iam_role) absent NoSuchEntity aws iam get-role --role-name "$id" ;;
    aws_iam_instance_profile) absent NoSuchEntity aws iam get-instance-profile --instance-profile-name "$id" ;;
    aws_iam_role_policy) absent NoSuchEntity aws iam get-role-policy --role-name "$role" --policy-name "$name" ;;
    *) continue ;;
  esac
  echo "PASS $type $id absent" >> "$proof"
done < <(jq -r '.values.root_module.resources[]? | select(.mode=="managed") | [.type,.values.id,(.values.name // "-"),(.values.role // "-")] | @tsv' "$state")
touch "$run_dir/native-absence-proven"
echo 'Native EC2/storage/ALB/network/IAM absence proven for saved run IDs.'
