#!/usr/bin/env bash
# Real end-user acceptance of Git-owned manifest rendering (ADR-021, RB-E01..RB-E10).
#
#   bash backend/test/integration/git-render-bundle-kind.sh --evidence /abs/new/dir [--run-id id]
#
# WHAT IS REAL: a NEW run-owned kind cluster with Vault Secrets Operator, a run-owned
# workload Vault, a NEW Compose project (PostgreSQL, Vault, backend and frontend built
# once from this tree), Terraform with the verified Kubernetes modules, score-k8s
# 0.15.0, the template worker, the acceptance application and a headed Chromium on
# Xvfb. Bundle A and B are two real commits of the same directory on the task-owned
# ref test/manifest-bundle/<run-id> of https://github.com/pr3s3nt/poc.git, inspected
# through the product's HTTPS Git transport. WHAT IS FAKE: nothing.
#
# Ownership: everything this run creates is named after the run ID and listed in
# <evidence>/run-manifest.json before it is created. Cleanup deletes only those exact
# objects (the cluster, the Compose project with its volumes, the workload Vault
# container/volumes, the fixture ref and worktree, run-tagged images). Retained
# installations, other kind clusters and other refs are never touched.
set -euo pipefail
umask 077
export LC_ALL=C.UTF-8

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
REPO="$(cd "${ROOT}/.." && pwd)"
usage() { echo "usage: $0 --evidence /absolute/new/dir [--run-id lowercase-id]" >&2; exit 2; }
EVIDENCE="" RUN_ID=""
while (( $# )); do
  case "$1" in
    --evidence) [[ $# -ge 2 ]] || usage; EVIDENCE="$2"; shift 2 ;;
    --run-id) [[ $# -ge 2 ]] || usage; RUN_ID="$2"; shift 2 ;;
    *) usage ;;
  esac
done
[[ "${EVIDENCE}" == /* ]] || usage
[[ ! -e "${EVIDENCE}" ]] || { echo "evidence directory must be new" >&2; exit 2; }
case "$(realpath -m "${EVIDENCE}")/" in "${REPO}/"*) echo "evidence must be outside the repository" >&2; exit 2 ;; esac
RUN_ID="${RUN_ID:-$(date -u +%m%d%H%M)$(printf '%04x' $((RANDOM)))}"
[[ "${RUN_ID}" =~ ^[a-z0-9]{4,16}$ ]] || { echo "run id must be 4-16 lowercase letters/digits" >&2; exit 2; }

CLUSTER="manifest-bundle-${RUN_ID}"
CONTEXT="kind-${CLUSTER}"
PROJECT="manifest-bundle-${RUN_ID}"
FIXTURE_REF="test/manifest-bundle/${RUN_ID}"
ORIGIN_URL="https://github.com/pr3s3nt/poc.git"
BUNDLE_PATH="backend/render-bundles/standard"
VAULT_NAME="${CLUSTER}-workload-vault"
TAG="mb-${RUN_ID}"
PRIVATE="${EVIDENCE}/private"
LOG="${EVIDENCE}/runner.log"
for tool in docker kind kubectl helm node go git curl jq xvfb-run; do command -v "${tool}" >/dev/null || { echo "${tool} is required" >&2; exit 2; }; done
[[ -d "${REPO}/frontend/node_modules/@playwright/test" ]] || { echo "install frontend dependencies first" >&2; exit 2; }
docker info >/dev/null
docker network inspect kind >/dev/null 2>&1 || true
! kind get clusters 2>/dev/null | grep -qx "${CLUSTER}" || { echo "cluster ${CLUSTER} already exists; run IDs are single-use" >&2; exit 2; }
[[ -z "$(docker ps -aq --filter "label=com.docker.compose.project=${PROJECT}")" ]] || { echo "compose project ${PROJECT} exists" >&2; exit 2; }
! git -C "${REPO}" ls-remote --exit-code "${ORIGIN_URL}" "refs/heads/${FIXTURE_REF}" >/dev/null 2>&1 || { echo "fixture ref exists" >&2; exit 2; }

mkdir -m 0700 "${EVIDENCE}" "${PRIVATE}"
exec > >(tee -a "${LOG}") 2>&1
log() { echo "[$(date -u +%H:%M:%S)] $*"; }
free_port() { node -e 'const s=require("net").createServer();s.listen(0,"127.0.0.1",()=>{console.log(s.address().port);s.close()})'; }
BACKEND_PORT="$(free_port)"
FRONTEND_PORT="$(free_port)"
HEAD_COMMIT="$(git -C "${REPO}" rev-parse HEAD)"
jq -n --arg run "${RUN_ID}" --arg cluster "${CLUSTER}" --arg project "${PROJECT}" --arg ref "${FIXTURE_REF}" --arg vault "${VAULT_NAME}" \
  --arg tag "${TAG}" --arg head "${HEAD_COMMIT}" --argjson bp "${BACKEND_PORT}" --argjson fp "${FRONTEND_PORT}" \
  '{runId:$run, commitUnderTest:$head, kindCluster:$cluster, composeProject:$project, fixtureRef:$ref, workloadVault:{container:$vault, volumes:[($vault+"-data"),($vault+"-bootstrap")]},
    images:["orchestrator-backend:"+$tag,"orchestrator-frontend:"+$tag,"acceptance-frontend:"+$tag,"acceptance-backend:"+$tag,"acceptance-worker:"+$tag], ports:{backend:$bp, frontend:$fp}}' > "${EVIDENCE}/run-manifest.json"
chmod 0644 "${EVIDENCE}/run-manifest.json"

KUBECONFIG_HOST="${PRIVATE}/kind.kubeconfig"
COMPOSE=(docker compose --env-file /dev/null --project-name "${PROJECT}" -f "${REPO}/docker-compose.yml" -f "${REPO}/deploy/local/compose.kind.yml" -f "${REPO}/deploy/local/compose.git-render-bundle.yml")
unset COMPOSE_FILE COMPOSE_PROJECT_NAME
export BACKEND_IMAGE="orchestrator-backend:${TAG}" FRONTEND_IMAGE="orchestrator-frontend:${TAG}" BACKEND_PORT FRONTEND_PORT
FIXTURE_PUSHED=0
CLUSTER_CREATED=0
STATUS_FILE="${EVIDENCE}/cleanup.txt"

cleanup() {
  local status=$?
  set +e
  log "cleanup (exit ${status})"
  {
    echo "run=${RUN_ID} scenario_exit=${status}"
    if ! docker info >/dev/null 2>&1; then echo "Docker is unavailable; absence cannot be verified"; status=1; fi
    if [[ -f "${PRIVATE}/compose-up" ]]; then
      "${COMPOSE[@]}" down --volumes --remove-orphans >/dev/null 2>&1
      [[ -z "$(docker ps -aq --filter "label=com.docker.compose.project=${PROJECT}")" ]] && echo "compose ${PROJECT}: containers absent" || { echo "compose ${PROJECT}: containers remain"; status=1; }
      [[ -z "$(docker volume ls -q --filter "label=com.docker.compose.project=${PROJECT}")" ]] && echo "compose ${PROJECT}: volumes absent" || { echo "compose ${PROJECT}: volumes remain"; status=1; }
    fi
    if docker inspect "${VAULT_NAME}" >/dev/null 2>&1; then
      if [[ "$(docker inspect -f '{{index .Config.Labels "poc.workload-vault/cluster"}}' "${VAULT_NAME}")" == "${CLUSTER}" ]]; then
        docker rm -f "${VAULT_NAME}" >/dev/null && docker volume rm "${VAULT_NAME}-data" "${VAULT_NAME}-bootstrap" >/dev/null
      else echo "workload vault ${VAULT_NAME} not owned; retained"; status=1; fi
    fi
    docker inspect "${VAULT_NAME}" >/dev/null 2>&1 && { echo "workload vault remains"; status=1; } || echo "workload vault ${VAULT_NAME}: absent"
    if [[ "${CLUSTER_CREATED}" == 1 ]]; then
      kind delete cluster --name "${CLUSTER}" --kubeconfig "${KUBECONFIG_HOST}" >/dev/null 2>&1
      kind get clusters 2>/dev/null | grep -qx "${CLUSTER}" && { echo "kind ${CLUSTER} remains"; status=1; } || echo "kind ${CLUSTER}: absent"
    fi
    if [[ "${FIXTURE_PUSHED}" == 1 ]]; then
      git -C "${REPO}" push --quiet "${ORIGIN_URL}" --delete "refs/heads/${FIXTURE_REF}" >/dev/null 2>&1
      local remote_refs
      if remote_refs="$(git -C "${REPO}" ls-remote "${ORIGIN_URL}" "refs/heads/${FIXTURE_REF}" 2>/dev/null)"; then
        if [[ -z "${remote_refs}" ]]; then echo "fixture ref ${FIXTURE_REF}: absent"; else echo "fixture ref remains"; status=1; fi
      else echo "fixture ref absence could not be verified"; status=1; fi
    fi
    if [[ -d "${PRIVATE}/fixture" ]]; then git -C "${REPO}" worktree remove --force "${PRIVATE}/fixture" >/dev/null 2>&1; fi
    git -C "${REPO}" worktree prune
    local images_removed=1
    for image in "orchestrator-backend:${TAG}" "orchestrator-frontend:${TAG}" "acceptance-frontend:${TAG}" "acceptance-backend:${TAG}" "acceptance-worker:${TAG}"; do
      docker rmi "${image}" >/dev/null 2>&1
      if docker image inspect "${image}" >/dev/null 2>&1; then echo "image ${image}: remains"; status=1; images_removed=0; fi
    done
    if [[ "${images_removed}" == 1 ]]; then echo "images ${TAG}: removed"; fi
    echo "final_status=${status}"
  } > "${STATUS_FILE}" 2>&1
  # Keep status in this shell: piping the group through tee would lose failures
  # in a subshell and let the runner exit successfully after failed teardown.
  cat "${STATUS_FILE}"
  # Credentials and private inputs never leave the private directory; it is removed after cleanup.
  if grep -q '^final_status=0$' "${STATUS_FILE}"; then rm -rf "${PRIVATE}"; else echo "private inputs retained for recovery: ${PRIVATE}"; fi
  exit "${status}"
}
trap cleanup EXIT
trap 'exit 129' HUP; trap 'exit 130' INT; trap 'exit 143' TERM

log "versions"
{ docker version --format 'docker {{.Server.Version}}'; kind version; kubectl version --client -o json | jq -r '"kubectl " + .clientVersion.gitVersion'; helm version --short; node --version; go version; git --version; } | tee "${EVIDENCE}/versions.txt"

# secret_scan: no run secret, Vault token or kubeconfig key/certificate material in public
# evidence. Only file names are reported, never a value.
secret_scan() {
  local needles="${PRIVATE}/scan-needles" hits=0 needle file
  : > "${needles}"
  [[ -s "${PRIVATE}/app-secrets.txt" ]] && cut -d= -f2- "${PRIVATE}/app-secrets.txt" >> "${needles}"
  [[ -s "${PRIVATE}/workload-vault/token" ]] && cat "${PRIVATE}/workload-vault/token" >> "${needles}" && echo >> "${needles}"
  sed -n 's/^ *\(client-key-data\|client-certificate-data\|certificate-authority-data\): *//p' "${PRIVATE}/internal.kubeconfig" >> "${needles}"
  while IFS= read -r needle; do
    [[ ${#needle} -ge 8 ]] || continue
    while IFS= read -r file; do echo "secret material found in ${file#"${EVIDENCE}/"}"; hits=1; done < <(grep -rlF --exclude-dir=private -- "${needle}" "${EVIDENCE}" || true)
  done < "${needles}"
  rm -f "${needles}"
  if [[ "${hits}" == 0 ]]; then echo "secret scan: no run secret, Vault token or kubeconfig material in public evidence" | tee "${EVIDENCE}/secret-scan.txt"; else echo "secret scan FAILED" | tee "${EVIDENCE}/secret-scan.txt"; return 1; fi
}

# ---- 1. Cluster, VSO and workload Vault (technical setup) -----------------------------------
log "creating kind cluster ${CLUSTER}"
CLUSTER_CREATED=1
kind create cluster --name "${CLUSTER}" --kubeconfig "${KUBECONFIG_HOST}" --wait 180s
export KUBECONFIG="${KUBECONFIG_HOST}"
kubectl --context "${CONTEXT}" get nodes -o name
helm --kube-context "${CONTEXT}" install vault-secrets-operator hashicorp/vault-secrets-operator --version 1.5.1 \
  --namespace vault-secrets-operator-system --create-namespace --wait --timeout 5m >/dev/null
WORKLOAD_VAULT_STATE_DIR="${PRIVATE}/workload-vault" WORKLOAD_VAULT_CONTAINER="${VAULT_NAME}" bash "${REPO}/deploy/local/workload-vault/start.sh" "${CLUSTER}"
kind get kubeconfig --internal --name "${CLUSTER}" > "${PRIVATE}/internal.kubeconfig"
chmod 0600 "${PRIVATE}/internal.kubeconfig"

# ---- 2. Images built once from this tree -----------------------------------------------------
log "building images ${TAG}"
docker build --quiet -t "orchestrator-backend:${TAG}" "${REPO}/backend" >/dev/null
docker build --quiet -t "orchestrator-frontend:${TAG}" "${REPO}/frontend" >/dev/null
(cd "${ROOT}" && ./test/integration/build-images.sh "${TAG}" "${PRIVATE}/acceptance-bin" >/dev/null)
for workload in frontend backend worker; do kind load docker-image "acceptance-${workload}:${TAG}" --name "${CLUSTER}" >/dev/null; done
docker image inspect -f '{{.Id}}' "orchestrator-backend:${TAG}" > "${EVIDENCE}/backend-image-id.txt"
(cd "${ROOT}" && go build -o "${PRIVATE}/v2ctl" ./test/integration/v2ctl)
docker run --rm --entrypoint score-k8s "orchestrator-backend:${TAG}" --version | head -n 1 | tee "${EVIDENCE}/score-k8s-version.txt"
docker run --rm --entrypoint sh "orchestrator-backend:${TAG}" -c 'sha256sum /usr/local/bin/score-k8s' | cut -d' ' -f1 > "${EVIDENCE}/score-k8s-sha256.txt"

# ---- 3. Fixture commits A and B of the same directory on the task-owned ref ------------------
log "preparing fixture ref ${FIXTURE_REF}"
git -C "${REPO}" worktree add --quiet --detach "${PRIVATE}/fixture" HEAD
FIX="${PRIVATE}/fixture/${BUNDLE_PATH}"
mkdir -p "${FIX}"
cp -R "${REPO}/${BUNDLE_PATH}/." "${FIX}/"
sed -i 's/cpu: 10m/cpu: 20m/; s|^podAnnotations: {}|podAnnotations:\n  example.com/render-policy: a|' "${FIX}/defaults.yaml"
git -C "${PRIVATE}/fixture" add "${BUNDLE_PATH}"
git -C "${PRIVATE}/fixture" -c user.name="Git render acceptance" -c user.email="it.thanhnt97@gmail.com" commit --quiet -m "test(fixture): render bundle A for run ${RUN_ID}"
COMMIT_A="$(git -C "${PRIVATE}/fixture" rev-parse HEAD)"
sed -i 's/cpu: 20m/cpu: 30m/; s|example.com/render-policy: a|example.com/render-policy: b|; s|^podSecurityContext: {}|podSecurityContext:\n  seccompProfile:\n    type: RuntimeDefault|' "${FIX}/defaults.yaml"
# B also changes normalization (a Score annotation) and a patch (revisionHistoryLimit).
sed -i 's|"metadata" (dict "name" .Input.identity.workload)|"metadata" (dict "name" .Input.identity.workload "annotations" (dict "example.com/normalized-by" "b"))|' "${FIX}/normalize/score.tpl"
perl -0pi -e 's|(- op: delete\n  path: "\{\{ \$i \}\}.metadata.annotations"\n)|$1- op: set\n  path: "{{ \$i }}.spec.revisionHistoryLimit"\n  value: 5\n|' "${FIX}/patches/deployment.tpl"
grep -q 'normalized-by' "${FIX}/normalize/score.tpl" && grep -q 'revisionHistoryLimit' "${FIX}/patches/deployment.tpl" && grep -q 'RuntimeDefault' "${FIX}/defaults.yaml"
git -C "${PRIVATE}/fixture" add "${BUNDLE_PATH}"
git -C "${PRIVATE}/fixture" -c user.name="Git render acceptance" -c user.email="it.thanhnt97@gmail.com" commit --quiet -m "test(fixture): render bundle B for run ${RUN_ID}"
COMMIT_B="$(git -C "${PRIVATE}/fixture" rev-parse HEAD)"
FIXTURE_PUSHED=1
git -C "${PRIVATE}/fixture" push --quiet "${ORIGIN_URL}" "${COMMIT_B}:refs/heads/${FIXTURE_REF}"
jq -n --arg a "${COMMIT_A}" --arg b "${COMMIT_B}" --arg ref "${FIXTURE_REF}" --arg path "${BUNDLE_PATH}" '{ref:$ref, path:$path, commitA:$a, commitB:$b}' > "${EVIDENCE}/fixture.json"
chmod 0644 "${EVIDENCE}/fixture.json"

# ---- 4. Run-owned Console stack --------------------------------------------------------------
log "starting compose project ${PROJECT} (backend :${BACKEND_PORT}, frontend :${FRONTEND_PORT})"
touch "${PRIVATE}/compose-up"
"${COMPOSE[@]}" up -d --wait --wait-timeout 300 --no-build
BACKEND_CONTAINER="$("${COMPOSE[@]}" ps -q backend)"
docker inspect -f '{{.Config.Image}} {{.Image}} {{.State.StartedAt}}' "${BACKEND_CONTAINER}" | tee "${EVIDENCE}/backend-before.txt"
cat > "${PRIVATE}/db-query" <<EOF
#!/usr/bin/env bash
exec docker compose --env-file /dev/null --project-name "${PROJECT}" -f "${REPO}/docker-compose.yml" -f "${REPO}/deploy/local/compose.kind.yml" -f "${REPO}/deploy/local/compose.git-render-bundle.yml" exec -T postgres psql -v ON_ERROR_STOP=1 -U orchestrator -d orchestrator -tAc "\$1"
EOF
chmod 0700 "${PRIVATE}/db-query"
curl -fsS "http://127.0.0.1:${FRONTEND_PORT}/ui/" >/dev/null

# ---- 5. Browser scenario ---------------------------------------------------------------------
log "running the browser scenario"
set +e
GRB_URL="http://127.0.0.1:${FRONTEND_PORT}" GRB_EVIDENCE="${EVIDENCE}" GRB_PRIVATE="${PRIVATE}" GRB_RUN_ID="${RUN_ID}" \
  GRB_GIT_URL="${ORIGIN_URL}" GRB_GIT_PATH="${BUNDLE_PATH}" GRB_REF_A="${COMMIT_A}" GRB_REF_B="${FIXTURE_REF}" GRB_COMMIT_A="${COMMIT_A}" GRB_COMMIT_B="${COMMIT_B}" \
  GRB_BACKEND_CONTAINER="${BACKEND_CONTAINER}" GRB_COMPOSE_PROJECT="${PROJECT}" GRB_IMAGE_TAG="${TAG}" GRB_V2CTL="${PRIVATE}/v2ctl" GRB_DB_QUERY="${PRIVATE}/db-query" \
  GRB_INTERNAL_KUBECONFIG="${PRIVATE}/internal.kubeconfig" GRB_HOST_KUBECONFIG="${KUBECONFIG_HOST}" GRB_CONTEXT="${CONTEXT}" \
  GRB_VAULT_ADDR="http://${VAULT_NAME}:8200" GRB_WORKLOAD_VAULT_TOKEN_FILE="${PRIVATE}/workload-vault/token" \
  xvfb-run -a -s "-screen 0 1600x1100x24" node "${REPO}/frontend/test/e2e/git-render-bundle-kind.mjs"
SCENARIO=$?
set -e
"${COMPOSE[@]}" logs --no-color backend > "${PRIVATE}/backend.log" 2>&1 || true
docker inspect -f '{{.Config.Image}} {{.Image}} {{.State.StartedAt}}' "${BACKEND_CONTAINER}" | tee "${EVIDENCE}/backend-after.txt"
# Public evidence must hold no run secret, kubeconfig or Vault token.
secret_scan || SCENARIO=1
[[ "${SCENARIO}" == 0 ]] || { log "scenario failed"; exit 1; }
log "scenario passed"
