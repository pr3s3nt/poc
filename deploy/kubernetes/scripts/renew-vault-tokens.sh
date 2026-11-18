#!/bin/sh
set -eu
# Only scoped tokens. Never receives root/unseal authorization.
while :; do
  for name in connection-token output-token app-token; do
    VAULT_TOKEN=$(cat "/private/$name")
    export VAULT_TOKEN
    vault token renew >/dev/null 2>&1 || { echo 'Scoped Vault token renewal failed' >&2; exit 1; }
  done
  unset VAULT_TOKEN
  sleep 60
done
