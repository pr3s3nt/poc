# Platform Vault store token (bootstrap input for the ordinary platform-vault
# store, ADR-012): application values,
# workload bundles and workload role/policy management only. It has no access
# to Connection credentials, other mounts or the root token.
path "auth/token/lookup-self" {
  capabilities = ["read"]
}

path "auth/token/renew-self" {
  capabilities = ["update"]
}

path "sys/internal/ui/mounts/kv" {
  capabilities = ["read"]
}

path "sys/capabilities-self" {
  capabilities = ["update"]
}

path "kv/data/orchestrator/apps/*" {
  capabilities = ["create", "read", "update"]
}

path "kv/metadata/orchestrator/apps/*" {
  capabilities = ["read", "list", "delete"]
}

path "sys/policies/acl/orch-*" {
  capabilities = ["create", "read", "update"]
}

path "auth/kubernetes/role/orch-*" {
  capabilities = ["create", "read", "update"]
}

path "auth/kubernetes/config" {
  capabilities = ["read"]
}
