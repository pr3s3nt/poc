#!/usr/bin/env bash
# I04-09 isolated verification of the acceptance app Redis cache:
#   acceptance-redis-local.sh [--evidence NEW_PATH] [--package PATTERN] [--help]
#
# Starts ONE disposable PostgreSQL (initdb in a private 0700 directory, scram
# password auth, random loopback port, random password kept in a 0600 file) and
# runs the acceptance-app Go tests against it with REQUIRE_POSTGRES=1 and
# REQUIRE_REDIS=1, so a missing PostgreSQL or redis-server FAILS instead of
# skipping. The tests themselves start and stop their own auth-enabled (and TLS)
# redis-server instances; every directory/process is labelled with this run's
# unique ID (ACCEPTANCE_FIXTURE_RUN_ID).
#
# Cleanup (EXIT/INT/TERM): stops only the PostgreSQL it started (pid file it
# wrote), kills any leftover redis-server whose config lives under this run's
# label, removes the private directory, then VERIFIES absence (processes, port,
# directory) into cleanup.txt. Nothing retained, shared or remote is touched;
# no .env, kubeconfig, cluster or cloud credential is read.
#
# Evidence (kept also on failure, outside the repository, owner-only):
#   go-test.log, summary.txt (PASS/FAIL/SKIP totals), cleanup.txt, run.json
# The run fails when any test fails, any test is skipped, or cleanup cannot be
# verified. When started as root the PostgreSQL server runs as user "postgres".
#
# Environment: REQUIRE_TERRAFORM is passed through when set (with --package ./... this
# runs the whole backend suite); PG_BIN (default: newest /usr/lib/postgresql/*/bin or PATH).
set -euo pipefail
umask 077

BACKEND="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
REPO="$(cd "${BACKEND}/.." && pwd)"
PACKAGE="./examples/acceptance-app/..."
EVIDENCE=""

usage() { echo "usage: acceptance-redis-local.sh [--evidence NEW_PATH] [--package PATTERN] [--help]" >&2; exit 2; }
help() { sed -n '2,/^set -euo/p' "${BASH_SOURCE[0]}" | sed '$d' | sed 's/^# \{0,1\}//'; exit 0; }
while (( $# )); do
  case "$1" in
    --help|-h) help ;;
    --evidence) [[ $# -ge 2 ]] || usage; EVIDENCE="$2"; shift 2 ;;
    --package) [[ $# -ge 2 ]] || usage; PACKAGE="$2"; shift 2 ;;
    *) usage ;;
  esac
done

RUN_ID="acc-redis-$(date -u +%Y%m%dT%H%M%SZ)-$(od -An -N4 -tx1 /dev/urandom | tr -d ' \n')"
TMP_ROOT="${TMPDIR:-/tmp}"
EVIDENCE="$(realpath -m "${EVIDENCE:-${TMP_ROOT}/${RUN_ID}-evidence}")"
case "${EVIDENCE}/" in "${REPO}/"*) echo "evidence path must be outside the repository" >&2; exit 2 ;; esac
[[ ! -e "${EVIDENCE}" && ! -L "${EVIDENCE}" ]] || { echo "evidence path exists; choose a new path: ${EVIDENCE}" >&2; exit 2; }

for tool in go od realpath grep sed awk; do
  command -v "${tool}" >/dev/null || { echo "${tool} is required" >&2; exit 1; }
done
if [[ -z "${PG_BIN:-}" ]]; then
  PG_BIN="$(ls -d /usr/lib/postgresql/*/bin 2>/dev/null | sort -V | tail -1 || true)"
  [[ -n "${PG_BIN}" ]] || { PG_BIN="$(dirname "$(command -v initdb || true)")"; }
fi
for tool in initdb pg_ctl; do
  [[ -x "${PG_BIN}/${tool}" ]] || { echo "${tool} not found (set PG_BIN)" >&2; exit 1; }
done
command -v redis-server >/dev/null || { echo "redis-server is required (tests fail without it)" >&2; exit 1; }

RUN_AS=()
if (( EUID == 0 )); then
  command -v runuser >/dev/null || { echo "runuser is required when running as root" >&2; exit 1; }
  RUN_AS=(runuser -u postgres --)
fi

WORK="$(mktemp -d "${TMP_ROOT}/${RUN_ID}.XXXXXX")"
chmod 700 "${WORK}"
mkdir -p "${EVIDENCE}"; chmod 700 "${EVIDENCE}"
PGDATA="${WORK}/pgdata"
PASSFILE="${WORK}/pg.pass"
PG_STARTED=0 CLEANED=0 FAILED=0 PORT=""

port_open() { (exec 3<>"/dev/tcp/127.0.0.1/$1") 2>/dev/null; }

cleanup() {
  (( CLEANED )) && return 0
  CLEANED=1
  set +e
  local out="${EVIDENCE}/cleanup.txt" bad=0 pid
  {
    echo "run id: ${RUN_ID}"
    if (( PG_STARTED )); then
      "${RUN_AS[@]}" "${PG_BIN}/pg_ctl" -D "${PGDATA}" -m immediate -w stop >/dev/null 2>&1 && echo "stopped owned postgres" || echo "pg_ctl stop reported an error (checked below)"
    fi
    # Leftover redis-server processes started by the tests of THIS run only.
    for pid in $(pgrep -f "/${RUN_ID}[^ ]*/redis.conf|acc-redis-${RUN_ID}" 2>/dev/null || true); do
      [[ "${pid}" == "$$" ]] && continue
      kill -9 "${pid}" 2>/dev/null && echo "killed leftover process ${pid}"
    done
    rm -rf "${WORK}"
    sleep 0.3
    if pgrep -f "${WORK}" >/dev/null 2>&1; then echo "ABSENCE FAILED: process still references ${WORK}"; bad=1; else echo "verified: no process references the run directory"; fi
    if pgrep -f "acc-redis-${RUN_ID}" >/dev/null 2>&1; then echo "ABSENCE FAILED: redis-server of this run still running"; bad=1; else echo "verified: no redis-server of this run"; fi
    if [[ -n "${PORT}" ]]; then
      if port_open "${PORT}"; then echo "ABSENCE FAILED: port ${PORT} still accepts connections"; bad=1; else echo "verified: port ${PORT} closed"; fi
    fi
    if compgen -G "${TMP_ROOT}/acc-redis-${RUN_ID}*" >/dev/null; then echo "ABSENCE FAILED: temporary directories remain"; bad=1; else echo "verified: no temporary directory remains"; fi
    [[ ! -e "${WORK}" ]] && echo "verified: ${WORK} removed" || { echo "ABSENCE FAILED: ${WORK} exists"; bad=1; }
    echo "cleanup-ok: $(( bad == 0 ))"
  } >"${out}" 2>&1
  (( bad == 0 )) || FAILED=1
  return 0
}
on_exit() { local rc=$?; cleanup; (( FAILED )) && rc=1; exit "${rc}"; }
trap on_exit EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

# ---- disposable PostgreSQL --------------------------------------------------
(( EUID == 0 )) && chown postgres "${WORK}"
head -c 24 /dev/urandom | od -An -tx1 | tr -d ' \n' >"${PASSFILE}"
chmod 600 "${PASSFILE}"
(( EUID == 0 )) && chown postgres "${PASSFILE}"
for _ in $(seq 1 50); do
  PORT=$(( 20000 + RANDOM % 40000 ))
  port_open "${PORT}" || break
done
"${RUN_AS[@]}" "${PG_BIN}/initdb" -D "${PGDATA}" -U acceptance_admin -A scram-sha-256 --pwfile="${PASSFILE}" -E UTF8 >"${EVIDENCE}/initdb.log" 2>&1
PG_STARTED=1
"${RUN_AS[@]}" "${PG_BIN}/pg_ctl" -D "${PGDATA}" -w -l "${WORK}/postgres.log" \
  -o "-c listen_addresses=127.0.0.1 -c port=${PORT} -c unix_socket_directories=${WORK} -c cluster_name=${RUN_ID} -c fsync=off -c max_connections=200" start >"${EVIDENCE}/pg_ctl.log" 2>&1

PGPASS_VALUE="$(cat "${PASSFILE}")"
TEST_URL="postgres://acceptance_admin:${PGPASS_VALUE}@127.0.0.1:${PORT}/postgres?sslmode=disable"

cat >"${EVIDENCE}/run.json" <<JSON
{"runId":"${RUN_ID}","postgresPort":${PORT},"package":"${PACKAGE}","startedAt":"$(date -u +%FT%TZ)","redis":"started per test by the Go fixtures"}
JSON

# ---- tests ------------------------------------------------------------------
set +e
(
  cd "${BACKEND}"
  env ORCHESTRATOR_POSTGRES_TEST_URL="${TEST_URL}" REQUIRE_POSTGRES=1 REQUIRE_REDIS=1 \
      ${REQUIRE_TERRAFORM:+REQUIRE_TERRAFORM="${REQUIRE_TERRAFORM}"} \
      ACCEPTANCE_FIXTURE_RUN_ID="${RUN_ID}" go test -count=1 -v "${PACKAGE}"
) 2>&1 | sed "s/${PGPASS_VALUE}/<redacted>/g" >"${EVIDENCE}/go-test.log"
RC=${PIPESTATUS[0]}
set -e

PASS=$(grep -c '^\s*--- PASS' "${EVIDENCE}/go-test.log" || true)
FAIL=$(grep -c '^\s*--- FAIL' "${EVIDENCE}/go-test.log" || true)
SKIP=$(grep -c '^\s*--- SKIP' "${EVIDENCE}/go-test.log" || true)
{
  echo "go test exit status: ${RC}"
  echo "PASS: ${PASS}"
  echo "FAIL: ${FAIL}"
  echo "SKIP: ${SKIP}"
} >"${EVIDENCE}/summary.txt"
cat "${EVIDENCE}/summary.txt"
if (( RC != 0 || FAIL != 0 || SKIP != 0 || PASS == 0 )); then FAILED=1; fi
cleanup
cat "${EVIDENCE}/cleanup.txt"
echo "evidence: ${EVIDENCE}"
(( FAILED == 0 )) || { echo "RESULT: FAILED" >&2; exit 1; }
echo "RESULT: OK"
