path "auth/token/renew-self" {
  capabilities = ["update"]
}

path "kv/data/orchestrator/connections/+/+/credentials/+" {
  capabilities = ["create", "read", "update"]
}

path "kv/metadata/orchestrator/connections/+/+/credentials/+" {
  capabilities = ["delete"]
}
