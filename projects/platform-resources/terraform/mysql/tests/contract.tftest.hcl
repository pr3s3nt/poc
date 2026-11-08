mock_provider "kubernetes" {}
variables {
  namespace              = "company-staging"
  image                  = "registry.company.example:5000/platform/mysql@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
  database               = "appdb"
  username               = "app"
  password               = "p@ss:w/rd%1"
  master_password        = "root-secret"
  image_pull_secret_name = "company-registry"
  tags                   = { orchestrator-instance = "1234567890abcdef" }
}
run "company_database_plan" {
  command = plan
  assert {
    condition     = kubernetes_stateful_set_v1.this.spec[0].template[0].spec[0].container[0].image == var.image && kubernetes_stateful_set_v1.this.spec[0].template[0].spec[0].container[0].env[2].value_from[0].secret_key_ref[0].name == kubernetes_secret_v1.auth.metadata[0].name && kubernetes_stateful_set_v1.this.spec[0].template[0].spec[0].image_pull_secrets[0].name == "company-registry"
    error_message = "Company image, pull Secret and instance auth Secret must reach the pod."
  }
  assert {
    condition     = kubernetes_persistent_volume_claim_v1.data.metadata[0].labels["orchestrator-instance"] == var.tags["orchestrator-instance"] && output.port == 3306 && output.namespace == var.namespace && kubernetes_persistent_volume_claim_v1.data.spec[0].resources[0].requests.storage == "10Gi" && kubernetes_stateful_set_v1.this.wait_for_rollout == false
    error_message = "Outputs, PVC and executor-owned readiness must match the contract."
  }
}
run "reject_mutable_image" {
  command = plan
  variables { image = "mysql:latest" }
  expect_failures = [var.image]
}

run "connection_outputs" {
  command = plan
  assert {
    condition     = output.uri == "mysql://app:p%40ss%3Aw%2Frd%251@mysql-1234567890abcdef.company-staging.svc:3306/appdb" && output.jdbcUrl == "jdbc:mysql://mysql-1234567890abcdef.company-staging.svc:3306/appdb" && output.password == var.password
    error_message = "uri must percent-encode credentials and jdbcUrl must carry no credentials."
  }
  assert {
    condition     = kubernetes_secret_v1.auth.data["user-password"] == var.password && kubernetes_secret_v1.auth.data["root-password"] == var.master_password
    error_message = "The instance Secret must hold the executor-owned passwords."
  }
}
