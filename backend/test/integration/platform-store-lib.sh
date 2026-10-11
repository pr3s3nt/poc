#!/usr/bin/env bash
# Ordinary-store setup shared by the kind verification scripts (ADR-012, T23).
#
# The backend has no flag-configured workload Vault. A script that needs the
# platform store registers it through the explicit opt-in bootstrap, which
# verifies the Vault, persists a private credential and stores only an opaque
# reference. The bootstrap needs the separate Connection credential store, so
# the operator supplies a scoped token for it; it is passed by path only.
#
#   ORCH_CONNECTION_VAULT_ADDR        Vault API address of the credential store
#   ORCH_CONNECTION_VAULT_TOKEN_FILE  owner-only file with its scoped token
#   ORCH_CONNECTION_VAULT_MOUNT       KV v2 mount (default kv)
#
# platform_store_flags <backend-address> <workload-address> <token-file>
# fills PLATFORM_STORE_FLAGS for the backend command line.
platform_store_flags() {
  local backend="$1" workload="$2" token_file="$3"
  if [[ -z "${ORCH_CONNECTION_VAULT_ADDR:-}" || ! -s "${ORCH_CONNECTION_VAULT_TOKEN_FILE:-}" ]]; then
    echo "set ORCH_CONNECTION_VAULT_ADDR and ORCH_CONNECTION_VAULT_TOKEN_FILE: the platform store is registered through the ordinary bootstrap, which needs the Connection credential store" >&2
    return 1
  fi
  PLATFORM_STORE_FLAGS=(
    -connection-credential-store vault -connection-vault-address "${ORCH_CONNECTION_VAULT_ADDR}"
    -connection-vault-token-file "${ORCH_CONNECTION_VAULT_TOKEN_FILE}" -connection-vault-mount "${ORCH_CONNECTION_VAULT_MOUNT:-kv}"
    -platform-vault-bootstrap -platform-vault-address "${backend}"
    -platform-vault-workload-address "${workload}" -platform-vault-token-file "${token_file}"
  )
}
