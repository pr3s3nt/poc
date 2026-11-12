mock_provider "kubernetes" {}
variables {
  namespace              = "company-staging"
  image                  = "registry.company.example:5000/platform/mongodb@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
  database               = "appdb"
  password               = "p@ss:w/rd%1"
  master_password        = "root-secret"
  image_pull_secret_name = "company-registry"
  tags                   = { orchestrator-instance = "1234567890abcdef" }
}
run "company_database_plan" {
  command = plan
  assert {
    condition     = kubernetes_stateful_set_v1.this.spec[0].template[0].spec[0].container[0].image == var.image && kubernetes_stateful_set_v1.this.spec[0].template[0].spec[0].container[0].env[1].value_from[0].secret_key_ref[0].name == kubernetes_secret_v1.auth.metadata[0].name && kubernetes_stateful_set_v1.this.spec[0].template[0].spec[0].image_pull_secrets[0].name == "company-registry"
    error_message = "Company image, pull Secret and instance auth Secret must reach the pod."
  }
  assert {
    condition     = kubernetes_persistent_volume_claim_v1.data.metadata[0].labels["orchestrator-instance"] == var.tags["orchestrator-instance"] && output.port == 27017 && output.username == "app" && kubernetes_persistent_volume_claim_v1.data.metadata[0].namespace == var.namespace && kubernetes_persistent_volume_claim_v1.data.spec[0].resources[0].requests.storage == "10Gi" && kubernetes_stateful_set_v1.this.wait_for_rollout == false
    error_message = "Outputs, PVC and executor-owned readiness must match the contract."
  }

  assert {
    condition     = strcontains(kubernetes_config_map_v1.init.data["10-app-user.js"], "readWrite") && strcontains(kubernetes_config_map_v1.init.data["10-app-user.js"], "process.env.MONGO_APP_PASSWORD")
    error_message = "Mongo must initialize an app-scoped user from environment, without embedding secret values."
  }
}
run "reject_mutable_image" {
  command = plan
  variables { image = "mongodb:latest" }
  expect_failures = [var.image]
}

run "connection_outputs" {
  command = plan
  assert {
    condition     = output.connection == "mongodb://app:p%40ss%3Aw%2Frd%251@mongodb-1234567890abcdef.company-staging.svc:27017/appdb?authSource=appdb" && output.password == var.password
    error_message = "connection must percent-encode credentials and authenticate against the app database."
  }
  assert {
    condition     = kubernetes_secret_v1.auth.data["user-password"] == var.password && kubernetes_secret_v1.auth.data["root-password"] == var.master_password
    error_message = "The instance Secret must hold the executor-owned passwords."
  }
}
