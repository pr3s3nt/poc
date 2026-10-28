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
variable "namespace" { type = string }
variable "image" {
  type = string
  validation {
    condition     = can(regex("@sha256:[0-9a-f]{64}$", var.image))
    error_message = "image must be digest pinned."
  }
}
variable "database" { type = string }
variable "username" { type = string }
# Secret is created by approved platform delivery. Terraform never reads its data.
variable "auth_secret_name" { type = string }
variable "root_password_key" { type = string }
variable "user_password_key" { type = string }
variable "storage" {
  type    = string
  default = "50Gi"
}
variable "storage_class" {
  type    = string
  default = ""
}
variable "tags" { type = map(string) }
locals {
  name = "mysql-${var.tags["orchestrator-instance"]}"
  labels = {
    "app.kubernetes.io/managed-by" = "orchestrator"
    "app.kubernetes.io/instance"   = local.name
  }
}
resource "kubernetes_persistent_volume_claim_v1" "data" {
  wait_until_bound = false
  metadata {
    name      = "${local.name}-data"
    namespace = var.namespace
    labels    = local.labels
  }
  spec {
    access_modes       = ["ReadWriteOnce"]
    storage_class_name = var.storage_class == "" ? null : var.storage_class
    resources { requests = { storage = var.storage } }
  }
}
resource "kubernetes_service_v1" "this" {
  metadata {
    name      = local.name
    namespace = var.namespace
    labels    = local.labels
  }
  spec {
    cluster_ip = "None"
    selector   = local.labels
    port {
      name        = "mysql"
      port        = 3306
      target_port = 3306
    }
  }
}
resource "kubernetes_stateful_set_v1" "this" {
  metadata {
    name      = local.name
    namespace = var.namespace
    labels    = local.labels
  }
  spec {
    service_name = kubernetes_service_v1.this.metadata[0].name
    replicas     = 1
    selector { match_labels = local.labels }
    template {
      metadata { labels = local.labels }
      spec {
        container {
          name  = "mysql"
          image = var.image
          port { container_port = 3306 }
          env {
            name  = "MYSQL_DATABASE"
            value = var.database
          }
          env {
            name  = "MYSQL_USER"
            value = var.username
          }
          env {
            name = "MYSQL_ROOT_PASSWORD"
            value_from {
              secret_key_ref {
                name = var.auth_secret_name
                key  = var.root_password_key
              }
            }
          }
          env {
            name = "MYSQL_PASSWORD"
            value_from {
              secret_key_ref {
                name = var.auth_secret_name
                key  = var.user_password_key
              }
            }
          }
          volume_mount {
            name       = "data"
            mount_path = "/var/lib/mysql"
          }
          resources {
            requests = { cpu = "100m", memory = "256Mi" }
            limits   = { cpu = "1", memory = "1Gi" }
          }
          readiness_probe {
            exec { command = ["sh", "-ec", "MYSQL_PWD=\"$MYSQL_ROOT_PASSWORD\" mysql -h 127.0.0.1 -u root -Nse 'SELECT 1'"] }
            period_seconds    = 5
            timeout_seconds   = 3
            failure_threshold = 60
          }
          liveness_probe {
            tcp_socket { port = 3306 }
            initial_delay_seconds = 120
            period_seconds        = 10
          }
        }
        volume {
          name = "data"
          persistent_volume_claim { claim_name = kubernetes_persistent_volume_claim_v1.data.metadata[0].name }
        }
      }
    }
  }
}
output "host" { value = "${kubernetes_service_v1.this.metadata[0].name}.${var.namespace}.svc" }
output "port" { value = 3306 }
output "database" { value = var.database }
output "username" { value = var.username }
output "claimName" { value = kubernetes_persistent_volume_claim_v1.data.metadata[0].name }
output "namespace" { value = var.namespace }
