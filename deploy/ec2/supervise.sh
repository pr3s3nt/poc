#!/usr/bin/env bash
# Start detached before a trial apply. A successful, retained handoff writes
# retain-approved; failure writes cleanup-requested. Loss of the caller is
# covered by a finite deadline. No product workloads may be created before handoff.
set -euo pipefail
umask 077
[[ $# == 3 && $3 =~ ^[0-9]+$ ]] || { echo 'Usage: supervise.sh CONFIG RUN_DIR DEADLINE_EPOCH' >&2; exit 2; }
config=$(realpath "$1")
run_dir=$(realpath "$2")
deadline=$3
script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
while [[ ! -f $run_dir/retain-approved ]]; do
  if [[ -f $run_dir/cleanup-requested || $(date +%s) -ge $deadline ]]; then
    # Retry while Terraform is releasing its own operation lock. Never break it.
    for attempt in {1..12}; do
      if "$script_dir/manage.sh" destroy "$config" "$run_dir"; then
        touch "$run_dir/terraform-destroy-finished"
        if "$script_dir/absence.sh" "$config" "$run_dir"; then exit 0; fi
        echo 'Terraform finished; native absence remains unproven. Recovery retained.' >&2
        exit 1
      fi
      sleep 30
    done
    echo 'Cleanup requires operator recovery; state retained.' >&2
    exit 1
  fi
  sleep 15
done
echo 'Trial retained for user handoff; automatic failure cleanup disarmed.'
