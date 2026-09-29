#!/bin/sh
# Orchestrator container entrypoint. Workload configuration arrives only as
# environment variables (UC-12 references), so this script writes credential
# values to owner-only files and maps ORCHESTRATOR_* settings to flags.
set -eu

runtime_dir=/tmp/orchestrator
mkdir -p "${runtime_dir}"
chmod 0700 "${runtime_dir}"

if [ -n "${ORCHESTRATOR_KUBECONFIG_B64:-}" ]; then
  (umask 077 && printf '%s' "${ORCHESTRATOR_KUBECONFIG_B64}" | base64 -d > "${runtime_dir}/kubeconfig")
  export KUBECONFIG="${runtime_dir}/kubeconfig"
fi
unset ORCHESTRATOR_KUBECONFIG_B64
if [ -n "${ORCHESTRATOR_VAULT_TOKEN:-}" ]; then
  (umask 077 && printf '%s' "${ORCHESTRATOR_VAULT_TOKEN}" > "${runtime_dir}/vault-token")
  export ORCHESTRATOR_VAULT_TOKEN_FILE="${runtime_dir}/vault-token"
fi
unset ORCHESTRATOR_VAULT_TOKEN
if [ -n "${ORCHESTRATOR_DATABASE_URL:-}" ]; then
  (umask 077 && printf '%s' "${ORCHESTRATOR_DATABASE_URL}" > "${runtime_dir}/database-url")
  export ORCHESTRATOR_DATABASE_URL_FILE="${runtime_dir}/database-url"
fi
unset ORCHESTRATOR_DATABASE_URL

set -- -addr "${ORCHESTRATOR_LISTEN_ADDR:-0.0.0.0:8080}" -ui-dir "" -adapters "${ORCHESTRATOR_ADAPTERS:-kubernetes}" "$@"
if [ -n "${ORCHESTRATOR_KUBE_CONTEXT:-}" ]; then set -- "$@" -kube-context "${ORCHESTRATOR_KUBE_CONTEXT}"; fi
if [ -n "${ORCHESTRATOR_CLUSTER:-}" ]; then set -- "$@" -cluster "${ORCHESTRATOR_CLUSTER}"; fi
if [ -n "${ORCHESTRATOR_VAULT_DELIVERY:-}" ]; then set -- "$@" -vault-delivery "${ORCHESTRATOR_VAULT_DELIVERY}"; fi
if [ -n "${ORCHESTRATOR_DATABASE_URL_FILE:-}" ]; then set -- "$@" -database-url-file "${ORCHESTRATOR_DATABASE_URL_FILE}"; fi
exec /usr/local/bin/orchestrator "$@"
