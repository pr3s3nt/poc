#!/usr/bin/env bash
# Enables and configures Kubernetes auth (mount "kubernetes") on the Console
# Vault in orchestrator-dev-3070, using the reviewer from
# 04-vault-kubernetes-auth.yaml. Idempotent: rewrites the same config.
# Prints no token, CA or root material. Requires KUBECONFIG for kind-k8s-4f.
set -euo pipefail
K=(kubectl --context kind-k8s-4f -n orchestrator-dev-3070)
"${K[@]}" get serviceaccount vault-token-reviewer >/dev/null
jwt=$("${K[@]}" get secret vault-token-reviewer -o jsonpath='{.data.token}' | base64 -d)
ca=$("${K[@]}" get secret vault-token-reviewer -o jsonpath='{.data.ca\.crt}' | base64 -d)
[[ -n "$jwt" && "$ca" == *"BEGIN CERTIFICATE"* ]] || { echo "reviewer token Secret is not populated" >&2; exit 1; }
# Token and CA travel over stdin into a private temp dir that is removed on exit.
printf '%s\n%s\n' "$jwt" "$ca" | "${K[@]}" exec -i deploy/vault -- sh -c '
set -eu
umask 077
dir=$(mktemp -d)
trap "rm -rf \"$dir\"" EXIT
IFS= read -r line; printf "%s" "$line" > "$dir/jwt"  # read: no read-ahead past the first line
cat > "$dir/ca.crt"
export VAULT_ADDR=http://127.0.0.1:8200
VAULT_TOKEN=$(cat /vault/bootstrap/root-token)
export VAULT_TOKEN
vault auth list -format=json | grep -q "\"kubernetes/\"" || vault auth enable -path=kubernetes kubernetes >/dev/null
vault write auth/kubernetes/config \
  kubernetes_host=https://kubernetes.default.svc:443 \
  kubernetes_ca_cert=@"$dir/ca.crt" \
  token_reviewer_jwt=@"$dir/jwt" \
  disable_local_ca_jwt=true >/dev/null
vault read -format=json auth/kubernetes/config | grep -q "\"kubernetes_host\": \"https://kubernetes.default.svc:443\""
echo "kubernetes auth configured"
'
