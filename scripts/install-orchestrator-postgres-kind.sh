#!/usr/bin/env bash
# Install a dedicated persistent PostgreSQL instance for Orchestrator on kind.
set -euo pipefail

script_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cluster_context="kind-idp-internal"
system_namespace="orchestrator-system"
credential_dir="${XDG_DATA_HOME:-${HOME}/.local/share}/orchestrator-kind-postgres"
credential_file="${credential_dir}/password"

if [[ "$(kubectl config current-context)" != "${cluster_context}" ]]; then
  echo "Current Kubernetes context must be ${cluster_context}; nothing changed." >&2
  exit 1
fi
if kubectl --context "${cluster_context}" get namespace "${system_namespace}" >/dev/null 2>&1; then
  actual_owner="$(kubectl --context "${cluster_context}" get namespace "${system_namespace}" -o jsonpath='{.metadata.labels.app\.kubernetes\.io/part-of}')"
  if [[ "${actual_owner}" != "orchestrator" ]]; then
    echo "Namespace ${system_namespace} exists without the expected owner label; nothing changed." >&2
    exit 1
  fi
else
  kubectl --context "${cluster_context}" create namespace "${system_namespace}"
  kubectl --context "${cluster_context}" label namespace "${system_namespace}" app.kubernetes.io/part-of=orchestrator
fi

if ! kubectl --context "${cluster_context}" -n "${system_namespace}" get secret postgres-auth >/dev/null 2>&1; then
  if [[ -e "${credential_file}" ]]; then
    echo "Credential file exists but Kubernetes Secret does not; refusing to overwrite or reuse it automatically." >&2
    exit 1
  fi
  install -d -m 0700 "${credential_dir}"
  umask 077
  openssl rand -hex 32 > "${credential_file}"
  kubectl --context "${cluster_context}" -n "${system_namespace}" create secret generic postgres-auth --from-file="password=${credential_file}"
elif [[ ! -s "${credential_file}" ]]; then
  echo "Kubernetes Secret exists but local credential file is missing; refusing to rotate it." >&2
  exit 1
fi

kubectl --context "${cluster_context}" apply -f "${script_root}/deploy/kind/orchestrator-postgres.yaml"
kubectl --context "${cluster_context}" -n "${system_namespace}" rollout status statefulset/postgres --timeout=300s
kubectl --context "${cluster_context}" -n "${system_namespace}" get pvc -l app.kubernetes.io/name=orchestrator-postgres -o name
echo "Orchestrator PostgreSQL is ready in ${system_namespace}; credential file: ${credential_file}"
