mock_provider "kubernetes" {}
variables {
  namespace              = "company-staging"
  image                  = "registry.company.example:5000/platform/mariadb@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
  database               = "appdb"
  username               = "app"
  auth_secret_name       = "mariadb-auth"
  root_password_key      = "root-password"
  user_password_key      = "user-password"
  image_pull_secret_name = "company-registry"
  tags                   = { orchestrator-instance = "1234567890abcdef" }
}
run "company_database_plan" {
  command = plan
  assert {
    condition     = kubernetes_stateful_set_v1.this.spec[0].template[0].spec[0].container[0].image == var.image && kubernetes_stateful_set_v1.this.spec[0].template[0].spec[0].container[0].env[2].value_from[0].secret_key_ref[0].name == var.auth_secret_name && kubernetes_stateful_set_v1.this.spec[0].template[0].spec[0].image_pull_secrets[0].name == "company-registry"
    error_message = "Company image, pull Secret and auth Secret must reach the pod."
  }
  assert {
    condition     = kubernetes_persistent_volume_claim_v1.data.metadata[0].labels["orchestrator-instance"] == var.tags["orchestrator-instance"] && output.port == 3306 && output.namespace == var.namespace && kubernetes_persistent_volume_claim_v1.data.spec[0].resources[0].requests.storage == "10Gi" && kubernetes_stateful_set_v1.this.wait_for_rollout == false
    error_message = "Outputs, PVC and executor-owned readiness must match the contract."
  }
}
run "reject_mutable_image" {
  command = plan
  variables { image = "mariadb:latest" }
  expect_failures = [var.image]
}
