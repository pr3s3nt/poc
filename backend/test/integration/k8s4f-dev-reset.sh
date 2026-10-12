#!/usr/bin/env bash
# T29 reviewed backup / IDP reset / K8S-4F rebuild wrapper for ONE fixed run.
#   k8s4f-dev-reset.sh [--dry-run]                 print the exact plan; contacts nothing (default)
#   k8s4f-dev-reset.sh --apply   --run-id <ID>     live backup, reset, UI rebuild, final checks
#   k8s4f-dev-reset.sh --recover --run-id <ID>     separate manual recovery; never automatic
#   k8s4f-dev-reset.sh --continue --run-id <ID>    ONE scoped UI resume after a failed runner (stage runner-started); then final checks
#   k8s4f-dev-reset.sh --inventory                 read-only kubectl get: prints PVC UID/PV to pin in this file
# Writing this file is NOT authorization: --apply/--recover run only after the
# user confirms this run. <ID> must equal the reviewed run ID below.
#
# Data decision (user, 2026-10-12): retain both old namespaces and PVC, old
# frontend/backend at zero replicas, old PostgreSQL at one replica, new workload
# DB empty, backups retained 30 days (record only; never auto-deleted).
# Never used: docker compose down -v, .env (Compose runs with --env-file /dev/null),
# namespace/PVC/dump deletion, Vault reset/token refresh/bootstrap, cluster or operator
# rebuild, kubeconfig on stdout. PLANNED writes (part of the approval manifest): the UI
# registers the Store credential privately in the IDP Vault; the new app's Secrets and
# Deploy create scoped workload-Vault delivery policy/roles/bundles for the new app only.
# Failure keeps evidence and backups and does NOT restore; run --recover by hand.
set -euo pipefail
umask 077

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"   # backend/
REPO="$(cd "${ROOT}/.." && pwd)"

# ---- Fixed reviewed run ------------------------------------------------------
RUN_ID="t29-20261012-r1"
APP_NAME="k8s4f-t29-20261012-r1"
FORWARD_PORT=18481
IMAGE_TAG="k8s4f-dev"   # already loaded acceptance-{backend,frontend}:k8s4f-dev (metadata checked only)
PRIVATE="${HOME}/.local/share/poc-T29/${RUN_ID}"
EVIDENCE="/tmp/poc-T29-live-20261012-r1"
UI_EVIDENCE="${EVIDENCE}/ui"
WORKLOAD_STATE="${HOME}/.local/share/poc-k8s-4f-vault"

CTX="kind-k8s-4f"
PG="orchestrator-local-postgres-1"
IDP_BACKEND="orchestrator-local-backend-1"
IDP_FRONTEND="orchestrator-local-frontend-1"
PG_VOLUME="orchestrator-local_postgres-data"
PG_IMAGE="postgres:16-alpine"
EXPECT_BACKEND_ID="sha256:61b2fed3dc59ff9425199bce2439025ec151462e51c811d41a9cc0f8fc103571"
EXPECT_FRONTEND_ID="sha256:de449a00db689693084656eeb9cd2f121cc7cf31dfb75eaef28516fa6a2763b9"
REC_BACKEND="orchestrator-backend:t29-recovery-r1"
REC_FRONTEND="orchestrator-frontend:t29-recovery-r1"
NEW_BACKEND="orchestrator-backend:${RUN_ID}"
NEW_FRONTEND="orchestrator-frontend:${RUN_ID}"
HIST_COMMIT="a0ef186"
NS_FINAL="app-3c5e4c33-b724-4032-80b1-d270e5c32a35-staging"
NS_EARLIER="app-a43bb834-d2d7-42cd-a035-41841155b5f7-staging"
APP_ID_FINAL="3c5e4c33-b724-4032-80b1-d270e5c32a35"
APP_ID_EARLIER="a43bb834-d2d7-42cd-a035-41841155b5f7"
PV_FINAL_PIN="pvc-d7cb93b1-e930-478c-9907-0b586b5ceb80"
PV_EARLIER_PIN="pvc-f7b74c53-97b3-4c7f-9288-93741bb8ed2d"
# PVC UIDs pinned from the reviewed read-only --inventory output.
PVC_UID_FINAL="d7cb93b1-e930-478c-9907-0b586b5ceb80"
PVC_UID_EARLIER="f7b74c53-97b3-4c7f-9288-93741bb8ed2d"
NODE="k8s-4f-control-plane"
DBSTS="modules-backend-externals-db"
DBPOD="${DBSTS}-0"
PVC="data-${DBSTS}-0"
SMOKE="poc-t29-${RUN_ID}-smoke"
SMOKE_LABEL="poc.t29.run=${RUN_ID}"
IDP_PUBLIC_PORT=18480   # retained old frontend forward; never touched, must not serve after quiesce
BACKUP_DAYS=30

# Public-table row counts, no values.
COUNT_SQL="SELECT table_name||'='||(xpath('/row/c/text()',query_to_xml(format('select count(*) as c from %I.%I',table_schema,table_name),false,true,'')))[1]::text FROM information_schema.tables WHERE table_schema='public' AND table_type='BASE TABLE' ORDER BY 1"
JOBS_SQL="SELECT coalesce(md5(string_agg(id||':'||payload||':'||status||':'||coalesce(result,'')||':'||created_at::text,'|' ORDER BY id)),'empty') FROM jobs"

# Any inherited setting that could retarget Docker/Compose/kubectl/the runner is refused
# (names only are shown, never values). The live runner then gets a clean whitelisted env.
FORBIDDEN_PREFIX='^(K8S4F_|ORCH_|ORCHESTRATOR_|WORKLOAD_VAULT_|COMPOSE_|DOCKER_|KUBECONFIG$|KUBERNETES_|KUBECTL_|BACKEND_|FRONTEND_|POSTGRES_IMAGE$|VAULT_|PG[A-Z]*$|T29_)'

usage() { sed -n '2,10p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'; }
die() { echo "error: $*" >&2; exit 2; }

# ---- Argument parsing --------------------------------------------------------
MODE="dry-run" GIVEN_RUN=""
while (( $# )); do
  case "$1" in
    --dry-run) MODE="dry-run" ;;
    --apply) MODE="apply" ;;
    --recover) MODE="recover" ;;
    --inventory) MODE="inventory" ;;
    --continue) MODE="continue" ;;
    --run-id) shift; [[ $# -gt 0 ]] || die "--run-id needs a value"; GIVEN_RUN="$1" ;;
    -h|--help) usage; exit 0 ;;
    *) die "unsupported argument: $1" ;;
  esac
  shift
done
if [[ "${MODE}" == inventory ]]; then
  for ns in "${NS_FINAL}" "${NS_EARLIER}"; do
    echo "${ns} pvc uid/pv: $(kubectl --context "${CTX}" -n "${ns}" get pvc "${PVC}" -o jsonpath='{.metadata.uid} {.spec.volumeName}')"
  done
  exit 0
fi
if [[ "${MODE}" != dry-run ]]; then
  [[ "${GIVEN_RUN}" == "${RUN_ID}" ]] || die "--${MODE} needs --run-id ${RUN_ID} (reviewed run only)"
elif [[ -n "${GIVEN_RUN}" && "${GIVEN_RUN}" != "${RUN_ID}" ]]; then
  die "unsupported run id ${GIVEN_RUN}; the reviewed run is ${RUN_ID}"
fi
[[ "${HOME}" == /home/thanhnt1 ]] || die "unsupported HOME ${HOME}; paths are reviewed for /home/thanhnt1"
[[ "${PRIVATE}" == /home/thanhnt1/.local/share/poc-T29/t29-20261012-r1 && "${EVIDENCE}" == /tmp/poc-T29-live-20261012-r1 ]] || die "unsupported path"
for p in "${PRIVATE}" "${EVIDENCE}"; do case "${p}/" in "${REPO}/"*) die "private and evidence paths must be outside the repository" ;; esac; done
while IFS= read -r v; do [[ ! "${v}" =~ ${FORBIDDEN_PREFIX} ]] || die "inherited ${v} is set; unset it (value not shown)"; done < <(compgen -e)

# ---- Dry-run: print the exact plan, contact nothing -------------------------
plan() {
cat <<EOF
T29 plan (DRY-RUN: no docker, kubectl, network, file or directory is touched)
run id            ${RUN_ID}   app/subdomain ${APP_NAME}   port ${FORWARD_PORT}
private dir       ${PRIVATE}  (0700, files 0600; never in Git/video/stdout)
evidence dir      ${EVIDENCE} (new, 0700); UI evidence ${UI_EVIDENCE} (runner creates it, one call)
retained apps     ${NS_FINAL} / ${NS_EARLIER}  (namespaces, PVC ${PVC}, PostgreSQL pods stay; FE/BE -> 0)
backups           idp.dump final.dump earlier.dump + .sha256; retention ${BACKUP_DAYS} days, record only, no auto deletion
Compose           docker compose --env-file /dev/null -p orchestrator-local -f docker-compose.yml -f deploy/local/compose.kind.yml

Stage 0 preflight (read-only; any drift aborts before any change)
  - inherited env names with prefixes K8S4F_ ORCH_ ORCHESTRATOR_ WORKLOAD_VAULT_ COMPOSE_ DOCKER_ KUBECONFIG KUBERNETES_ KUBECTL_ BACKEND_ FRONTEND_ VAULT_ PG* T29_ rejected (values never shown); private/evidence paths absent; port ${FORWARD_PORT} and ${WORKLOAD_STATE}/token usable
  - docker inspect ${IDP_BACKEND} image == ${EXPECT_BACKEND_ID}
  - docker inspect ${IDP_FRONTEND} image == ${EXPECT_FRONTEND_ID}
  - ${PG}: image ${PG_IMAGE}, PG16, volume ${PG_VOLUME}, DB/user orchestrator; no ACTIVE/INTERRUPTED/RECOVERING environment_operations
  - ${CTX} node exactly ${NODE}; VSO rollout; workload Vault healthy; PVC UID/PV pinned in this file
  - IDP read-only snapshot: both apps staging bound k8s-4f-2/k8s-4f-vault, generation 0, no active claim; compared again after quiesce
  - for both namespaces: deployments exactly backend,frontend (1 replica, label app.kubernetes.io/name=<name>), statefulset exactly ${DBSTS} (1),
    pod ${DBPOD} PG16 DB/user acceptance, PVC ${PVC} Bound; no extra controllers/pods/PVCs, DB/user acceptance; new app id not in {${APP_ID_FINAL}, ${APP_ID_EARLIER}}
  - loaded images acceptance-{backend,frontend}:${IMAGE_TAG} present in kind-k8s-4f node (metadata only); ${PG_IMAGE} present locally; no pull
Stage 1 prepare (no downtime)
  - mkdir -m 700 ${PRIVATE}; stage marker; docker image tag <pinned ids> ${REC_BACKEND} ${REC_FRONTEND}
  - git show ${HIST_COMMIT}:docker-compose.yml and :deploy/local/compose.kind.yml -> ${PRIVATE}/recovery/; recovery override (pinned image, pull_policy never)
  - BACKEND_IMAGE=${NEW_BACKEND} FRONTEND_IMAGE=${NEW_FRONTEND} compose build backend frontend   (builds only; starts nothing)
  - kubectl config view --flatten --raw --minify --context ${CTX} | server -> https://k8s-4f-control-plane:6443 > ${PRIVATE}/k8s-4f.kubeconfig (0600; no stdout)
Stage 2 quiesce (downtime starts)
  - docker stop ${IDP_BACKEND} ${IDP_FRONTEND}
  - per namespace: kubectl scale deployment/backend deployment/frontend --replicas=0; wait --for=delete pod by label; PostgreSQL stays 1; re-assert no active operation
Stage 3 backup + restore smoke (stop before dropdb on any failure)
  - docker exec ${PG} pg_dump -U orchestrator -d orchestrator -Fc > ${PRIVATE}/idp.dump
  - kubectl -n <ns> exec ${DBPOD} -- pg_dump -U acceptance -d acceptance -Fc > ${PRIVATE}/{final,earlier}.dump
  - non-empty, chmod 0600, sha256sum; source public-table counts and jobs checksum recorded privately
  - docker create --pull never --name ${SMOKE} --label ${SMOKE_LABEL} --network none --tmpfs /var/lib/postgresql/data (no ports/binds/volumes) ${PG_IMAGE}
  - per dump: pg_restore --list; pg_restore --exit-on-error --no-owner --no-acl into its own empty DB; restored counts/jobs checksum == source
  - docker rm -f -v ${SMOKE} (label-owned only; also on interrupt/failure, including interrupted creation)
Stage 4 reset IDP DB only (preserves postgres/Vault volumes, no down -v)
  - docker exec ${PG} dropdb -U orchestrator --force orchestrator ; createdb -U orchestrator -O orchestrator orchestrator
  - BACKEND_IMAGE=${NEW_BACKEND} FRONTEND_IMAGE=${NEW_FRONTEND} compose up -d --no-build --no-deps --pull never --wait backend frontend
  - new IDP DB holds only the seeded acceptance catalog (1 application key acceptance, Connections aws-account/internal-cluster, one empty seed DeploymentSet bound to the seed Environment, no deployments, no retained IDs)
Planned Vault writes (no Vault reset, token refresh or bootstrap): UI registers the Store credential privately in the IDP Vault; new app Secrets + Deploy create scoped
  workload-Vault delivery policy/roles/bundles for the new app only. Cluster/VSO/operators and old app Vault data are not rebuilt or changed.
Stage 5 live UI runner (invoked ONCE, never replayed)
  - env -i (PATH HOME USER LANG TERM only) K8S4F_RUN_ID=${RUN_ID} K8S4F_IMAGE_TAG=${IMAGE_TAG} K8S4F_APPLICATION=${APP_NAME} K8S4F_SUBDOMAIN=${APP_NAME} K8S4F_FORWARD_PORT=${FORWARD_PORT}
    K8S4F_KUBECONFIG_FILE=${PRIVATE}/k8s-4f.kubeconfig K8S4F_FORBIDDEN_APP_IDS=${APP_ID_FINAL},${APP_ID_EARLIER} K8S4F_EXPECT_EMPTY_JOBS=1
    bash backend/test/integration/k8s4f-dev-playwright.sh ${UI_EVIDENCE}
  - UI: register Connection + ordinary Store, create app, save PostgreSQL/BE/FE, Preview, Deploy, four diagnostics, first job, reload
Stage 6 final checks
  - docker restart ${IDP_BACKEND}; wait healthy; /api/v1/healthz; IDP applications exactly {Acceptance Application (seed), ${APP_NAME}} identical before and after restart, no retained ID; new app workloads/PVC Ready
  - both old namespaces: backend/frontend replicas 0, no pods, no endpoints; ${DBSTS} 1/1; PVC ${PVC} same PV; port ${IDP_PUBLIC_PORT} serves nothing
  - retention.txt is written right after backups (Stage 3); result written to ${EVIDENCE}/result.txt
Continue (--continue --run-id ${RUN_ID}; single use, only after a failed first runner that created no app): guards (stage runner-started, ui/result FAIL and no ui/run.json, IDP = seed +
  Connection k8s-4f only, stores = platform-vault only, old apps 0/PVC pins, no new app namespace, port free), marker ${PRIVATE}/ui-evidence=ui-resume1, then the runner once with
  K8S4F_RESUME_CONNECTION_ONLY=1 K8S4F_RESUME_CONNECTION_KEY=k8s-4f -> ${EVIDENCE}/ui-resume1, then Stage 6 final checks
Recovery (--recover --run-id ${RUN_ID}; explicit, never automatic; stage marker decides)
  before reset: stage prepared = no-op; later stages start only stopped IDP containers and scale to 1 only deployments now at 0
  after reset : scale newly created run app FE/BE to 0 only if ${UI_EVIDENCE}/run.json appName+appId+namespace validate (wait pods gone, PVC kept); docker stop IDP FE/BE;
                verify idp.dump checksum; dropdb/createdb; pg_restore --exit-on-error --no-owner --no-acl < idp.dump;
                compose (snapshot ${HIST_COMMIT} + pinned override) up -d --no-deps --no-build --pull never --wait backend frontend; scale old FE/BE to 1
                no workload data restore into the new app; nothing deleted
EOF
}
if [[ "${MODE}" == dry-run ]]; then plan; exit 0; fi

# ---- Live helpers (apply/recover only) --------------------------------------
STAGE_FILE="${PRIVATE}/stage"
STEP_LOG="${EVIDENCE}/steps.log"
log() { local line; line="$(date -u +%FT%TZ) $*"; echo "${line}"; [[ -d "${EVIDENCE}" ]] && echo "${line}" >> "${STEP_LOG}" || true; }
stage() { echo "$1" > "${STAGE_FILE}"; log "stage=$1"; }
dc_rec() { docker compose --env-file /dev/null -p orchestrator-local --project-directory "${REPO}" \
  -f "${PRIVATE}/recovery/docker-compose.yml" -f "${PRIVATE}/recovery/compose.kind.yml" -f "${PRIVATE}/recovery/override.yml" "$@"; }
kc() { kubectl --context "${CTX}" "$@"; }
q_idp() { docker exec "${PG}" psql -U orchestrator -d orchestrator -tAq -v ON_ERROR_STOP=1 -c "$1"; }
q_ns() { kc -n "$1" exec "${DBPOD}" -- psql -U acceptance -d acceptance -tAq -v ON_ERROR_STOP=1 -c "$2"; }
q_smoke() { docker exec "${SMOKE}" psql -U smoke -d "$1" -tAq -v ON_ERROR_STOP=1 -c "$2"; }
for t in docker kubectl git sha256sum curl ss; do command -v "$t" >/dev/null || die "missing tool: $t"; done

smoke_cleanup() {
  # Remove only containers whose label proves ownership by this run (never by name alone);
  # residue is a failure, not suppressed.
  local id rc=0
  local list; list="$(docker ps -aq --filter "label=${SMOKE_LABEL}")" || { log "ERROR cannot list smoke containers"; return 1; }
  for id in ${list}; do
    [[ "$(docker inspect -f '{{index .Config.Labels "poc.t29.run"}}' "${id}")" == "${RUN_ID}" ]] || { log "refusing to remove ${id}: label mismatch"; rc=1; continue; }
    docker rm -f -v "${id}" >/dev/null || rc=1
  done
  list="$(docker ps -aq --filter "label=${SMOKE_LABEL}")" || { log "ERROR cannot verify smoke cleanup"; return 1; }
  if [[ -n "${list}" ]]; then log "ERROR owned smoke container residue remains"; rc=1; else log "smoke container absent"; fi
  return "${rc}"
}
fail_note() {
  local status=$?
  if (( status != 0 )); then
    if [[ "${MODE}" == apply && -f "${PRIVATE}/retention.txt" ]]; then echo "failure at $(date -u +%FT%TZ); dump files present: $(cd "${PRIVATE}" && ls ./*.dump 2>/dev/null | tr '\n' ' ')" >> "${PRIVATE}/retention.txt" || true; fi
    log "FAILED status=${status} stage=$(cat "${STAGE_FILE}" 2>/dev/null || echo none); backups/evidence kept; NO automatic recovery. Review, then run --recover manually if needed"
  fi
  if [[ "${MODE}" == apply ]]; then smoke_cleanup || { (( status != 0 )) || status=1; }; fi
  exit "${status}"
}
ns_check() { # $1 namespace $2 pinned PV $3 pinned PVC UID: exact inventory, abort on drift
  local ns="$1" names
  names="$(kc -n "${ns}" get deploy -o jsonpath='{range .items[*]}{.metadata.name}={.spec.replicas}{"\n"}{end}' | sort | tr '\n' ' ')"
  [[ "${names}" == "backend=1 frontend=1 " ]] || { echo "unexpected deployments in ${ns}: ${names}" >&2; return 1; }
  names="$(kc -n "${ns}" get sts -o jsonpath='{range .items[*]}{.metadata.name}={.spec.replicas}{"\n"}{end}' | sort | tr '\n' ' ')"
  [[ "${names}" == "${DBSTS}=1 " ]] || { echo "unexpected statefulsets in ${ns}: ${names}" >&2; return 1; }
  # Selectors/labels and no extra controllers or pods.
  [[ "$(kc -n "${ns}" get deploy backend -o jsonpath='{.spec.selector.matchLabels.app\.kubernetes\.io/name}')" == backend ]]
  [[ "$(kc -n "${ns}" get deploy frontend -o jsonpath='{.spec.selector.matchLabels.app\.kubernetes\.io/name}')" == frontend ]]
  is_empty kc -n "${ns}" get ds,job,cronjob,hpa -o name || { echo "unexpected controllers (or read failure) in ${ns}" >&2; return 1; }
  names="$(kc -n "${ns}" get pods -o name)" || return 1
  [[ -z "$(grep -v -e '^pod/backend-' -e '^pod/frontend-' -e "^pod/${DBPOD}$" <<<"${names}" || true)" ]] || { echo "unexpected pods in ${ns}" >&2; return 1; }
  [[ "$(kc -n "${ns}" get pvc -o name)" == "persistentvolumeclaim/${PVC}" ]] || { echo "unexpected PVCs in ${ns}" >&2; return 1; }
  [[ "$(kc -n "${ns}" get pvc "${PVC}" -o jsonpath='{.status.phase}')" == Bound ]]
  [[ "$(pv_of "${ns}")" == "$2" ]] || { echo "PV drift in ${ns}" >&2; return 1; }
  [[ "$(uid_of "${ns}")" == "$3" ]] || { echo "PVC UID drift in ${ns}" >&2; return 1; }
  [[ "$(q_ns "${ns}" "select current_user||'/'||current_database()")" == acceptance/acceptance ]]
  [[ "$(q_ns "${ns}" "show server_version_num" | cut -c1-2)" == 16 ]]
}
pv_of() { kc -n "$1" get pvc "${PVC}" -o jsonpath='{.spec.volumeName}'; }
uid_of() { kc -n "$1" get pvc "${PVC}" -o jsonpath='{.metadata.uid}'; }
SNAP_SQL="SELECT a.id||'|'||e.environment_key||'|'||coalesce(c.connection_key,'')||'|'||coalesce(s.store_key,'')||'|'||e.target_generation||'|'||coalesce(e.current_deployment_set_id::text,'')||'|'||(SELECT count(*) FROM deployment_sets d WHERE d.environment_id=e.id)||'|'||(e.active_operation_id IS NULL)||'|'||e.infrastructure_scope||'|'||e.runtime_status FROM environments e JOIN applications a ON a.id=e.application_id LEFT JOIN connections c ON c.id=e.connection_id LEFT JOIN secret_store_connections s ON s.id=e.secret_store_id WHERE a.id IN ('${APP_ID_FINAL}','${APP_ID_EARLIER}') ORDER BY 1"
is_empty() { local o; o="$("$@")" || return 2; [[ -z "$o" ]]; }   # a failed read is an error, never "empty"
tag_id() { docker image inspect -f '{{.Id}}' "$1"; }
no_serving_endpoints() { # $1 namespace $2 service: any EndpointSlice endpoint not explicitly ready=false fails
  local o; o="$(kc -n "$1" get endpointslices -l "kubernetes.io/service-name=$2" -o jsonpath='{range .items[*].endpoints[*]}ready={.conditions.ready}{"\n"}{end}')" || return 2
  [[ -z "$(grep -vx 'ready=false' <<<"$o" | grep -v '^$' || true)" ]]
}
restore_old() { # scale old FE/BE to 1 only from 0; any other value is drift
  local ns n cur
  for ns in "${NS_FINAL}" "${NS_EARLIER}"; do for n in backend frontend; do
    cur="$(kc -n "${ns}" get deploy "${n}" -o jsonpath='{.spec.replicas}')" || return 1
    case "${cur}" in
      0) log "scale ${ns}/${n} 0 -> 1"; kc -n "${ns}" scale "deployment/${n}" --replicas=1 ;;
      1) ;;
      *) echo "replica drift ${ns}/${n}=${cur}; refusing" >&2; return 1 ;;
    esac
    kc -n "${ns}" rollout status "deployment/${n}" --timeout=180s
  done; done
}
wait_pods_gone() { local ns="$1" n; for n in backend frontend; do
  kc -n "${ns}" wait --for=delete pod -l "app.kubernetes.io/name=${n}" --timeout=120s >/dev/null 2>&1 || true
  is_empty kc -n "${ns}" get pod -l "app.kubernetes.io/name=${n}" -o name || return 1; done; }
wait_healthy() { local c="$1" i; for i in $(seq 60); do [[ "$(docker inspect -f '{{.State.Health.Status}}' "${c}")" == healthy ]] && return 0; sleep 2; done; return 1; }
active_ops() { q_idp "SELECT count(*) FROM environment_operations WHERE status IN ('ACTIVE','INTERRUPTED','RECOVERING')"; }
old_apps_state() { # $1 = expected FE/BE replicas
  local ns n
  for ns in "${NS_FINAL}" "${NS_EARLIER}"; do
    for n in backend frontend; do
      [[ "$(kc -n "${ns}" get deploy "${n}" -o jsonpath='{.spec.replicas}')" == "$1" ]]
      if [[ "$1" == 0 ]]; then
        is_empty kc -n "${ns}" get pod -l "app.kubernetes.io/name=${n}" -o name
        no_serving_endpoints "${ns}" "${n}"
      fi
    done
    [[ "$(kc -n "${ns}" get sts "${DBSTS}" -o jsonpath='{.status.readyReplicas}/{.spec.replicas}')" == 1/1 ]]
    [[ "$(kc -n "${ns}" get pvc "${PVC}" -o jsonpath='{.status.phase}')" == Bound ]]
  done
}
scale_old_zero() { local ns; for ns in "${NS_FINAL}" "${NS_EARLIER}"; do kc -n "${ns}" scale deployment/backend deployment/frontend --replicas=0; done
  for ns in "${NS_FINAL}" "${NS_EARLIER}"; do wait_pods_gone "${ns}"; done; }
image_id() { docker inspect -f '{{.Image}}' "$1"; }

resolve_ui() { # recorded UI evidence subdir: owner-only marker restricted to two fixed names
  local m="${PRIVATE}/ui-evidence" v
  [[ -e "${m}" ]] || return 0
  [[ -f "${m}" && ! -L "${m}" && "$(stat -c %a "${m}")" == 600 ]] || die "unsafe ui-evidence marker"
  v="$(cat "${m}")"
  case "${v}" in ui|ui-resume1) UI_EVIDENCE="${EVIDENCE}/${v}" ;; *) die "unsupported ui-evidence marker value" ;; esac
}
final_checks() {
  resolve_ui
NEWNS="$(sed -n 's/.*"namespace": *"\(app-[0-9a-f-]*-staging\)".*/\1/p' "${UI_EVIDENCE}/run.json")"
[[ -n "${NEWNS}" && "${NEWNS}" != "${NS_FINAL}" && "${NEWNS}" != "${NS_EARLIER}" ]]
APPS_SQL="SELECT count(*)||':'||string_agg(name,',' ORDER BY name) FROM applications"
APPS_BEFORE="$(q_idp "${APPS_SQL}")"
docker restart "${IDP_BACKEND}" >/dev/null
wait_healthy "${IDP_BACKEND}"
curl -fsS "http://127.0.0.1:3001/api/v1/healthz" >/dev/null
[[ "$(q_idp "${APPS_SQL}")" == "${APPS_BEFORE}" && "${APPS_BEFORE}" == "2:Acceptance Application,${APP_NAME}" ]]
[[ "$(q_idp "SELECT count(*) FROM applications WHERE id IN ('${APP_ID_FINAL}','${APP_ID_EARLIER}')")" == 0 ]]
for n in backend frontend; do kc -n "${NEWNS}" rollout status "deployment/${n}" --timeout=120s >/dev/null; done
kc -n "${NEWNS}" rollout status "statefulset/${DBSTS}" --timeout=120s >/dev/null
[[ "$(kc -n "${NEWNS}" get pvc -o jsonpath='{.items[*].status.phase}')" == Bound ]]
old_apps_state 0
[[ "$(pv_of "${NS_FINAL}")" == "${PV_FINAL_PIN}" && "$(pv_of "${NS_EARLIER}")" == "${PV_EARLIER_PIN}" ]]
! curl -fsS -m 3 "http://127.0.0.1:${IDP_PUBLIC_PORT}/" >/dev/null 2>&1 || { echo "old endpoint ${IDP_PUBLIC_PORT} serves" >&2; exit 1; }
  [[ "$(uid_of "${NS_FINAL}")" == "${PVC_UID_FINAL}" && "$(uid_of "${NS_EARLIER}")" == "${PVC_UID_EARLIER}" ]]
}

# ---- Recover ------------------------------------------------------------------
if [[ "${MODE}" == recover ]]; then
  [[ -f "${STAGE_FILE}" && ! -L "${STAGE_FILE}" ]] || die "no stage marker at ${STAGE_FILE}; nothing was mutated by this run"
  [[ -d "${EVIDENCE}" ]] || { mkdir -m 700 "${EVIDENCE}"; }
  trap fail_note EXIT
  S="$(cat "${STAGE_FILE}")"
  resolve_ui
  log "recover start; recorded stage=${S}"
  case "${S}" in
    prepared)
      log "stage prepared: nothing was stopped or scaled; recovery is a no-op"
      echo "nothing-to-recover" > "${EVIDENCE}/recovery-result.txt" ;;
    quiesce-started|quiesced|backups-verified)
      # Idempotent: start only stopped IDP containers; scale only deployments now at 0; other values are drift.
      for c in "${IDP_BACKEND}" "${IDP_FRONTEND}"; do
        r="$(docker inspect -f '{{.State.Running}}' "${c}")"
        [[ "${r}" == true ]] || { log "docker start ${c}"; docker start "${c}" >/dev/null; }
      done
      restore_old
      wait_healthy "${IDP_BACKEND}"
      old_apps_state 1
      echo "recovered-before-reset" > "${EVIDENCE}/recovery-result.txt" ;;
    reset-started|new-stack-up|runner-started|runner-done|resume-runner-started|resume-runner-done|final-checks-done)
      # Idempotent: every step below may be repeated after a partial earlier recovery.
      [[ "$(tag_id "${REC_BACKEND}")" == "${EXPECT_BACKEND_ID}" && "$(tag_id "${REC_FRONTEND}")" == "${EXPECT_FRONTEND_ID}" ]] || { echo "recovery image tag drift" >&2; exit 1; }
      # Newly created run app: run.json must prove name, id and namespace ownership; abort otherwise.
      if [[ -s "${UI_EVIDENCE}/run.json" ]]; then
        jf() { sed -n "s/.*\"$1\": *\"\([^\"]*\)\".*/\1/p" "${UI_EVIDENCE}/run.json"; }
        R_NAME="$(jf applicationName)" R_ID="$(jf applicationId)" NEWNS="$(jf namespace)"
        [[ "${R_NAME}" == "${APP_NAME}" && "${R_ID}" =~ ^[0-9a-f-]{36}$ && "${NEWNS}" == "app-${R_ID}-staging" ]] || { echo "run.json ownership mismatch; refusing to scale" >&2; exit 1; }
        [[ "${R_ID}" != "${APP_ID_FINAL}" && "${R_ID}" != "${APP_ID_EARLIER}" ]] || { echo "run.json names a retained app" >&2; exit 1; }
        found="$(kc get namespace "${NEWNS}" --ignore-not-found -o name)"
        if [[ -n "${found}" ]]; then
          log "quiesce new run app ${NEWNS} (FE/BE only; PVC retained)"
          for n in backend frontend; do
            d="$(kc -n "${NEWNS}" get deploy "${n}" --ignore-not-found -o name)"
            if [[ -n "${d}" ]]; then kc -n "${NEWNS}" scale "deployment/${n}" --replicas=0; fi
          done
          wait_pods_gone "${NEWNS}"
        fi
      fi
      log "recover: stop IDP containers, verify dump, restore IDP DB"
      docker stop "${IDP_BACKEND}" "${IDP_FRONTEND}" >/dev/null
      (cd "${PRIVATE}" && sha256sum -c idp.dump.sha256)
      docker exec "${PG}" dropdb -U orchestrator --force orchestrator
      docker exec "${PG}" createdb -U orchestrator -O orchestrator orchestrator
      docker exec -i "${PG}" pg_restore -U orchestrator --exit-on-error --no-owner --no-acl -d orchestrator < "${PRIVATE}/idp.dump"
      [[ "$(active_ops)" == 0 ]]
      [[ "$(q_idp "${SNAP_SQL}")" == "$(cat "${PRIVATE}/snap.before")" ]] || { echo "restored app scope differs from the pre-reset snapshot" >&2; exit 1; }
      dc_rec up -d --no-deps --no-build --pull never --wait backend frontend
      [[ "$(image_id "${IDP_BACKEND}")" == "${EXPECT_BACKEND_ID}" && "$(image_id "${IDP_FRONTEND}")" == "${EXPECT_FRONTEND_ID}" ]]
      restore_old
      old_apps_state 1
      echo "recovered-after-reset" > "${EVIDENCE}/recovery-result.txt" ;;
    *) die "unknown stage marker: ${S}" ;;
  esac
  log "recover finished OK; backups and evidence kept; no workload data restored into any new app"
  trap - EXIT
  exit 0
fi

# ---- Continue: one scoped UI resume of the same run -------------------------------
if [[ "${MODE}" == continue ]]; then
  [[ -f "${STAGE_FILE}" && ! -L "${STAGE_FILE}" && "$(cat "${STAGE_FILE}")" == runner-started ]] || die "--continue needs stage marker runner-started"
  RESUME_UI="${EVIDENCE}/ui-resume1"
  [[ ! -e "${RESUME_UI}" && ! -L "${RESUME_UI}" ]] || die "${RESUME_UI} already exists; one continuation only"
  [[ "$(cat "${EVIDENCE}/ui/result.txt")" == FAIL && ! -e "${EVIDENCE}/ui/run.json" ]] || die "first runner state is not the reviewed pre-app failure"
  trap fail_note EXIT
  trap 'exit 129' HUP; trap 'exit 130' INT; trap 'exit 143' TERM
  log "continue start run=${RUN_ID}"
  wait_healthy "${IDP_BACKEND}"
  [[ "$(image_id "${IDP_BACKEND}")" != "${EXPECT_BACKEND_ID}" ]]   # current baseline build is serving, not the old image
  [[ "$(q_idp "SELECT string_agg(application_key,',' ORDER BY application_key) FROM applications")" == acceptance ]]
  [[ "$(q_idp "SELECT string_agg(connection_key,',' ORDER BY connection_key) FROM connections")" == aws-account,internal-cluster,k8s-4f ]]
  [[ "$(q_idp "SELECT count(*) FROM connections WHERE connection_key='k8s-4f' AND name='K8S-4F'")" == 1 ]]
  [[ "$(q_idp "SELECT string_agg(store_key,',' ORDER BY store_key) FROM secret_store_connections")" == platform-vault ]]
  old_apps_state 0
  [[ "$(pv_of "${NS_FINAL}")" == "${PV_FINAL_PIN}" && "$(pv_of "${NS_EARLIER}")" == "${PV_EARLIER_PIN}" ]]
  [[ "$(uid_of "${NS_FINAL}")" == "${PVC_UID_FINAL}" && "$(uid_of "${NS_EARLIER}")" == "${PVC_UID_EARLIER}" ]]
  [[ "$(kc get namespace -o name | grep -c '^namespace/app-')" == 2 ]]
  ! ss -ltn | grep -q ":${FORWARD_PORT} " || { echo "local port ${FORWARD_PORT} is busy" >&2; exit 1; }
  ( set -o noclobber; echo started > "${PRIVATE}/resume1.started" ) 2>/dev/null || die "continuation already started (single use)"
  stage resume-runner-started
  ( umask 077; echo ui-resume1 > "${PRIVATE}/ui-evidence" ); chmod 600 "${PRIVATE}/ui-evidence"
  env -i PATH="${PATH}" HOME="${HOME}" USER="${USER:-}" LOGNAME="${LOGNAME:-}" LANG="${LANG:-C.UTF-8}" TERM="${TERM:-dumb}" \
    K8S4F_RUN_ID="${RUN_ID}" K8S4F_IMAGE_TAG="${IMAGE_TAG}" K8S4F_APPLICATION="${APP_NAME}" K8S4F_SUBDOMAIN="${APP_NAME}" \
    K8S4F_FORWARD_PORT="${FORWARD_PORT}" K8S4F_KUBECONFIG_FILE="${PRIVATE}/k8s-4f.kubeconfig" \
    K8S4F_FORBIDDEN_APP_IDS="${APP_ID_FINAL},${APP_ID_EARLIER}" K8S4F_EXPECT_EMPTY_JOBS=1 \
    K8S4F_RESUME_CONNECTION_ONLY=1 K8S4F_RESUME_CONNECTION_KEY=k8s-4f \
    bash "${ROOT}/test/integration/k8s4f-dev-playwright.sh" "${RESUME_UI}"
  stage resume-runner-done
  final_checks
  stage final-checks-done
  { echo "PASS run=${RUN_ID} (continued once via ui-resume1) new_namespace=${NEWNS}"; echo "application=${APP_NAME} plus seeded Acceptance Application; old FE/BE=0, old PostgreSQL=1, PVC UID/PV unchanged"; echo "restore smoke counts/jobs checksums equal; backups ${PRIVATE} retained ${BACKUP_DAYS} days"; } > "${EVIDENCE}/result.txt"
  log "continue finished OK"
  trap - EXIT
  exit 0
fi

# ---- Apply --------------------------------------------------------------------
mkdir -p -m 700 "$(dirname "${PRIVATE}")"
[[ ! -e "${PRIVATE}" && ! -L "${PRIVATE}" ]] || die "private dir ${PRIVATE} already exists; run IDs are single-use"
[[ ! -e "${EVIDENCE}" && ! -L "${EVIDENCE}" ]] || die "evidence dir ${EVIDENCE} already exists; run IDs are single-use"
[[ -s "${WORKLOAD_STATE}/token" ]] || die "workload Vault token file missing (path only checked)"
(( (8#$(stat -c %a "${WORKLOAD_STATE}/token") & 8#077) == 0 )) || die "workload Vault token file must be owner-only"
ss -ltn | grep -q ":${FORWARD_PORT} " && die "local port ${FORWARD_PORT} is busy"
mkdir -m 700 "${EVIDENCE}"
trap fail_note EXIT
trap 'exit 129' HUP; trap 'exit 130' INT; trap 'exit 143' TERM
log "apply start run=${RUN_ID}"

# Stage 0: read-only preflight.
[[ "$(image_id "${IDP_BACKEND}")" == "${EXPECT_BACKEND_ID}" ]]
[[ "$(image_id "${IDP_FRONTEND}")" == "${EXPECT_FRONTEND_ID}" ]]
[[ "$(docker inspect -f '{{.Config.Image}}' "${PG}")" == "${PG_IMAGE}" ]]
[[ "$(docker inspect -f '{{range .Mounts}}{{.Name}} {{end}}' "${PG}")" == *"${PG_VOLUME}"* ]]
[[ "$(q_idp 'show server_version_num' | cut -c1-2)" == 16 && "$(q_idp 'select current_user')" == orchestrator ]]
[[ "$(active_ops)" == 0 ]] || { echo "active environment operation present" >&2; exit 1; }
docker image inspect "${PG_IMAGE}" >/dev/null
{ is_empty docker ps -aq --filter "label=${SMOKE_LABEL}" && is_empty docker ps -aq --filter "name=^${SMOKE}$"; } || { echo "smoke container exists or docker read failed" >&2; exit 1; }
[[ "$(kc get node -o jsonpath='{.items[0].status.conditions[?(@.type=="Ready")].status}')" == True ]]
for w in backend frontend; do docker exec k8s-4f-control-plane crictl images 2>/dev/null | grep -q "acceptance-${w} *${IMAGE_TAG}"; done
[[ "$(kc get node -o name)" == "node/${NODE}" ]]
kc -n vault-secrets-operator-system rollout status deployment/vault-secrets-operator-controller-manager --timeout=60s >/dev/null
[[ "$(docker inspect -f '{{.State.Health.Status}}' k8s-4f-workload-vault)" == healthy ]]
ns_check "${NS_FINAL}" "${PV_FINAL_PIN}" "${PVC_UID_FINAL}"; ns_check "${NS_EARLIER}" "${PV_EARLIER_PIN}" "${PVC_UID_EARLIER}"
SNAP_BEFORE="$(q_idp "${SNAP_SQL}")"
[[ "$(printf '%s\n' "${SNAP_BEFORE}" | grep -c '|staging|k8s-4f-2|k8s-4f-vault|0|[0-9a-f-]\{36\}|[0-9]*|true|ENVIRONMENT|READY$')" == 2 ]] || { echo "app binding/generation/current-set scope drift" >&2; exit 1; }
PV_FINAL="${PV_FINAL_PIN}" PV_EARLIER="${PV_EARLIER_PIN}"
[[ "$(q_idp "SELECT count(*) FROM applications WHERE id IN ('${APP_ID_FINAL}','${APP_ID_EARLIER}')")" == 2 ]]
log "preflight OK (images, ownership, PVC, replicas, operations)"

# Stage 1: prepare without downtime.
mkdir -m 700 "${PRIVATE}"
printf '%s\n' "${SNAP_BEFORE}" > "${PRIVATE}/snap.before"
stage prepared
{ printf '%s\n' "${SNAP_BEFORE}"; echo "final_ns=${NS_FINAL} pv=${PV_FINAL} uid=${PVC_UID_FINAL}"; echo "earlier_ns=${NS_EARLIER} pv=${PV_EARLIER} uid=${PVC_UID_EARLIER}"
  kc -n "${NS_FINAL}" get deploy,sts -o jsonpath='{range .items[*]}{.kind}/{.metadata.name} replicas={.spec.replicas} image={.spec.template.spec.containers[0].image}{"\n"}{end}'
  kc -n "${NS_EARLIER}" get deploy,sts -o jsonpath='{range .items[*]}{.kind}/{.metadata.name} replicas={.spec.replicas} image={.spec.template.spec.containers[0].image}{"\n"}{end}'
} > "${PRIVATE}/inventory.txt"
docker image tag "${EXPECT_BACKEND_ID}" "${REC_BACKEND}"
docker image tag "${EXPECT_FRONTEND_ID}" "${REC_FRONTEND}"
[[ "$(tag_id "${REC_BACKEND}")" == "${EXPECT_BACKEND_ID}" && "$(tag_id "${REC_FRONTEND}")" == "${EXPECT_FRONTEND_ID}" ]]
mkdir -m 700 "${PRIVATE}/recovery" "${PRIVATE}/recovery/deploy"
git -C "${REPO}" show "${HIST_COMMIT}:docker-compose.yml" > "${PRIVATE}/recovery/docker-compose.yml"
git -C "${REPO}" show "${HIST_COMMIT}:deploy/local/compose.kind.yml" > "${PRIVATE}/recovery/compose.kind.yml"
cat > "${PRIVATE}/recovery/override.yml" <<EOF
services:
  backend:
    image: ${REC_BACKEND}
    pull_policy: never
  frontend:
    image: ${REC_FRONTEND}
    pull_policy: never
EOF
env BACKEND_IMAGE="${NEW_BACKEND}" FRONTEND_IMAGE="${NEW_FRONTEND}" docker compose --env-file /dev/null -p orchestrator-local \
  -f "${REPO}/docker-compose.yml" -f "${REPO}/deploy/local/compose.kind.yml" build backend frontend > "${PRIVATE}/build.log" 2>&1
docker image inspect "${NEW_BACKEND}" "${NEW_FRONTEND}" >/dev/null
[[ "$(image_id "${IDP_BACKEND}")" == "${EXPECT_BACKEND_ID}" ]]   # builds did not replace the running containers
kc config view --flatten --raw --minify --context "${CTX}" > "${PRIVATE}/k8s-4f.raw"
[[ "$(grep -c '^ *server:' "${PRIVATE}/k8s-4f.raw")" == 1 ]]
sed -E 's#^( *server:).*#\1 https://k8s-4f-control-plane:6443#' "${PRIVATE}/k8s-4f.raw" > "${PRIVATE}/k8s-4f.kubeconfig"
rm -f "${PRIVATE}/k8s-4f.raw"; chmod 600 "${PRIVATE}/k8s-4f.kubeconfig"
log "prepared: recovery tags, historical compose, current builds, private kubeconfig (not shown)"

# Stage 2: quiesce. Marker first so --recover knows downtime may have started.
stage quiesce-started
docker stop "${IDP_BACKEND}" "${IDP_FRONTEND}" >/dev/null
scale_old_zero
[[ "$(active_ops)" == 0 ]]
[[ "$(q_idp "${SNAP_SQL}")" == "${SNAP_BEFORE}" ]] || { echo "app scope changed during quiesce" >&2; exit 1; }
old_apps_state 0
! curl -fsS -m 3 "http://127.0.0.1:${IDP_PUBLIC_PORT}/" >/dev/null 2>&1 || { echo "old frontend still serves on ${IDP_PUBLIC_PORT}" >&2; exit 1; }
stage quiesced

# Stage 3: backups + restore smoke.
# Retention record first, then one checksum line per completed dump, so a later failure keeps it.
echo "run=${RUN_ID} created=$(date -u +%FT%TZ) expires=$(date -u -d "+${BACKUP_DAYS} days" +%FT%TZ) (30 days; record only, no automatic deletion)" > "${PRIVATE}/retention.txt"
record_dump() { [[ -s "${PRIVATE}/$1.dump" ]]; chmod 600 "${PRIVATE}/$1.dump"
  (cd "${PRIVATE}" && sha256sum "$1.dump" > "$1.dump.sha256" && chmod 600 "$1.dump.sha256" && cat "$1.dump.sha256" >> retention.txt); }
docker exec "${PG}" pg_dump -U orchestrator -d orchestrator -Fc > "${PRIVATE}/idp.dump"; record_dump idp
q_ns "${NS_FINAL}" "select 1" >/dev/null
kc -n "${NS_FINAL}" exec "${DBPOD}" -- pg_dump -U acceptance -d acceptance -Fc > "${PRIVATE}/final.dump"; record_dump final
kc -n "${NS_EARLIER}" exec "${DBPOD}" -- pg_dump -U acceptance -d acceptance -Fc > "${PRIVATE}/earlier.dump"; record_dump earlier
q_idp "${COUNT_SQL}" > "${PRIVATE}/idp.counts.src"
for f in final earlier; do
  ns="NS_${f^^}"; ns="${!ns}"
  q_ns "${ns}" "${COUNT_SQL}" > "${PRIVATE}/${f}.counts.src"
  q_ns "${ns}" "SELECT CASE WHEN to_regclass('public.jobs') IS NULL THEN 'no-table' ELSE 'has-table' END" > "${PRIVATE}/${f}.jobs.kind"
  if [[ "$(cat "${PRIVATE}/${f}.jobs.kind")" == has-table ]]; then q_ns "${ns}" "${JOBS_SQL}" > "${PRIVATE}/${f}.jobs.src"; else echo no-table > "${PRIVATE}/${f}.jobs.src"; fi
done
log "backups written, checksummed, retention record written; source counts recorded privately"

docker create --pull never --name "${SMOKE}" --label "${SMOKE_LABEL}" --network none --tmpfs /var/lib/postgresql/data:rw \
  -e POSTGRES_USER=smoke -e POSTGRES_HOST_AUTH_METHOD=trust "${PG_IMAGE}" >/dev/null
docker start "${SMOKE}" >/dev/null
ready=0; for i in $(seq 60); do
  [[ "$(docker logs "${SMOKE}" 2>&1 | grep -c 'database system is ready to accept connections')" -ge 2 ]] && docker exec "${SMOKE}" pg_isready -U smoke >/dev/null 2>&1 && { ready=1; break; }
  sleep 1; done
[[ "${ready}" == 1 ]]
for f in idp final earlier; do
  docker exec "${SMOKE}" createdb -U smoke "t29_${f}"
  docker exec -i "${SMOKE}" pg_restore --list < "${PRIVATE}/${f}.dump" > "${PRIVATE}/${f}.list" 2> "${PRIVATE}/${f}.list.err"
  docker exec -i "${SMOKE}" pg_restore --exit-on-error --no-owner --no-acl -U smoke -d "t29_${f}" < "${PRIVATE}/${f}.dump" > "${PRIVATE}/${f}.restore.log" 2>&1
  q_smoke "t29_${f}" "${COUNT_SQL}" > "${PRIVATE}/${f}.counts.restored"
  cmp "${PRIVATE}/${f}.counts.src" "${PRIVATE}/${f}.counts.restored"
  [[ -s "${PRIVATE}/${f}.counts.src" ]]
  if [[ "${f}" != idp ]]; then
    if [[ "$(cat "${PRIVATE}/${f}.jobs.kind")" == has-table ]]; then q_smoke "t29_${f}" "${JOBS_SQL}" > "${PRIVATE}/${f}.jobs.restored"; else echo no-table > "${PRIVATE}/${f}.jobs.restored"; fi
    cmp "${PRIVATE}/${f}.jobs.src" "${PRIVATE}/${f}.jobs.restored"
  fi
  extra=""
  if [[ "${f}" != idp ]]; then extra=", jobs checksum equal ($(cat "${PRIVATE}/${f}.jobs.src"))"; fi
  log "restore smoke ${f}: tables=$(wc -l < "${PRIVATE}/${f}.counts.src") counts equal${extra}"
done
smoke_cleanup
stage backups-verified

# Stage 4: reset only the IDP database; start current backend/frontend only.
stage reset-started
docker exec "${PG}" dropdb -U orchestrator --force orchestrator
docker exec "${PG}" createdb -U orchestrator -O orchestrator orchestrator
env BACKEND_IMAGE="${NEW_BACKEND}" FRONTEND_IMAGE="${NEW_FRONTEND}" docker compose --env-file /dev/null -p orchestrator-local \
  -f "${REPO}/docker-compose.yml" -f "${REPO}/deploy/local/compose.kind.yml" up -d --no-build --no-deps --pull never --wait backend frontend
# Current seed.Apply always creates the Acceptance catalog (cloud app only with a region; none here).
# Assert exactly that seed: no retained ID, no history, no run app yet.
[[ "$(q_idp "SELECT string_agg(application_key,',' ORDER BY application_key) FROM applications")" == acceptance ]]
[[ "$(q_idp "SELECT count(*) FROM applications WHERE id IN ('${APP_ID_FINAL}','${APP_ID_EARLIER}') OR name='${APP_NAME}'")" == 0 ]]
[[ "$(q_idp "SELECT string_agg(connection_key,',' ORDER BY connection_key) FROM connections")" == aws-account,internal-cluster ]]
# seed.Apply saves exactly one empty DeploymentSet (seed.go DocumentHash literal 'empty') bound to the seed Environment; no deployments.
[[ "$(q_idp "SELECT (SELECT count(*) FROM deployments)||'/'||(SELECT count(*) FROM deployment_sets)||'/'||(SELECT count(*) FROM environments)||'/'||(SELECT count(*) FROM environments e JOIN deployment_sets d ON d.id=e.current_deployment_set_id AND d.environment_id=e.id WHERE d.document_hash='empty' AND d.created_by_deployment_id IS NULL)")" == 0/1/1/1 ]]
log "IDP database reset; baseline holds only the seeded acceptance catalog (1 application, 2 seeded Connections, one empty seed DeploymentSet, no deployments)"
stage new-stack-up

# Stage 5: live UI runner, exactly once.
stage runner-started
env -i PATH="${PATH}" HOME="${HOME}" USER="${USER:-}" LOGNAME="${LOGNAME:-}" LANG="${LANG:-C.UTF-8}" TERM="${TERM:-dumb}" \
  K8S4F_RUN_ID="${RUN_ID}" K8S4F_IMAGE_TAG="${IMAGE_TAG}" K8S4F_APPLICATION="${APP_NAME}" K8S4F_SUBDOMAIN="${APP_NAME}" \
  K8S4F_FORWARD_PORT="${FORWARD_PORT}" K8S4F_KUBECONFIG_FILE="${PRIVATE}/k8s-4f.kubeconfig" \
  K8S4F_FORBIDDEN_APP_IDS="${APP_ID_FINAL},${APP_ID_EARLIER}" K8S4F_EXPECT_EMPTY_JOBS=1 \
  bash "${ROOT}/test/integration/k8s4f-dev-playwright.sh" "${UI_EVIDENCE}"
stage runner-done

final_checks
stage final-checks-done
{ echo "PASS run=${RUN_ID} new_namespace=${NEWNS}"; echo "new application=${APP_NAME} plus seeded Acceptance Application; old FE/BE=0, old PostgreSQL=1, PVC unchanged"; echo "restore smoke counts/jobs checksums equal; backups ${PRIVATE} retained ${BACKUP_DAYS} days"; } > "${EVIDENCE}/result.txt"
log "apply finished OK"
trap - EXIT
exit 0
