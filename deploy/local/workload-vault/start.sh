#!/usr/bin/env bash
# Starts (or reuses) the dedicated workload Vault container for one kind
# cluster and configures its Kubernetes auth. Idempotent only for an exact,
# script-owned match; anything else fails without touching it. Prints no secrets.
#   deploy/local/workload-vault/start.sh <kind-cluster-name>
# Container <cluster>-workload-vault is attached to the external "kind" Docker
# network, so both the Compose backend (compose.kind.yml) and cluster Pods
# resolve it by name: http://<container>:8200.
#
# Token lifetime: the scoped token is periodic (768h) and renewed every minute
# while the container runs, including after restarts. After more than 768h of
# downtime the token expires and this script mints a new one, but the Console
# Secret Store keeps the ORIGINAL credential: refresh that store's credential or
# register a new store. Recovery is not automatic in that case.
set -euo pipefail
CLUSTER="${1:?kind cluster name}"
[[ "${CLUSTER}" =~ ^[a-z0-9]([a-z0-9-]*[a-z0-9])?$ ]] || { echo "invalid cluster name" >&2; exit 2; }
CTX="kind-${CLUSTER}"
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
NAME="${WORKLOAD_VAULT_CONTAINER:-${CLUSTER}-workload-vault}"
[[ "${NAME}" =~ ^[a-z0-9][a-z0-9_.-]*$ ]] || { echo "invalid container name" >&2; exit 2; }
STATE="${WORKLOAD_VAULT_STATE_DIR:-${HOME}/.local/share/poc-${CLUSTER}-vault}"
IMAGE="${VAULT_IMAGE:-hashicorp/vault:1.21}"
OWNER_LABEL="poc.workload-vault/cluster"
K8S_LABEL="poc.workload-vault/owner"
umask 077

# Preflight: the named kind cluster, context and network must exist.
kind get clusters | grep -qx "${CLUSTER}" || { echo "kind cluster ${CLUSTER} not found" >&2; exit 1; }
kubectl config get-contexts -o name | grep -qx "${CTX}" || { echo "context ${CTX} not found" >&2; exit 1; }
kubectl --context "${CTX}" get --raw /readyz >/dev/null
docker network inspect kind >/dev/null

# Temp files and the container-side copies are removed only once this script
# has verified (or created) a container it owns.
# Both are set only after this attempt created its own unique temp paths.
owned_container=false
host_tmp=""
container_tmp=""
cleanup() {
  [[ -z "${host_tmp}" ]] || rm -rf -- "${host_tmp}"
  [[ -z "${container_tmp}" ]] || docker exec "${NAME}" rm -rf -- "${container_tmp}" >/dev/null 2>&1 || true
}
trap cleanup EXIT

# Expected mounts, used for creation and for the reuse check.
declare -A BINDS=(
  [/local/vault.hcl]="${HERE}/vault.hcl"
  [/local/entrypoint.sh]="${HERE}/entrypoint.sh"
  [/local/applications-policy.hcl]="$(cd "${HERE}/.." && pwd)/applications-policy.hcl"
  [/token]="${STATE}"
)
declare -A VOLUMES=([/vault/data]="${NAME}-data" [/vault/bootstrap]="${NAME}-bootstrap")

verify_owned_container() {
  local fmt
  [[ "$(docker inspect -f "{{index .Config.Labels \"${OWNER_LABEL}\"}}" "${NAME}")" == "${CLUSTER}" ]] \
    || { echo "container ${NAME} exists but is not a workload vault for ${CLUSTER}; not touching it" >&2; return 1; }
  [[ "$(docker inspect -f '{{.Config.Image}}' "${NAME}")" == "${IMAGE}" ]] || { echo "container ${NAME} uses another image" >&2; return 1; }
  docker inspect -f '{{range $k, $_ := .NetworkSettings.Networks}}{{$k}} {{end}}' "${NAME}" | grep -qw kind \
    || { echo "container ${NAME} is not on the kind network" >&2; return 1; }
  local dest
  for dest in "${!BINDS[@]}"; do
    fmt="{{range .Mounts}}{{if eq .Destination \"${dest}\"}}{{.Type}}:{{.Source}}{{end}}{{end}}"
    [[ "$(docker inspect -f "${fmt}" "${NAME}")" == "bind:${BINDS[${dest}]}" ]] || { echo "container ${NAME}: ${dest} is not the expected bind" >&2; return 1; }
  done
  for dest in "${!VOLUMES[@]}"; do
    fmt="{{range .Mounts}}{{if eq .Destination \"${dest}\"}}{{.Type}}:{{.Name}}{{end}}{{end}}"
    [[ "$(docker inspect -f "${fmt}" "${NAME}")" == "volume:${VOLUMES[${dest}]}" ]] || { echo "container ${NAME}: ${dest} is not the expected volume" >&2; return 1; }
  done
}

if docker inspect "${NAME}" >/dev/null 2>&1; then
  verify_owned_container
  owned_container=true
  docker start "${NAME}" >/dev/null
else
  mkdir -p "${STATE}"
  chmod 700 "${STATE}"
  docker run -d --name "${NAME}" --label "${OWNER_LABEL}=${CLUSTER}" --restart unless-stopped --network kind --user 0:0 \
    --entrypoint /bin/sh -e VAULT_ADDR=http://127.0.0.1:8200 -e "TOKEN_OWNER=$(id -u):$(id -g)" \
    -v "${BINDS[/local/vault.hcl]}:/local/vault.hcl:ro" \
    -v "${BINDS[/local/entrypoint.sh]}:/local/entrypoint.sh:ro" \
    -v "${BINDS[/local/applications-policy.hcl]}:/local/applications-policy.hcl:ro" \
    -v "${VOLUMES[/vault/data]}:/vault/data" -v "${VOLUMES[/vault/bootstrap]}:/vault/bootstrap" \
    -v "${STATE}:/token" \
    --health-cmd 'test -f /tmp/vault-ready && vault status >/dev/null 2>&1' --health-interval 5s --health-retries 30 \
    "${IMAGE}" /local/entrypoint.sh >/dev/null
  owned_container=true
fi
for _ in $(seq 1 60); do
  [[ "$(docker inspect -f '{{.State.Health.Status}}' "${NAME}")" == healthy ]] && break
  sleep 2
done
[[ "$(docker inspect -f '{{.State.Health.Status}}' "${NAME}")" == healthy ]] || { echo "${NAME} not healthy" >&2; exit 1; }
chmod 700 "${STATE}"

# Cluster side: fixed names, so every object is checked for this owner label
# before apply; a foreign object of the same name fails the script.
k8s_owner() { kubectl --context "${CTX}" "$@" --ignore-not-found -o "jsonpath={.metadata.labels.poc\.workload-vault/owner}"; }
check_k8s() { # <expected-owner-or-empty-if-absent> description + get args
  local description="$1"; shift
  local found
  found="$(kubectl --context "${CTX}" "$@" --ignore-not-found -o name)"
  if [[ -n "${found}" ]]; then
    [[ "$(k8s_owner "$@")" == "${NAME}" ]] || { echo "${description} exists and is not owned by ${NAME}; not touching it" >&2; exit 1; }
  fi
}
check_k8s "namespace vault-auth" get namespace vault-auth
check_k8s "serviceaccount vault-reviewer" -n vault-auth get serviceaccount vault-reviewer
check_k8s "clusterrolebinding vault-reviewer-auth-delegator" get clusterrolebinding vault-reviewer-auth-delegator
check_k8s "secret vault-reviewer-token" -n vault-auth get secret vault-reviewer-token
kubectl --context "${CTX}" apply -f - >/dev/null <<YAML
apiVersion: v1
kind: Namespace
metadata: {name: vault-auth, labels: {${K8S_LABEL}: "${NAME}"}}
---
apiVersion: v1
kind: ServiceAccount
metadata: {name: vault-reviewer, namespace: vault-auth, labels: {${K8S_LABEL}: "${NAME}"}}
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata: {name: vault-reviewer-auth-delegator, labels: {${K8S_LABEL}: "${NAME}"}}
roleRef: {apiGroup: rbac.authorization.k8s.io, kind: ClusterRole, name: "system:auth-delegator"}
subjects: [{kind: ServiceAccount, name: vault-reviewer, namespace: vault-auth}]
---
apiVersion: v1
kind: Secret
metadata:
  name: vault-reviewer-token
  namespace: vault-auth
  labels: {${K8S_LABEL}: "${NAME}"}
  annotations: {kubernetes.io/service-account.name: vault-reviewer}
type: kubernetes.io/service-account-token
YAML
for _ in $(seq 1 30); do
  [[ -n "$(kubectl --context "${CTX}" -n vault-auth get secret vault-reviewer-token -o jsonpath='{.data.token}')" ]] && break
  sleep 1
done
host_tmp="$(mktemp -d "${STATE}/.tmp.XXXXXX")"
container_tmp="$(docker exec "${NAME}" mktemp -d /tmp/wv.XXXXXX)"
kubectl --context "${CTX}" -n vault-auth get secret vault-reviewer-token -o jsonpath='{.data.token}' | base64 -d > "${host_tmp}/reviewer-jwt"
[[ -s "${host_tmp}/reviewer-jwt" ]] || { echo "reviewer token is empty" >&2; exit 1; }
kubectl --context "${CTX}" -n vault-auth get secret vault-reviewer-token -o jsonpath='{.data.ca\.crt}' | base64 -d > "${host_tmp}/k8s-ca.crt"
[[ -s "${host_tmp}/k8s-ca.crt" ]] || { echo "cluster CA is empty" >&2; exit 1; }
docker cp "${host_tmp}/reviewer-jwt" "${NAME}:${container_tmp}/reviewer-jwt"
docker cp "${host_tmp}/k8s-ca.crt" "${NAME}:${container_tmp}/k8s-ca.crt"
docker exec -e "CLUSTER=${CLUSTER}" -e "TMP=${container_tmp}" "${NAME}" sh -ec '
  export VAULT_TOKEN="$(cat /vault/bootstrap/root-token)"
  vault auth list -format=json | grep -q "\"kubernetes/\"" || vault auth enable kubernetes >/dev/null
  vault write auth/kubernetes/config \
    kubernetes_host="https://${CLUSTER}-control-plane:6443" \
    kubernetes_ca_cert=@"${TMP}/k8s-ca.crt" token_reviewer_jwt="$(cat "${TMP}/reviewer-jwt")" >/dev/null'
echo "workload vault ${NAME} ready; token file ${STATE}/token"
