#!/bin/sh
# Dedicated workload Vault for one kind cluster (personal machine). Same
# pattern as deploy/local/start-vault.sh: initialize once, unseal on every
# start, enable KV v2 at kv/, write the backend policy and keep one orphan
# periodic token renewed in /token/token. Run with restart policy
# unless-stopped so a Docker/host restart unseals automatically. Root and
# unseal material stay in /vault/bootstrap and are never printed.
set -eu
umask 077
mkdir -p /vault/data /vault/bootstrap /token
chmod 700 /vault/data /vault/bootstrap
rm -f /tmp/vault-ready

vault server -config=/local/vault.hcl &
server_pid=$!
trap 'rm -f /tmp/vault-ready; kill "$server_pid" 2>/dev/null || true; wait "$server_pid" 2>/dev/null || true' EXIT
trap 'exit 0' INT TERM

attempt=0
while :; do
  status=0
  vault status >/dev/null 2>&1 || status=$?
  case "$status" in 0|2) break ;; esac
  attempt=$((attempt + 1))
  [ "$attempt" -ge 60 ] && { echo 'Vault did not become reachable' >&2; exit 1; }
  kill -0 "$server_pid"
  sleep 1
done

if ! vault operator init -status >/dev/null 2>&1; then
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
grep -q '"kv/"' /tmp/vault-mounts.json || vault secrets enable -path=kv kv-v2 >/dev/null
rm -f /tmp/vault-mounts.json
vault policy write orchestrator-applications /local/applications-policy.hcl >/dev/null

renew_token() {
  [ -s /token/token ] || return 1
  VAULT_TOKEN="$(cat /token/token)" vault token renew >/dev/null 2>&1
}
if ! renew_token; then
  vault token create -orphan -no-default-policy -policy=orchestrator-applications \
    -period=768h -field=token > /token/token.new
  chmod 440 /token/token.new
  mv /token/token.new /token/token
fi
[ -z "${TOKEN_OWNER:-}" ] || chown "$TOKEN_OWNER" /token/token

touch /tmp/vault-ready
echo 'Workload Vault ready (unsealed, kv/ enabled, scoped token renewed)'

while kill -0 "$server_pid" 2>/dev/null; do
  sleep 60 &
  wait $! || true
  if ! renew_token; then
    echo 'Scoped token renewal failed; restarting' >&2
    exit 1
  fi
done
exit 1
