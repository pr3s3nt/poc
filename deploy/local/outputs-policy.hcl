path "auth/token/renew-self" {
  capabilities = ["update"]
}

path "kv/data/orchestrator/outputs/*" {
  capabilities = ["create", "read"]
}
