#!/usr/bin/env bash
# Keeps a kubectl port-forward alive for a retained workload Service. Meant to
# run detached (setsid) and be stopped by its owner: kill "$(cat <pid-file>)".
# Stopping the supervisor also stops its own port-forward and sleep children.
#   retained-port-forward.sh <context> <namespace> <service> <local-port> <remote-port> <pid-file>
set -u
[[ $# -eq 6 ]] || { echo "usage: $0 <context> <namespace> <service> <local-port> <remote-port> <pid-file>" >&2; exit 2; }
CONTEXT="$1" NAMESPACE="$2" SERVICE="$3" LOCAL="$4" REMOTE="$5" PIDFILE="$6"
[[ "${CONTEXT}" =~ ^[A-Za-z0-9._:/@-]+$ && "${NAMESPACE}" =~ ^[a-z0-9]([a-z0-9-]*[a-z0-9])?$ && "${SERVICE}" =~ ^[a-z0-9]([a-z0-9-]*[a-z0-9])?$ ]] || { echo "invalid context/namespace/service" >&2; exit 2; }
[[ "${LOCAL}" =~ ^[0-9]+$ && "${REMOTE}" =~ ^[0-9]+$ ]] || { echo "invalid port" >&2; exit 2; }
echo $$ > "${PIDFILE}"
child=""
stop() {
  # Only PIDs this supervisor started are signalled; an empty value is never used.
  if [[ -n "${child}" ]] && kill -0 "${child}" 2>/dev/null; then kill "${child}" 2>/dev/null; wait "${child}" 2>/dev/null; fi
  rm -f "${PIDFILE}"
  exit 0
}
trap stop TERM INT
while :; do
  kubectl --context "${CONTEXT}" -n "${NAMESPACE}" port-forward --address 127.0.0.1 "svc/${SERVICE}" "${LOCAL}:${REMOTE}" &
  child=$!
  wait "${child}"
  child=""
  # sleep runs as a tracked child too, so a stop signal never leaves it behind.
  sleep 3 &
  child=$!
  wait "${child}"
  child=""
done
