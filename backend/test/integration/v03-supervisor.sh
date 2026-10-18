#!/usr/bin/env bash
# Independent owner of one V03 run's cleanup. Started detached (own session) by the runner, it survives the runner's
# death (SIGKILL included): when the runner is gone, or its deadline passed, it runs the same cleanup script.
#
#   v03-supervisor.sh CLEANUP_ENV RUNNER_PID DEADLINE_EPOCH
set -uo pipefail
umask 077
ENVFILE="$1" PARENT="$2" DEADLINE="$3"
# shellcheck disable=SC1090
source "${ENVFILE}"
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
note() { printf '%s\n' "$*" >> "${WORK}/cleanup.txt"; }
while kill -0 "${PARENT}" 2> /dev/null; do
  if (( $(date +%s) >= DEADLINE )); then
    note "supervisor: the run exceeded its deadline; asking the runner to stop"
    kill -TERM "${PARENT}" 2> /dev/null || true
    for _ in $(seq 1 120); do kill -0 "${PARENT}" 2> /dev/null || break; sleep 0.5; done
    kill -KILL "${PARENT}" 2> /dev/null || true
    break
  fi
  sleep 2
done
sleep 2
if [[ "$(cat "${WORK}/cleanup-state.txt" 2> /dev/null || true)" != proven ]]; then
  note "supervisor: the runner is gone before a proven cleanup; running the cleanup"
  "${HERE}/v03-cleanup.sh" "${ENVFILE}" supervisor && note "supervisor: cleanup proven" || note "supervisor: cleanup NOT proven; recovery material is kept"
fi
