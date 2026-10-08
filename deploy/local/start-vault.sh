#!/bin/sh
# Personal-machine setup: bootstrap material is kept outside the backend volume.
set -eu
umask 077
mkdir -p /vault/data /vault/bootstrap /connection-token /app-token
chmod 700 /vault/data /vault/bootstrap
chmod 755 /connection-token /app-token
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
vault policy write orchestrator-applications /local/applications-policy.hcl >/dev/null

# ensure_token DIR POLICY renews DIR/token or, when it is absent, expired or
# revoked, atomically replaces it with a new orphan periodic token that is
# readable only by the backend UID. Token values are never printed.
renew_token() {
  [ -s "$1/token" ] || return 1
  VAULT_TOKEN="$(cat "$1/token")" vault token renew >/dev/null 2>&1
}
ensure_token() {
  renew_token "$1" && return 0
  vault token create -orphan -no-default-policy -policy="$2" \
    -period=768h -field=token > "$1/token.new"
  chown 65532:65532 "$1/token.new"
  chmod 400 "$1/token.new"
  mv "$1/token.new" "$1/token"
}
ensure_token /connection-token orchestrator-connections
ensure_token /app-token orchestrator-applications

# Prove the application token can write, read and delete a value before the
# backend is allowed to start; a failure keeps Vault unhealthy (fail closed).
check_application_token() {
  probe="orchestrator/apps/_bootstrap/envs/local/values/probe-$$"
  (
    export VAULT_TOKEN="$(cat /app-token/token)"
    ok=1
    vault kv put -mount=kv "$probe" probe=ok >/dev/null 2>&1 \
      && [ "$(vault kv get -mount=kv -field=probe "$probe" 2>/dev/null)" = ok ] || ok=0
    vault kv metadata delete -mount=kv "$probe" >/dev/null 2>&1 || ok=0
    [ "$ok" = 1 ]
  )
}
if ! check_application_token; then
  echo 'Application token cannot write, read and delete kv/orchestrator/apps values; verify the vault-bootstrap volume matches vault-data and check applications-policy.hcl' >&2
  exit 1
fi
touch /tmp/vault-ready
echo 'Vault ready; Connection credential store and application secret store configured'

# Keep the scoped tokens valid, including when Vault restarts independently.
while kill -0 "$server_pid" 2>/dev/null; do
  sleep 60 &
  wait $! || true
  if ! renew_token /connection-token || ! renew_token /app-token; then
    echo 'Scoped token renewal failed; restarting Vault bootstrap' >&2
    exit 1
  fi
done
exit 1
