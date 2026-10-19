#!/usr/bin/env bash
# Truthful, idempotent cleanup of ONE V03 run. It is called by the runner's EXIT trap AND by the independent
# supervisor (v03-supervisor.sh), so it also runs when the runner was SIGKILLed or hit its deadline.
#
#   v03-cleanup.sh CLEANUP_ENV [run|supervisor]
#
# Exit 0 ONLY when the cleanup is physically proven. Order (never reversed):
#   1. stop every owned process group (product and the Terraform it spawned, the scenario browser process, Xvfb)
#      and PROVE no process is left, so nothing can still write while objects are deleted;
#   2. remove exactly the run-owned namespaces: ownership is the scenario's earlier record and/or the product's own
#      receipts plus this run's exact tags and the recorded UID; the delete is atomic on the UID;
#   3. delete the credential objects of the run's exact Internal-account keys in Vault (strict HTTP/JSON handling);
#   4. only then drop the disposable database and roles.
# When anything is not proven, the database, the state key, the recovery artifacts, the credentials and the private
# directory are KEPT (private, 0700) so a retry or an operator can still recover, CLEANUP-NOT-PROVEN.txt says so and
# the exit code is non-zero. Removing the private directory is the runner's job, after its secret scan.
set -uo pipefail
umask 077
ENVFILE="${1:?cleanup environment file}"
MODE="${2:-run}"
# shellcheck disable=SC1090
source "${ENVFILE}"
: "${WORK:?}" "${PRIVATE:?}" "${RUN_ID:?}"
mkdir -p "${PRIVATE}"
exec 9> "${PRIVATE}/cleanup.lock"
flock -x 9
STATE="${WORK}/cleanup-state.txt"
note() { printf '%s\n' "$*" >> "${WORK}/cleanup.txt"; }
if [[ "$(cat "${STATE}" 2>/dev/null || true)" == proven ]]; then exit 0; fi
failed=0
fail() { note "FAILED $*"; failed=1; }
output_inventory_requested=0
output_inventory_ready=1
if [[ -n "${OUTPUT_REFS_FILE:-}" || -n "${OUTPUT_REFS_OWNER_URL_FILE:-}" || -n "${OUTPUT_CLEANUP_TOKEN:-}" ]]; then output_inventory_requested=1; fi

stop_group() { # label file-with-pgid
  local label="$1" file="$2" pgid
  [[ -s "${file}" ]] || return 0
  pgid="$(head -n1 "${file}" | tr -dc '0-9')"
  [[ -n "${pgid}" && "${pgid}" -gt 1 ]] || return 0
  if pgrep -g "${pgid}" > /dev/null 2>&1; then
    kill -TERM -- "-${pgid}" 2> /dev/null || true
    for _ in $(seq 1 80); do pgrep -g "${pgid}" > /dev/null 2>&1 || break; sleep 0.5; done
    if pgrep -g "${pgid}" > /dev/null 2>&1; then kill -KILL -- "-${pgid}" 2> /dev/null || true; sleep 1; fi
  fi
  if pgrep -g "${pgid}" > /dev/null 2>&1; then
    fail "${label}: process group ${pgid} still has live processes; nothing is destroyed while a writer may still run"
    return 1
  fi
  note "${label}: process group ${pgid} stopped, no live process remains"
  : > "${file}"
  return 0
}

group_has_live_process() {
  local pgid="$1"
  ps -eo pgid=,stat= | awk -v group="${pgid}" '$1==group && $2 !~ /Z/ { found=1 } END { exit !found }'
}

stop_auxiliary() { # label pid-file; optional service URL or X display in line 4
  local label="$1" file="$2" pid pgid expected target start_time comm actual_group actual_start
  [[ -s "${file}" ]] || return 0
  pid="$(sed -n '1p' "${file}" | tr -dc '0-9')"
  pgid="$(sed -n '2p' "${file}" | tr -dc '0-9')"
  expected="$(sed -n '3p' "${file}")"
  target="$(sed -n '4p' "${file}")"
  start_time="$(sed -n '5p' "${file}")"
  # Older Xvfb records had only PID and process name; PID must still be the expected process.
  if [[ -z "${expected}" ]]; then expected="$(sed -n '2p' "${file}")"; pgid="${pid}"; target="$(sed -n '3p' "${file}")"; fi
  [[ -n "${pid}" && "${pid}" -gt 1 && -n "${pgid}" && "${pgid}" -gt 1 && -n "${expected}" ]] || { fail "${label}: incomplete process ownership record"; return 1; }
  if [[ "${label}" == "run-owned secret-store Vault" || "${label}" == Xvfb ]] && ! [[ "${start_time}" =~ ^[0-9]+$ ]]; then fail "${label}: exact process start identity is missing"; return 1; fi
  if [[ -e "/proc/${pid}" ]]; then
    comm="$(cat "/proc/${pid}/comm" 2>/dev/null || true)"
    actual_group="$(ps -o pgid= -p "${pid}" 2>/dev/null | tr -d ' ')"
    actual_start="$(sed 's/^.*) //' "/proc/${pid}/stat" 2>/dev/null | awk '{print $20}')"
    if [[ "${comm}" != "${expected}" || "${actual_group}" != "${pgid}" || ( ( "${label}" == "run-owned secret-store Vault" || "${label}" == Xvfb ) && "${actual_start}" != "${start_time}" ) ]]; then fail "${label}: process identity/group differs from the recorded owner"; return 1; fi
    if [[ "${pgid}" == "${pid}" ]]; then kill -TERM -- "-${pgid}" 2>/dev/null || true; else kill -TERM "${pid}" 2>/dev/null || true; fi
    for _ in $(seq 1 80); do group_has_live_process "${pgid}" || break; sleep 0.5; done
    if group_has_live_process "${pgid}"; then
      if [[ "${pgid}" == "${pid}" ]]; then kill -KILL -- "-${pgid}" 2>/dev/null || true; else kill -KILL "${pid}" 2>/dev/null || true; fi
      for _ in $(seq 1 20); do group_has_live_process "${pgid}" || break; sleep 0.5; done
    fi
  fi
  if group_has_live_process "${pgid}"; then fail "${label}: owned process group remains alive"; return 1; fi
  if [[ "${label}" == "run-owned secret-store Vault" ]]; then
    if ! node -e 'const net=require("node:net"),u=new URL(process.argv[1]),s=net.createConnection({host:u.hostname,port:Number(u.port)});s.setTimeout(1500);s.on("connect",()=>{s.destroy();process.exit(3)});s.on("error",e=>process.exit(e.code==="ECONNREFUSED"?0:4));s.on("timeout",()=>{s.destroy();process.exit(4)})' "${target}"; then fail "${label}: service port absence could not be proven"; return 1; fi
  elif [[ "${label}" == Xvfb ]]; then
    local display="${target#:}"
    [[ -z "${target}" || ( ! -e "/tmp/.X${display}-lock" && ! -e "/tmp/.X11-unix/X${display}" ) ]] || { fail "Xvfb: display lock or socket remains"; return 1; }
  fi
  note "${label}: exact process/group stopped and service absence verified"
  : > "${file}"
}

# 1. Quiesce.
stop_group "scenario browser process" "${PRIVATE}/node.pid" || true
stop_group "product server (and the Terraform it spawned)" "${PRIVATE}/server.pid" || true
for aux in "${PRIVATE}"/aux-*.pid; do
  [[ -e "${aux}" ]] || continue
  case "${aux##*/}" in
    aux-secret-vault.pid) stop_auxiliary "run-owned secret-store Vault" "${aux}" || true ;;
    aux-xvfb.pid) stop_auxiliary "Xvfb" "${aux}" || true ;;
    *) fail "unknown auxiliary process record ${aux##*/}" ;;
  esac
done
if (( failed )); then
  note "cleanup stopped before any deletion: a process could not be stopped"
else
  # 2. Cluster.
  if [[ ! -s "${PRIVATE}/baseline.json" ]]; then
    note "no cluster baseline was recorded: nothing was created in the cluster"
  elif [[ ! -x "${CTL}" ]]; then
    fail "the operator tool ${CTL} is missing; the cluster cannot be proven clean"
  else
    owned_flag=()
    [[ -s "${PRIVATE}/owned.json" ]] && owned_flag=(-owned "${PRIVATE}/owned.json")
    "${CTL}" v03-reap -kubectl "${KUBECTL}" -kubeconfig "${HOST_KUBECONFIG}" -context "${HOST_CONTEXT}" -baseline "${PRIVATE}/baseline.json" \
      "${owned_flag[@]}" -run-id "${RUN_ID}" -database-url-file "${PRIVATE}/console.url" -tmp-dir "${PRIVATE}/tmp" -out "${PRIVATE}/reap-result.json" >> "${WORK}/cleanup.txt" 2>&1 \
      && note "cluster: run namespaces proven absent, baseline namespaces unchanged" || fail "cluster cleanup is not proven (see the lines above)"
  fi
  # 3. Vault.
  if [[ ! -e "${PRIVATE}/product-started" ]]; then
    note "the product never started: no credential object was written"
  elif [[ ! -x "${CTL}" ]]; then
    fail "the operator tool ${CTL} is missing; Vault cannot be proven clean"
  else
    if [[ -e "${PRIVATE}/product-started" ]] && (( output_inventory_requested )); then
      if [[ ! -s "${OUTPUT_REFS_OWNER_URL_FILE:-}" || -z "${OUTPUT_REFS_FILE:-}" ]]; then
        output_inventory_ready=0
        fail "the private operator database URL or durable output-reference inventory path is missing; output Vault material, SQL and recovery handles are retained"
      else
        "${CTL}" v04-output-refs -database-owner-url-file "${OUTPUT_REFS_OWNER_URL_FILE}" -run-id "${RUN_ID}" -out "${OUTPUT_REFS_FILE}" >> "${WORK}/cleanup.txt" 2>&1 \
          && note "vault: exact durable output references inventoried from this run's persisted outputs" || { output_inventory_ready=0; fail "durable output secret references could not be inventoried; SQL and recovery handles are retained"; }
      fi
    elif [[ -e "${PRIVATE}/product-started" ]]; then
      note "V04 output inventory is not configured; retaining the legacy cleanup contract"
    fi
    scenario_keys=()
    [[ -s "${PRIVATE}/owned-vault.json" ]] && scenario_keys=(-scenario-keys "${PRIVATE}/owned-vault.json")
    "${CTL}" v03-vault-clean -vault-addr "${VAULT_ADDR_LOCAL}" -mount "${MOUNT}" -token-file "${PRIVATE}/vault-token" -org "${ORG}" \
      -database-url-file "${PRIVATE}/console.url" "${scenario_keys[@]}" >> "${WORK}/cleanup.txt" 2>&1 \
      && note "vault: credential objects of this run's exact account keys deleted and proven absent" || fail "Vault cleanup is not proven (see the lines above)"
  fi
  if [[ -e "${PRIVATE}/product-started" ]] && (( output_inventory_requested )); then
    if (( output_inventory_ready )) && [[ -n "${OUTPUT_BASELINE_FILE:-}" ]]; then
      "${CTL}" v04-output-inventory -mode check -vault-addr "${OUTPUT_VAULT_ADDR:-${VAULT_ADDR_LOCAL}}" -mount "${OUTPUT_MOUNT:-${MOUNT:-secret}}" \
        -token-file "${OUTPUT_CLEANUP_TOKEN}" -baseline-file "${OUTPUT_BASELINE_FILE}" -run-id "${RUN_ID}" -refs-file "${OUTPUT_REFS_FILE}" >> "${WORK}/cleanup.txt" 2>&1 \
        || { output_inventory_ready=0; fail "output metadata completeness/ownership is not proven; keep SQL and recovery"; }
    fi
    if (( ! output_inventory_ready )); then
      fail "output deletion blocked: inventory is incomplete; keep SQL and recovery"
    elif [[ ! -x "${CTL}" || ! -s "${OUTPUT_REFS_FILE:-}" || ! -s "${OUTPUT_CLEANUP_TOKEN:-}" ]]; then
      fail "durable output secret references or their scoped cleanup token are missing; SQL and recovery handles are retained"
    else
      output_guard_args=()
      if [[ -n "${OUTPUT_BASELINE_FILE:-}" ]]; then output_guard_args=(-baseline-file "${OUTPUT_BASELINE_FILE}" -run-id "${RUN_ID}"); fi
      "${CTL}" v04-output-clean -vault-addr "${OUTPUT_VAULT_ADDR:-${VAULT_ADDR_LOCAL}}" -mount "${OUTPUT_MOUNT:-${MOUNT:-secret}}" \
        -token-file "${OUTPUT_CLEANUP_TOKEN}" -refs-file "${OUTPUT_REFS_FILE}" "${output_guard_args[@]}" >> "${WORK}/cleanup.txt" 2>&1 \
        && note "vault: exact durable output references inventoried for this run were deleted and absence verified" || fail "durable output secret cleanup is not proven (see the lines above)"
    fi
  fi
  if [[ -e "${PRIVATE}/product-started" && -n "${OUTPUT_BASELINE_FILE:-}" ]] && (( output_inventory_requested && output_inventory_ready )); then
    "${CTL}" v04-output-inventory -mode verify -vault-addr "${OUTPUT_VAULT_ADDR:-${VAULT_ADDR_LOCAL}}" -mount "${OUTPUT_MOUNT:-${MOUNT:-secret}}" \
      -token-file "${OUTPUT_CLEANUP_TOKEN}" -baseline-file "${OUTPUT_BASELINE_FILE}" -run-id "${RUN_ID}" -refs-file "${OUTPUT_REFS_FILE}" >> "${WORK}/cleanup.txt" 2>&1 \
      || fail "output metadata does not equal protected baseline; keep SQL and recovery"
  fi
  # 4. Database, only after the physical cleanup is confirmed.
  if [[ -s "${PRIVATE}/pg-manifest.json" ]]; then
    if (( failed )); then
      note "the database and roles are KEPT: the physical cleanup is not proven and the product state is needed to recover"
    elif "${CTL}" pg-drop -admin-url-file "${PG_ADMIN}" -manifest "${PRIVATE}/pg-manifest.json" >> "${WORK}/cleanup.txt" 2>&1; then
      note "disposable database and roles dropped"
      : > "${PRIVATE}/pg-manifest.json"
    else
      fail "the disposable database could not be dropped; pg-manifest.json is kept"
    fi
  else
    note "no disposable database was provisioned (or it is already dropped)"
  fi
fi

if (( failed )); then
  printf 'failed\n' > "${STATE}"
  cat > "${WORK}/CLEANUP-NOT-PROVEN.txt" <<TXT
The cleanup of run ${RUN_ID} is NOT proven. Nothing needed to recover was deleted.
Kept (private, 0700): ${PRIVATE} - state key, Terraform recovery artifacts, credentials, database manifest.
Kept: the disposable database and roles (when the cluster or Vault cleanup failed).
See cleanup.txt for the exact steps. Retry with: $0 ${ENVFILE}
TXT
  exit 1
fi
printf 'proven\n' > "${STATE}"
rm -f "${WORK}/CLEANUP-NOT-PROVEN.txt"
note "cleanup proven (${MODE})"
exit 0
