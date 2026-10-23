#!/usr/bin/env bash
# Operator-side entry point. Run directory contains sensitive state and keys.
set -euo pipefail
umask 077
operation=${1:-}
[[ $# == 3 && $operation =~ ^(plan|apply|status|destroy|tunnel)$ ]] || {
  echo 'Usage: manage.sh {plan|apply|status|destroy|tunnel} CONFIG_JSON PRIVATE_RUN_DIR' >&2; exit 2;
}
repo_root=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)
config=$(realpath "$2")
mkdir -p "$3"
run_dir=$(realpath "$3")
[[ $run_dir != "$repo_root" && $run_dir != "$repo_root/"* && $run_dir != / ]] || { echo 'Run directory must be outside the repository.' >&2; exit 2; }
chmod 0700 "$run_dir"
for tool in aws terraform jq ssh-keygen; do command -v "$tool" >/dev/null; done
expected_account=$(jq -er .expected_account_id "$config")
region=$(jq -er .region "$config")
name=$(jq -er .name "$config")
actual_account=$(aws sts get-caller-identity --query Account --output text --region "$region")
[[ $actual_account == "$expected_account" ]] || { echo 'AWS account mismatch.' >&2; exit 1; }
export AWS_REGION=$region AWS_DEFAULT_REGION=$region AWS_PAGER=''
export TF_DATA_DIR="$run_dir/.terraform" TF_IN_AUTOMATION=1
module="$repo_root/deploy/ec2/terraform"
tf() { terraform -chdir="$module" "$@"; }
if [[ -f $run_dir/deployment-name ]]; then
  [[ $(cat "$run_dir/deployment-name") == "$name" ]] || { echo 'Run directory belongs to a different deployment.' >&2; exit 1; }
else
  printf '%s\n' "$name" > "$run_dir/deployment-name"
fi
exec 9>"$run_dir/operation.lock"
flock -n 9 || { echo 'Another operation holds this deployment lock.' >&2; exit 1; }
tf init -input=false -lockfile=readonly -backend-config="path=$run_dir/terraform.tfstate" > "$run_dir/init.log" 2>&1
ownership() {
  local id tag
  id=$(jq -er .instance_id.value "$run_dir/outputs.json")
  tag=$(aws ec2 describe-instances --instance-ids "$id" --query 'Reservations[0].Instances[0].Tags[?Key==`run-id`].Value | [0]' --output text)
  [[ $tag == "$name" ]] || { echo 'Instance ownership check failed; retaining state.' >&2; return 1; }
}
case "$operation" in
  plan)
    [[ -f $run_dir/operator-key ]] || ssh-keygen -q -t ed25519 -N '' -C "$name" -f "$run_dir/operator-key"
    jq -n --arg key "$(cat "$run_dir/operator-key.pub")" '{ssh_public_key:$key}' > "$run_dir/key.tfvars.json"
    tf plan -input=false -var-file="$config" -var-file="$run_dir/key.tfvars.json" -out="$run_dir/deployment.tfplan" > "$run_dir/plan.log" 2>&1
    tf show -json "$run_dir/deployment.tfplan" > "$run_dir/plan.json"
    # Every managed mutation is in this module's narrow resource allowlist.
    jq -e '[.resource_changes[]? | select(.mode=="managed") | .type] | all(. == "aws_instance" or . == "aws_key_pair" or . == "aws_security_group" or . == "aws_iam_role" or . == "aws_iam_role_policy" or . == "aws_iam_instance_profile" or . == "aws_lb" or . == "aws_lb_target_group" or . == "aws_lb_target_group_attachment" or . == "aws_lb_listener")' "$run_dir/plan.json" >/dev/null
    (sha256sum "$config" "$run_dir/key.tfvars.json" "$run_dir/deployment.tfplan") > "$run_dir/plan-inputs.sha256"
    jq -r '.resource_changes[]? | select(.mode=="managed") | "\(.address): \(.change.actions|join(","))"' "$run_dir/plan.json"
    echo "Plan saved privately in $run_dir. Review plan.log before apply."
    ;;
  apply)
    # Only fresh trials get automatic failure cleanup. Retained deployments
    # require deliberate recovery to avoid destroying user-created data.
    if [[ ! -f $run_dir/retain-approved ]]; then
      trap 'rc=$?; if (( rc != 0 )); then touch "$run_dir/cleanup-requested"; fi' EXIT
      if [[ ! -f $run_dir/supervisor.pid ]] || ! kill -0 "$(cat "$run_dir/supervisor.pid")" 2>/dev/null; then
        deadline=$(( $(date +%s) + 7200 ))
        printf '%s\n' "$deadline" > "$run_dir/bootstrap-deadline"
        nohup setsid "$repo_root/deploy/ec2/supervise.sh" "$config" "$run_dir" "$deadline" 9>&- </dev/null > "$run_dir/supervisor.log" 2>&1 &
        echo $! > "$run_dir/supervisor.pid"
      fi
    fi
    (sha256sum -c "$run_dir/plan-inputs.sha256" >/dev/null)
    tf apply -input=false -auto-approve "$run_dir/deployment.tfplan" > "$run_dir/apply.log" 2>&1
    tf output -json > "$run_dir/outputs.json"
    ownership
    ip=$(jq -er .public_ip.value "$run_dir/outputs.json")
    deadline=$(( $(date +%s) + 5700 ))
    echo 'Waiting for EC2 bootstrap and application health...'
    while true; do
      result=$(ssh -o ConnectTimeout=10 -o BatchMode=yes -o StrictHostKeyChecking=accept-new -o "UserKnownHostsFile=$run_dir/known_hosts" -i "$run_dir/operator-key" "ec2-user@$ip" 'if sudo test -f /var/lib/orchestrator-bootstrap-ready; then echo READY; elif sudo test -f /var/lib/orchestrator-bootstrap-failed; then echo FAILED; else echo WAIT; fi' 2>>"$run_dir/ssh.log" || true)
      if [[ $result == FAILED ]]; then
        # Preserve diagnostics before the failure trap lets the supervisor
        # terminate this run-owned host. Never print the private log.
        ssh -o ConnectTimeout=10 -o BatchMode=yes -o "UserKnownHostsFile=$run_dir/known_hosts" -i "$run_dir/operator-key" "ec2-user@$ip" 'sudo journalctl -u orchestrator --no-pager -n 200; sudo tail -n 80 /var/log/orchestrator-bootstrap.log; sudo docker logs --tail 80 orchestrator-ec2-postgres-1; sudo docker logs --tail 80 orchestrator-ec2-backend-1' > "$run_dir/bootstrap-failure.log" 2>&1 || true
        echo 'EC2 bootstrap failed; diagnostics saved privately before cleanup.' >&2
        exit 1
      fi
      if [[ $result == READY ]]; then break; fi
      (( $(date +%s) < deadline )) || { echo 'EC2 bootstrap timed out.' >&2; exit 1; }
      sleep 15
    done
    ssh -o BatchMode=yes -o "UserKnownHostsFile=$run_dir/known_hosts" -i "$run_dir/operator-key" "ec2-user@$ip" 'sudo /opt/orchestrator/current/deploy/ec2/host.sh health' > "$run_dir/health.log" 2>&1
    jq '{instance:.instance_id.value, console:.console_url.value, role:.role_arn.value}' "$run_dir/outputs.json"
    ;;
  status)
    tf output -json > "$run_dir/outputs.json"
    ownership
    jq '{instance:.instance_id.value, ip:.public_ip.value, console:.console_url.value, role:.role_arn.value}' "$run_dir/outputs.json"
    aws ec2 describe-instance-status --instance-ids "$(jq -r .instance_id.value "$run_dir/outputs.json")" --query 'InstanceStatuses[].{State:InstanceState.Name,System:SystemStatus.Status,Instance:InstanceStatus.Status}' --output json
    ;;
  tunnel)
    ownership
    ip=$(jq -er .public_ip.value "$run_dir/outputs.json")
    echo 'Open http://localhost:3001/ui/ while this tunnel is running.'
    exec ssh -o IdentitiesOnly=yes -o StrictHostKeyChecking=accept-new -o "UserKnownHostsFile=$run_dir/known_hosts" -i "$run_dir/operator-key" -N -L 3001:127.0.0.1:3001 "ec2-user@$ip"
    ;;
  destroy)
    # Partial applies may lack outputs. Verify every instance recorded in state
    # by exact ID before destroy; do not use tag/prefix deletion searches.
    tf show -json > "$run_dir/pre-destroy-state.json"
    while IFS= read -r id; do
      tag=$(aws ec2 describe-instances --instance-ids "$id" --query 'Reservations[0].Instances[0].Tags[?Key==`run-id`].Value | [0]' --output text)
      [[ $tag == "$name" ]] || { echo 'Ownership check failed; no destroy.' >&2; exit 1; }
    done < <(jq -r '.values.root_module.resources[]? | select(.type=="aws_instance") | .values.id' "$run_dir/pre-destroy-state.json")
    tf destroy -input=false -auto-approve -var-file="$config" -var-file="$run_dir/key.tfvars.json" > "$run_dir/destroy.log" 2>&1
    echo "Terraform destroy finished. Verify native absence using $run_dir/pre-destroy-state.json before closing cleanup."
    ;;
esac
