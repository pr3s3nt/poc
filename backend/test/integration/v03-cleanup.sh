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

# 1. Quiesce.
stop_group "scenario browser process" "${PRIVATE}/node.pid" || true
stop_group "product server (and the Terraform it spawned)" "${PRIVATE}/server.pid" || true
for aux in "${PRIVATE}"/aux-*.pid; do
  [[ -e "${aux}" ]] || continue
  pid="$(tr -dc '0-9' < "${aux}")"
  # Only a process that still is the recorded program may be signalled (never a recycled pid).
  if [[ -n "${pid}" ]] && [[ "$(cat "/proc/${pid}/comm" 2> /dev/null || true)" == "$(sed -n 2p "${aux}")" ]]; then kill -TERM "${pid}" 2> /dev/null || true; fi
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
    scenario_keys=()
    [[ -s "${PRIVATE}/owned-vault.json" ]] && scenario_keys=(-scenario-keys "${PRIVATE}/owned-vault.json")
    "${CTL}" v03-vault-clean -vault-addr "${VAULT_ADDR_LOCAL}" -mount "${MOUNT}" -token-file "${PRIVATE}/vault-token" -org "${ORG}" \
      -database-url-file "${PRIVATE}/console.url" "${scenario_keys[@]}" >> "${WORK}/cleanup.txt" 2>&1 \
      && note "vault: credential objects of this run's exact account keys deleted and proven absent" || fail "Vault cleanup is not proven (see the lines above)"
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
