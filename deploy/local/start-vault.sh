#!/bin/sh
# Personal-machine setup: bootstrap material is kept outside the backend volume.
set -eu
umask 077
mkdir -p /vault/data /vault/bootstrap /connection-token
chmod 700 /vault/data /vault/bootstrap
chmod 755 /connection-token
rm -f /tmp/vault-ready

vault server -config=/local/vault.hcl &
server_pid=$!
stop() {
  rm -f /tmp/vault-ready
  kill "$server_pid" 2>/dev/null || true
  wait "$server_pid" 2>/dev/null || true
}
trap stop EXIT
trap 'exit 0' INT TERM

# status returns 2 for a reachable sealed Vault, 0 for an unsealed Vault.
attempt=0
while :; do
  status=0
  vault status >/dev/null 2>&1 || status=$?
  case "$status" in 0|2) break ;; esac
  attempt=$((attempt + 1))
  if [ "$attempt" -ge 60 ]; then
    echo 'Vault did not become reachable' >&2
    exit 1
  fi
  kill -0 "$server_pid"
  sleep 1
done

if ! vault operator init -status >/dev/null 2>&1; then
  # Retain the complete initialization response before extracting local files.
  vault operator init -key-shares=1 -key-threshold=1 -format=table > /vault/bootstrap/init.txt
  awk '/^Unseal Key 1:/ {print $4}' /vault/bootstrap/init.txt > /vault/bootstrap/unseal-key
  awk '/^Initial Root Token:/ {print $4}' /vault/bootstrap/init.txt > /vault/bootstrap/root-token
fi
if [ ! -s /vault/bootstrap/unseal-key ] || [ ! -s /vault/bootstrap/root-token ]; then
  echo 'Vault bootstrap files are missing; restore the matching bootstrap volume' >&2
  exit 1
fi
vault operator unseal "$(cat /vault/bootstrap/unseal-key)" >/dev/null
export VAULT_TOKEN="$(cat /vault/bootstrap/root-token)"

vault secrets list -format=json > /tmp/vault-mounts.json
if ! grep -q '"kv/"' /tmp/vault-mounts.json; then
  vault secrets enable -path=kv kv-v2 >/dev/null
fi
rm -f /tmp/vault-mounts.json
vault policy write orchestrator-connections /local/connections-policy.hcl >/dev/null

renew_connection_token() {
  [ -s /connection-token/token ] || return 1
  VAULT_TOKEN="$(cat /connection-token/token)" vault token renew >/dev/null 2>&1
}
if ! renew_connection_token; then
  vault token create -orphan -no-default-policy -policy=orchestrator-connections \
    -period=768h -field=token > /connection-token/token.new
  chown 65532:65532 /connection-token/token.new
  chmod 400 /connection-token/token.new
  mv /connection-token/token.new /connection-token/token
fi
touch /tmp/vault-ready
echo 'Vault ready; Connection credential store configured'

# Keep the scoped token valid, including when Vault restarts independently.
while kill -0 "$server_pid" 2>/dev/null; do
  sleep 60 &
  wait $! || true
  if ! renew_connection_token; then
    echo 'Connection token renewal failed; restarting Vault bootstrap' >&2
    exit 1
  fi
done
exit 1
