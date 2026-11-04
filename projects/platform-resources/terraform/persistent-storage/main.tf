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
  validation {
    condition     = can(regex("^[a-z0-9]([-a-z0-9]{0,61}[a-z0-9])?$", var.namespace))
    error_message = "namespace must be a DNS label."
  }
}
variable "storage" {
  type    = string
  default = "2Gi"
  validation {
    condition     = can(regex("^[1-9][0-9]{0,5}(Mi|Gi|Ti)$", var.storage))
    error_message = "storage must be a positive integral binary quantity."
  }
}
variable "storage_class" {
  type    = string
  default = ""
}
variable "access_mode" {
  type    = string
  default = "ReadWriteOnce"
  validation {
    condition     = contains(["ReadWriteOnce", "ReadWriteOncePod", "ReadOnlyMany", "ReadWriteMany"], var.access_mode)
    error_message = "access_mode must be a Kubernetes persistent volume access mode."
  }
}
variable "tags" {
  type = map(string)
  validation {
    condition     = can(regex("^[a-z0-9]{16}$", var.tags["orchestrator-instance"]))
    error_message = "executor instance identity is required."
  }
}
locals {
  name = "data-${var.tags["orchestrator-instance"]}"
  labels = merge({
    for key, value in var.tags : key => value
    if can(regex("^[A-Za-z0-9]([-A-Za-z0-9_.]{0,61}[A-Za-z0-9])?$", key)) && can(regex("^([A-Za-z0-9]([-A-Za-z0-9_.]{0,61}[A-Za-z0-9])?)?$", value))
  }, { "app.kubernetes.io/managed-by" = "orchestrator" })
}
resource "kubernetes_persistent_volume_claim_v1" "data" {
  # A delayed-binding StorageClass needs a consumer before the claim can bind.
  wait_until_bound = false
  metadata {
    name      = local.name
    namespace = var.namespace
    labels    = local.labels
  }
  spec {
    access_modes       = [var.access_mode]
    storage_class_name = var.storage_class == "" ? null : var.storage_class
    volume_mode        = "Filesystem"
    resources {
      requests = { storage = var.storage }
    }
  }
}
output "claimName" { value = kubernetes_persistent_volume_claim_v1.data.metadata[0].name }
output "namespace" { value = kubernetes_persistent_volume_claim_v1.data.metadata[0].namespace }
