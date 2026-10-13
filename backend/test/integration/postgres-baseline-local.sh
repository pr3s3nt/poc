#!/usr/bin/env bash
# T28 isolated PostgreSQL verification of the fresh persistence baseline:
#   postgres-baseline-local.sh --evidence NEW_PATH [--allow-pull] [--dry-run]
#   postgres-baseline-local.sh --help
#
# What it does (needs explicit approval before the first real run):
#   1. Creates ONE uniquely named and labelled postgres:16-alpine container on a
#      NEW private Docker network. Its data directory is a tmpfs (no volume, no
#      bind mount, no Compose project) and its port is published on 127.0.0.1
#      with a random host port.
#   2. Runs, with a minimal environment that holds only the owned server's URL
#      and REQUIRE_POSTGRES=1 (a missing URL fails instead of skipping):
#        go build ./...
#        go test -count=1 -v ./...                (full uncached suite, PostgreSQL opted in)
#        go test -race -count=1 on the persistence packages (targeted races)
#      and proves that the required PostgreSQL baseline tests ran, not skipped.
#   3. Runs a SYNTHETIC logical backup/restore smoke (pg_dump/pg_restore of a
#      generated table) inside the OWN container. It proves runner tooling only;
#      it is not a backup or restore of any retained IDP/K8S-4F data.
#   4. Always removes ONLY the run-owned container and network (also on error,
#      Ctrl-C and SIGTERM), verifies they are gone and writes cleanup.txt.
#
# What it never touches: .env files, an inherited database URL, retained or
# Compose-managed containers/volumes/networks, Vault, kubeconfig, a cluster or
# any cloud. The only database credential is a random password generated for
# this run; it is redacted from every evidence file.
#
# NEW_PATH must not exist and must be outside the git repository. Evidence
# (logs, per-step exit statuses, cleanup record) is kept also on failure.
# Evidence layout: run.json, steps.tsv, <step>.log, <step>.status, versions.txt,
# git.txt, docker-isolation.json, backup-restore-smoke.txt, cleanup.txt, result.txt.
#
# --allow-pull   permit `docker pull postgres:16-alpine` when the image is absent
#                (default: stop and tell you, so no hidden network access).
# --dry-run      validate arguments and print the exact plan/commands; runs no
#                docker command and creates nothing.
set -euo pipefail
umask 077

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"   # backend/
REPO="$(cd "${ROOT}/.." && pwd)"
IMAGE="postgres:16-alpine"
STEP_TIMEOUT="${T28_STEP_TIMEOUT:-45m}"

usage() {
  cat >&2 <<'EOF'
usage: postgres-baseline-local.sh --evidence NEW_PATH [--allow-pull] [--dry-run]
       postgres-baseline-local.sh --help
EOF
  exit 2
}
help() {
  sed -n '2,/^set -euo/p' "${BASH_SOURCE[0]}" | sed '$d' | sed 's/^# \{0,1\}//'
  exit 0
}

EVIDENCE="" ALLOW_PULL=0 DRY_RUN=0
while (( $# )); do
  case "$1" in
    --help|-h) help ;;
    --evidence) [[ $# -ge 2 ]] || usage; EVIDENCE="$2"; shift 2 ;;
    --allow-pull) ALLOW_PULL=1; shift ;;
    --dry-run) DRY_RUN=1; shift ;;
    *) usage ;;
  esac
done
[[ -n "${EVIDENCE}" ]] || usage

EVIDENCE="$(realpath -m "${EVIDENCE}")"
case "${EVIDENCE}/" in "${REPO}/"*) echo "evidence path must be outside the repository" >&2; exit 2 ;; esac
if [[ -e "${EVIDENCE}" || -L "${EVIDENCE}" ]]; then
  echo "evidence path already exists; choose a new path: ${EVIDENCE}" >&2; exit 2
fi

RUN_ID="poc-t28-pg-$(date -u +%Y%m%dT%H%M%SZ)-$(od -An -N4 -tx1 /dev/urandom | tr -d ' \n')"
CONTAINER_NAME="${RUN_ID}"
NETWORK_NAME="${RUN_ID}-net"
LABELS=(--label "com.poc.run-id=${RUN_ID}" --label "com.poc.owner=postgres-baseline-local" --label "com.poc.task=T28")
RUN_CMD=(docker run -d --name "${CONTAINER_NAME}" --network "${NETWORK_NAME}" "${LABELS[@]}"
  --tmpfs /var/lib/postgresql/data:rw,size=1g --memory 1g --pids-limit 512 --security-opt no-new-privileges
  -p 127.0.0.1::5432 -e POSTGRES_PASSWORD "${IMAGE}"
  -c fsync=off -c synchronous_commit=off -c full_page_writes=off -c max_connections=400)

if (( DRY_RUN )); then
  cat <<EOF
DRY RUN (no docker command is executed, nothing is created)
run id        : ${RUN_ID}
evidence      : ${EVIDENCE} (created 0700, kept also on failure)
image         : ${IMAGE} (docker image inspect; --allow-pull=${ALLOW_PULL})
ownership     : label com.poc.run-id=${RUN_ID}, com.poc.owner=postgres-baseline-local, com.poc.task=T28
network       : docker network create --driver bridge <labels> ${NETWORK_NAME}
container     : ${RUN_CMD[*]}
                (POSTGRES_PASSWORD is a random per-run value exported to the docker client only)
steps         : go build ./... ; go test -count=1 -v ./... ; targeted go test -race ;
                synthetic pg_dump/pg_restore smoke inside the owned container
go environment: env -i with PATH/HOME/Go cache variables plus REQUIRE_POSTGRES=1 and ORCHESTRATOR_POSTGRES_TEST_URL
                (no .env, no inherited DATABASE/ORCHESTRATOR_* variable, no ORCH_KIND_VERIFY, GOFLAGS empty, GOENV=off)
cleanup       : docker rm -f -v <owned container id>; docker network rm <owned network id>;
                both removals are label-checked first and verified afterwards (cleanup.txt)
EOF
  exit 0
fi

for tool in docker go git timeout od realpath sed grep; do
  command -v "${tool}" >/dev/null || { echo "${tool} is required" >&2; exit 1; }
done

# ---- run-owned resources and cleanup -------------------------------------
CONTAINER_ID="" NETWORK_ID="" PASSWORD="" CLEANED=0 FAILED=0 CLEANUP_FAILED=0 STARTED=0

owned() { # owned container|network ID: the object exists and carries this run's label
  local kind="$1" id="$2" label
  if [[ "${kind}" == network ]]; then
    label="$(docker network inspect -f '{{index .Labels "com.poc.run-id"}}' "${id}" 2>/dev/null || true)"
  else
    label="$(docker container inspect -f '{{index .Config.Labels "com.poc.run-id"}}' "${id}" 2>/dev/null || true)"
  fi
  [[ "${label}" == "${RUN_ID}" ]]
}

cleanup() {
  (( CLEANED )) && return 0
  CLEANED=1
  (( STARTED )) || return 0
  set +e
  local out="${EVIDENCE}/cleanup.txt" id ids
  {
    echo "run id: ${RUN_ID}"
    # Containers/networks are found by the recorded id AND by this run's unique
    # label/name, so a resource created by an interrupted or failed command (id
    # never captured) is still removed. Each one is label-checked before removal.
    ids="$( { [[ -n "${CONTAINER_ID}" ]] && echo "${CONTAINER_ID}"
              docker ps -aq --no-trunc --filter "label=com.poc.run-id=${RUN_ID}"
              docker ps -aq --no-trunc --filter "name=^/${CONTAINER_NAME}$"; } 2>/dev/null | sort -u )"
    for id in ${ids}; do
      docker logs "${id}" >"${EVIDENCE}/postgres-container.log" 2>&1
      if owned container "${id}"; then
        docker rm -f -v "${id}" >/dev/null 2>&1 && echo "removed container ${id}" || { echo "FAILED to remove container ${id}"; CLEANUP_FAILED=1; }
      else
        echo "container ${id} is not owned by this run: not touched"
      fi
    done
    ids="$( { [[ -n "${NETWORK_ID}" ]] && echo "${NETWORK_ID}"
              docker network ls -q --no-trunc --filter "label=com.poc.run-id=${RUN_ID}"
              docker network ls -q --no-trunc --filter "name=^${NETWORK_NAME}$"; } 2>/dev/null | sort -u )"
    for id in ${ids}; do
      if owned network "${id}"; then
        docker network rm "${id}" >/dev/null 2>&1 && echo "removed network ${id}" || { echo "FAILED to remove network ${id}"; CLEANUP_FAILED=1; }
      else
        echo "network ${id} is not owned by this run: not touched"
      fi
    done
    echo "verify: containers carrying this run id: $(docker ps -aq --filter "label=com.poc.run-id=${RUN_ID}" | wc -l)"
    echo "verify: networks carrying this run id:   $(docker network ls -q --filter "label=com.poc.run-id=${RUN_ID}" | wc -l)"
    echo "verify: volumes carrying this run id:    $(docker volume ls -q --filter "label=com.poc.run-id=${RUN_ID}" | wc -l)"
    [[ "$(docker ps -aq --filter "label=com.poc.run-id=${RUN_ID}" | wc -l)" == 0 ]] || CLEANUP_FAILED=1
    [[ "$(docker ps -aq --filter "name=^/${CONTAINER_NAME}$" | wc -l)" == 0 ]] || CLEANUP_FAILED=1
    [[ "$(docker network ls -q --filter "label=com.poc.run-id=${RUN_ID}" | wc -l)" == 0 ]] || CLEANUP_FAILED=1
    [[ "$(docker network ls -q --filter "name=^${NETWORK_NAME}$" | wc -l)" == 0 ]] || CLEANUP_FAILED=1
    echo "cleanup failed: ${CLEANUP_FAILED}"
  } >"${out}" 2>&1
  # The per-run password never stays in evidence.
  if [[ -n "${PASSWORD}" && -d "${EVIDENCE}" ]]; then
    grep -rlF -- "${PASSWORD}" "${EVIDENCE}" 2>/dev/null | while read -r f; do
      sed -i "s/${PASSWORD}/[REDACTED]/g" "${f}"
    done
  fi
  cat "${out}"
  set -e
}

on_exit() {
  local status=$?
  cleanup
  if (( CLEANUP_FAILED )) && (( status == 0 )); then status=3; fi
  if [[ -d "${EVIDENCE}" ]]; then
    {
      if (( status == 0 && FAILED == 0 )); then echo "PASS"; else echo "FAIL exit=${status} failed_steps=${FAILED}"; fi
      echo "cleanup_failed=${CLEANUP_FAILED}"
    } >"${EVIDENCE}/result.txt"
  fi
  exit "${status}"
}
trap on_exit EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
trap 'exit 129' HUP

# ---- preflight (read-only) -------------------------------------------------
if ! docker image inspect "${IMAGE}" >/dev/null 2>&1; then
  if (( ALLOW_PULL )); then
    echo "image ${IMAGE} absent: pulling (--allow-pull)"
    docker pull "${IMAGE}"
  else
    echo "image ${IMAGE} is not present locally; pull it yourself or pass --allow-pull" >&2
    trap - EXIT; exit 1
  fi
fi

mkdir -p "$(dirname "${EVIDENCE}")"
mkdir -m 0700 "${EVIDENCE}"
STARTED=1
printf 'step\tstatus\tlog\n' >"${EVIDENCE}/steps.tsv"
{
  echo "run id: ${RUN_ID}"
  docker version --format 'docker client {{.Client.Version}} server {{.Server.Version}}' 2>&1 || true
  echo "image: ${IMAGE} $(docker image inspect -f '{{.Id}}' "${IMAGE}")"
  go version
} >"${EVIDENCE}/versions.txt"
{
  git -C "${REPO}" rev-parse HEAD
  git -C "${REPO}" status --short --branch
} >"${EVIDENCE}/git.txt" 2>&1 || true

# ---- owned network and container -------------------------------------------
NETWORK_ID="$(docker network create --driver bridge "${LABELS[@]}" "${NETWORK_NAME}")"
PASSWORD="$(od -An -N24 -tx1 /dev/urandom | tr -d ' \n')"
export POSTGRES_PASSWORD="${PASSWORD}"
CONTAINER_ID="$("${RUN_CMD[@]}")"
unset POSTGRES_PASSWORD
owned container "${CONTAINER_ID}" || { echo "created container is not labelled for this run" >&2; exit 1; }

PORT_LINE="$(docker port "${CONTAINER_ID}" 5432/tcp | head -n1)"
PORT="${PORT_LINE##*:}"
[[ "${PORT_LINE}" == 127.0.0.1:* && "${PORT}" =~ ^[0-9]+$ ]] || { echo "unexpected published port: ${PORT_LINE}" >&2; exit 1; }

ready=0
for _ in $(seq 1 90); do
  if docker exec "${CONTAINER_ID}" pg_isready -q -h 127.0.0.1 -U postgres \
     && docker exec "${CONTAINER_ID}" psql -U postgres -d postgres -tAqc 'select 1' >/dev/null 2>&1 \
     && (exec 3<>"/dev/tcp/127.0.0.1/${PORT}") 2>/dev/null; then ready=1; break; fi
  sleep 1
done
(( ready )) || { echo "owned PostgreSQL did not become ready" >&2; exit 1; }

# Isolation proof: tmpfs data only, no volume/bind, only the own network, loopback publish.
docker inspect -f '{"name":"{{.Name}}","labels":{{json .Config.Labels}},"mounts":{{json .Mounts}},"hostConfigBinds":{{json .HostConfig.Binds}},"tmpfs":{{json .HostConfig.Tmpfs}},"networks":{{json .NetworkSettings.Networks}},"ports":{{json .NetworkSettings.Ports}}}' \
  "${CONTAINER_ID}" | sed "s/${PASSWORD}/[REDACTED]/g" >"${EVIDENCE}/docker-isolation.json"
MOUNT_TYPES="$(docker inspect -f '{{range .Mounts}}{{.Type}} {{end}}' "${CONTAINER_ID}")"
if [[ "${MOUNT_TYPES}" == *volume* || "${MOUNT_TYPES}" == *bind* ]] \
   || [[ "$(docker inspect -f '{{len .HostConfig.Binds}}' "${CONTAINER_ID}")" != 0 ]] \
   || [[ "$(docker inspect -f '{{json .HostConfig.Tmpfs}}' "${CONTAINER_ID}")" != *'"/var/lib/postgresql/data"'* ]] \
   || [[ "$(docker inspect -f '{{range $k,$v := .NetworkSettings.Networks}}{{$k}} {{end}}' "${CONTAINER_ID}")" != "${NETWORK_NAME} " ]]; then
  echo "container is not isolated (volume, bind or foreign network found); see docker-isolation.json" >&2
  exit 1
fi

TEST_URL="postgres://postgres:${PASSWORD}@127.0.0.1:${PORT}/postgres?sslmode=disable"
{
  echo "{\"runId\":\"${RUN_ID}\",\"container\":\"${CONTAINER_NAME}\",\"containerId\":\"${CONTAINER_ID}\",\"network\":\"${NETWORK_NAME}\",\"networkId\":\"${NETWORK_ID}\","
  echo " \"image\":\"${IMAGE}\",\"publish\":\"127.0.0.1:${PORT}\",\"url\":\"postgres://postgres:[REDACTED]@127.0.0.1:${PORT}/postgres\",\"started\":\"$(date -u +%FT%TZ)\"}"
} >"${EVIDENCE}/run.json"

# ---- steps -------------------------------------------------------------------
# Minimal Go environment: the owned server's URL is the only database setting.
GOENV_NAMES=(PATH HOME TMPDIR LANG GOCACHE GOMODCACHE GOPATH GOPROXY GOSUMDB GONOSUMDB GONOSUMCHECK GOTOOLCHAIN GOROOT)
passthrough=()
for name in "${GOENV_NAMES[@]}"; do
  if [[ -n "${!name:-}" ]]; then passthrough+=("${name}=${!name}"); fi
done
# GOFLAGS is forced empty (no build tags/opt-ins) and GOENV=off ignores the user's go env file.
printf '%s\n' "${GOENV_NAMES[@]}" GOFLAGS GOENV CGO_ENABLED REQUIRE_POSTGRES ORCHESTRATOR_POSTGRES_TEST_URL >"${EVIDENCE}/go-env-names.txt"

step() { # step NAME CMD...   (records the status; never aborts the runner)
  local name="$1"; shift
  local status=0
  echo "== ${name}"
  ( cd "${ROOT}" && timeout "${STEP_TIMEOUT}" "$@" ) >"${EVIDENCE}/${name}.log" 2>&1 || status=$?
  echo "${status}" >"${EVIDENCE}/${name}.status"
  printf '%s\t%s\t%s\n' "${name}" "${status}" "${name}.log" >>"${EVIDENCE}/steps.tsv"
  if (( status != 0 )); then FAILED=$((FAILED + 1)); echo "   FAILED (exit ${status}); log kept in ${name}.log"; else echo "   ok"; fi
}
GO_CMD_ENV=(env -i "${passthrough[@]}" GOFLAGS= GOENV=off CGO_ENABLED=1 REQUIRE_POSTGRES=1 "ORCHESTRATOR_POSTGRES_TEST_URL=${TEST_URL}")

step go-build "${GO_CMD_ENV[@]}" go build ./...
step go-test-full "${GO_CMD_ENV[@]}" go test -count=1 -v ./...

# The suite must have used PostgreSQL: required baseline tests passed, none skipped.
REQUIRED_TESTS=(
  TestFreshInitializationAndIdempotentReopen TestConcurrentFreshInitialization TestFailedInitializationLeavesEmptyDatabase
  TestOldLayoutRequiresResetAndIsNeverEligible TestSchemaDriftIsDetectedWithoutRepair TestBaselineEnforcesCurrentConstraints
  TestReopenNeverReconstructsWorkloadHistory TestPostgresEnvironmentBindingContractSurvivesRestart
  TestNormalizedStoreRestartAndSnapshotConstraints
)
proof_status=0
: >"${EVIDENCE}/postgres-opt-in-proof.txt"
for t in "${REQUIRED_TESTS[@]}"; do
  if grep -q -- "^--- PASS: ${t} " "${EVIDENCE}/go-test-full.log"; then echo "PASS ${t}"; else echo "MISSING/SKIPPED/FAILED ${t}"; proof_status=1; fi
done >>"${EVIDENCE}/postgres-opt-in-proof.txt"
echo "skipped tests in full run: $(grep -c -- '^--- SKIP' "${EVIDENCE}/go-test-full.log" || true)" >>"${EVIDENCE}/postgres-opt-in-proof.txt"
echo "${proof_status}" >"${EVIDENCE}/postgres-opt-in-proof.status"
printf '%s\t%s\t%s\n' postgres-opt-in-proof "${proof_status}" postgres-opt-in-proof.txt >>"${EVIDENCE}/steps.tsv"
(( proof_status == 0 )) || { FAILED=$((FAILED + 1)); echo "== postgres-opt-in-proof FAILED (see postgres-opt-in-proof.txt)"; }

step go-test-race "${GO_CMD_ENV[@]}" go test -race -count=1 \
  ./internal/adapters/postgres/... ./internal/adapters/store/... ./internal/application/catalog/... \
  ./internal/application/deployment/... ./internal/application/preview/... ./internal/application/connection/... \
  ./internal/ports/...

# Synthetic logical backup/restore smoke on the OWN container: tooling proof only.
smoke() {
  local c="${CONTAINER_ID}"
  docker exec "${c}" psql -U postgres -d postgres -v ON_ERROR_STOP=1 -qc 'CREATE DATABASE t28_smoke_src' &&
  docker exec "${c}" psql -U postgres -d t28_smoke_src -v ON_ERROR_STOP=1 -qc "CREATE TABLE synthetic(id int PRIMARY KEY, note text NOT NULL); INSERT INTO synthetic SELECT g, 'synthetic-' || g FROM generate_series(1,500) g" &&
  docker exec "${c}" pg_dump -U postgres -Fc -f /tmp/t28-smoke.dump t28_smoke_src &&
  docker exec "${c}" psql -U postgres -d postgres -v ON_ERROR_STOP=1 -qc 'CREATE DATABASE t28_smoke_restore' &&
  docker exec "${c}" pg_restore -U postgres --exit-on-error -d t28_smoke_restore /tmp/t28-smoke.dump &&
  local a b
  a="$(docker exec "${c}" psql -U postgres -d t28_smoke_src -tAc "SELECT md5(string_agg(id || note, ',' ORDER BY id)) FROM synthetic")" &&
  b="$(docker exec "${c}" psql -U postgres -d t28_smoke_restore -tAc "SELECT md5(string_agg(id || note, ',' ORDER BY id)) FROM synthetic")" &&
  echo "source checksum   ${a}" && echo "restored checksum ${b}" && [[ -n "${a}" && "${a}" == "${b}" ]] &&
  docker exec "${c}" psql -U postgres -d postgres -tAc "SELECT datname FROM pg_database WHERE datname NOT LIKE 'template%' ORDER BY 1" &&
  echo "synthetic smoke only: no retained IDP, K8S-4F, Vault or workload data was read or written"
}
echo "== backup-restore-smoke"
smoke_status=0
smoke >"${EVIDENCE}/backup-restore-smoke.txt" 2>&1 || smoke_status=$?
echo "${smoke_status}" >"${EVIDENCE}/backup-restore-smoke.status"
printf '%s\t%s\t%s\n' backup-restore-smoke "${smoke_status}" backup-restore-smoke.txt >>"${EVIDENCE}/steps.tsv"
(( smoke_status == 0 )) || { FAILED=$((FAILED + 1)); echo "   FAILED (exit ${smoke_status})"; }

echo "== summary (steps.tsv)"
cat "${EVIDENCE}/steps.tsv"
(( FAILED == 0 )) || exit 1
exit 0
