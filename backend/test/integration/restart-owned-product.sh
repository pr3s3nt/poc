#!/usr/bin/env bash
# Restart only the acceptance product identified by its original private PID,
# executable and handle paths. Preserve its cleared environment and API port.
set -euo pipefail
umask 077
PRIVATE="${1:?usage: restart-owned-product.sh /absolute/run/private}"
[[ "${PRIVATE}" == /* && -d "${PRIVATE}" && ! -L "${PRIVATE}" ]]
PID="$(cat "${PRIVATE}/server.pid")"
[[ "${PID}" =~ ^[0-9]+$ && -r "/proc/${PID}/cmdline" ]]
mapfile -d '' -t ARGV < "/proc/${PID}/cmdline"
mapfile -d '' -t ORIGINAL_ENV < "/proc/${PID}/environ"
[[ "${ARGV[0]}" == "${PRIVATE}/bin/orchestrator" ]]
SERVER_CWD="$(readlink "/proc/${PID}/cwd")"
ADDR_INDEX="" ADDR_FILE="" DB_OK=0 KEY_OK=0
for (( i=1; i<${#ARGV[@]}-1; i++ )); do
  case "${ARGV[i]}" in
    -addr) ADDR_INDEX=$((i+1)) ;;
    -addr-file) ADDR_FILE="${ARGV[i+1]}" ;;
    -database-url-file) [[ "${ARGV[i+1]}" == "${PRIVATE}/console.url" ]] && DB_OK=1 ;;
    -terraform-state-key-file) [[ "${ARGV[i+1]}" == "${PRIVATE}/state.key" ]] && KEY_OK=1 ;;
  esac
done
[[ -n "${ADDR_INDEX}" && -n "${ADDR_FILE}" && "${DB_OK}" == 1 && "${KEY_OK}" == 1 ]]
ADDRESS="$(cat "${ADDR_FILE}")"
[[ "${ADDRESS}" =~ ^127\.0\.0\.1:[0-9]+$ ]]
ARGV[ADDR_INDEX]="${ADDRESS}"
SHA="$(sha256sum "${ARGV[0]}" | cut -d' ' -f1)"
kill -TERM "${PID}"
stopped() { [[ ! -r "/proc/${PID}/stat" ]] || [[ "$(awk '{print $3}' "/proc/${PID}/stat")" == Z ]]; }
for _ in $(seq 1 80); do stopped && break; sleep 0.25; done
stopped || { echo 'owned product did not stop; restart held' >&2; exit 1; }
(cd "${SERVER_CWD}" && exec setsid env -i "${ORIGINAL_ENV[@]}" "${ARGV[@]}") > "${PRIVATE}/restart-server.log" 2>&1 &
NEW_PID=$!
printf '%s\n' "${NEW_PID}" > "${PRIVATE}/server.pid"
for _ in $(seq 1 120); do
  if curl --max-time 2 -fsS "http://${ADDRESS}/api/v1/healthz" >/dev/null 2>&1; then
    [[ -r "/proc/${NEW_PID}/cmdline" && "${NEW_PID}" != "${PID}" ]]
    jq -n --argjson before "${PID}" --argjson after "${NEW_PID}" --arg sha "${SHA}" --arg at "$(date -u +%FT%TZ)" \
      '{beforePid:$before,afterPid:$after,binarySha256:$sha,at:$at,sameOriginalHandles:true,healthPassed:true}' > "${PRIVATE}/product-restart.json"
    echo 'owned product restarted with original handles and API port'
    exit 0
  fi
  kill -0 "${NEW_PID}" 2>/dev/null || break
  sleep 0.25
done
echo 'restarted product did not become healthy; private recovery retained' >&2
exit 1
