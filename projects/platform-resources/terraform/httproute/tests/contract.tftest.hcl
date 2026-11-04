mock_provider "kubernetes" {}
variables {
  namespace            = "app-example-staging"
  gateway_name         = "company-gateway"
  gateway_namespace    = "networking"
  gateway_section_name = "https"
  hostname             = "app.staging.company.example"
  service              = "frontend"
  port                 = 8080
  path                 = "/"
  tags                 = { orchestrator-instance = "1234567890abcdef" }
}
run "existing_gateway_route" {
  command = plan
  assert {
    condition     = kubernetes_manifest.this.manifest.kind == "HTTPRoute" && kubernetes_manifest.this.manifest.apiVersion == "gateway.networking.k8s.io/v1" && kubernetes_manifest.this.manifest.metadata.namespace == var.namespace && kubernetes_manifest.this.manifest.metadata.labels["orchestrator-instance"] == var.tags["orchestrator-instance"]
    error_message = "Only an owned namespaced v1 HTTPRoute is planned."
  }
  assert {
    condition     = kubernetes_manifest.this.manifest.spec.parentRefs[0].name == var.gateway_name && kubernetes_manifest.this.manifest.spec.parentRefs[0].namespace == var.gateway_namespace && kubernetes_manifest.this.manifest.spec.parentRefs[0].sectionName == "https" && kubernetes_manifest.this.manifest.spec.hostnames[0] == var.hostname
    error_message = "The route must attach to the supplied existing Gateway listener."
  }
  assert {
    condition     = kubernetes_manifest.this.manifest.spec.rules[0].backendRefs[0].name == var.service && kubernetes_manifest.this.manifest.spec.rules[0].backendRefs[0].port == 8080 && kubernetes_manifest.this.manifest.spec.rules[0].matches[0].path.type == "PathPrefix" && output.hostname == var.hostname
    error_message = "Hostname and Service routing must match input."
  }
}
run "optional_listener" {
  command = plan
  variables { gateway_section_name = "" }
  assert {
    condition     = !contains(keys(kubernetes_manifest.this.manifest.spec.parentRefs[0]), "sectionName")
    error_message = "An unspecified listener must be omitted rather than empty."
  }
}
run "invalid_port" {
  command = plan
  variables { port = 70000 }
  expect_failures = [var.port]
}
