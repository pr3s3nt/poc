# Real Kubernetes resources for the UC-08 BR-10 browser acceptance. Every
# instance owns its objects through its Terraform state; no API is stubbed.
terraform {
  required_version = ">= 1.5"
  required_providers {
    kubernetes = {
      source  = "hashicorp/kubernetes"
      version = ">= 2.38.0, < 3.0.0"
    }
  }
}

provider "kubernetes" {}

variable "namespace" {
  type = string
}

variable "revision" {
  type    = string
  default = "v1"
}

variable "tags" {
  type = map(string)
}

locals {
  name = "runtime-${var.tags["orchestrator-instance"]}"
  labels = {
    "app.kubernetes.io/managed-by" = "orchestrator"
    "orchestrator-instance"        = var.tags["orchestrator-instance"]
  }
}

resource "kubernetes_config_map_v1" "runtime" {
  metadata {
    name      = local.name
    namespace = var.namespace
    labels    = local.labels
  }
  data = {
    config   = "acceptance-config-ok"
    revision = var.revision
  }
}

resource "kubernetes_service_account_v1" "runtime" {
  metadata {
    name      = local.name
    namespace = var.namespace
    labels    = local.labels
  }
  automount_service_account_token = false
  wait_for_service_account_token  = false
}

output "config" {
  value = kubernetes_config_map_v1.runtime.data["config"]
}

output "revision" {
  value = kubernetes_config_map_v1.runtime.data["revision"]
}

output "name" {
  value = kubernetes_service_account_v1.runtime.metadata[0].name
}
