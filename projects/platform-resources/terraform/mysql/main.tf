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
variable "image" {
  type = string
  validation {
    condition     = can(regex("^[a-z0-9][a-z0-9._:/-]*(:[A-Za-z0-9._-]{1,128})?@sha256:[0-9a-f]{64}$", var.image))
    error_message = "image must be a digest-pinned registry reference."
  }
}
variable "database" {
  type = string
  validation {
    condition     = can(regex("^[A-Za-z_][A-Za-z0-9_]{0,62}$", var.database)) && !contains(["admin", "local", "config", "mysql", "sys", "performance_schema", "information_schema"], lower(var.database))
    error_message = "database must be a non-system database identifier."
  }
}
variable "username" {
  type = string
  validation {
    condition     = can(regex("^[A-Za-z_][A-Za-z0-9_]{0,31}$", var.username)) && !contains(["root", "mysql", "mariadb.sys"], lower(var.username))
    error_message = "username must be a non-root application identifier."
  }
}
# Executor-owned: stable passwords drawn once per instance and kept in the
# platform secret store (issue #52). password is the application user's,
# master_password the root account's; root is never an output.
variable "password" {
  type      = string
  sensitive = true
}
variable "master_password" {
  type      = string
  sensitive = true
}
variable "storage" {
  type    = string
  default = "10Gi"
  validation {
    condition     = can(regex("^[1-9][0-9]{0,3}(Mi|Gi)$", var.storage))
    error_message = "storage must be whole Mi or Gi."
  }
}
variable "storage_class" {
  type    = string
  default = ""
}
variable "image_pull_secret_name" {
  type    = string
  default = ""
}
variable "tags" {
  type = map(string)
  validation {
    condition     = can(regex("^[0-9a-f]{8,64}$", lookup(var.tags, "orchestrator-instance", "")))
    error_message = "tags require the stable orchestrator-instance identity."
  }
}
locals {
  name = "mysql-${substr(var.tags["orchestrator-instance"], 0, 40)}"
  selector = {
    "app.kubernetes.io/name"     = "mysql"
    "app.kubernetes.io/instance" = local.name
  }
  platform_labels = {
    for k, v in var.tags : k => v
    if can(regex("^[A-Za-z0-9]([-A-Za-z0-9_.]{0,61}[A-Za-z0-9])?$", k)) && can(regex("^([A-Za-z0-9]([-A-Za-z0-9_.]{0,61}[A-Za-z0-9])?)?$", v))
  }
  labels = merge(local.platform_labels, local.selector, {
    "app.kubernetes.io/managed-by" = "orchestrator"
  })
}

# Credentials for the pod only; consumers receive the password through the
# orchestrator's secret-store output, never from this object's metadata.
resource "kubernetes_secret_v1" "auth" {
  metadata {
    name        = "${local.name}-auth"
    namespace   = var.namespace
    labels      = local.labels
    annotations = var.tags
  }
  data = {
    "root-password" = var.master_password
    "user-password" = var.password
  }
}

resource "kubernetes_persistent_volume_claim_v1" "data" {
  wait_until_bound = false
  metadata {
    name        = "${local.name}-data"
    namespace   = var.namespace
    labels      = local.labels
    annotations = var.tags
  }
  spec {
    access_modes       = ["ReadWriteOnce"]
    storage_class_name = var.storage_class == "" ? null : var.storage_class
    resources { requests = { storage = var.storage } }
  }
}
resource "kubernetes_service_v1" "this" {
  metadata {
    name        = local.name
    namespace   = var.namespace
    labels      = local.labels
    annotations = var.tags
  }
  spec {
    cluster_ip = "None"
    selector   = local.selector
    port {
      name        = "mysql"
      port        = 3306
      target_port = 3306
    }
  }
}

resource "kubernetes_stateful_set_v1" "this" {
  wait_for_rollout = false
  metadata {
    name        = local.name
    namespace   = var.namespace
    labels      = local.labels
    annotations = var.tags
  }
  spec {
    service_name = kubernetes_service_v1.this.metadata[0].name
    replicas     = 1
    selector { match_labels = local.selector }
    template {
      metadata { labels = local.labels }
      spec {
        automount_service_account_token  = false
        termination_grace_period_seconds = 60
        dynamic "image_pull_secrets" {
          for_each = var.image_pull_secret_name == "" ? [] : [var.image_pull_secret_name]
          content { name = image_pull_secrets.value }
        }
        container {
          name              = "mysql"
          image             = var.image
          image_pull_policy = "IfNotPresent"
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
                name = kubernetes_secret_v1.auth.metadata[0].name
                key  = "root-password"
              }
            }
          }
          env {
            name = "MYSQL_PASSWORD"
            value_from {
              secret_key_ref {
                name = kubernetes_secret_v1.auth.metadata[0].name
                key  = "user-password"
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
          startup_probe {
            exec { command = ["sh", "-ec", "MYSQL_PWD=\"$MYSQL_PASSWORD\" mysql --protocol=TCP -h 127.0.0.1 -u \"$MYSQL_USER\" \"$MYSQL_DATABASE\" -Nse \"SELECT 1\" >/dev/null"] }
            period_seconds    = 5
            timeout_seconds   = 5
            failure_threshold = 120
          }
          readiness_probe {
            exec { command = ["sh", "-ec", "MYSQL_PWD=\"$MYSQL_PASSWORD\" mysql --protocol=TCP -h 127.0.0.1 -u \"$MYSQL_USER\" \"$MYSQL_DATABASE\" -Nse \"SELECT 1\" >/dev/null"] }
            period_seconds    = 5
            timeout_seconds   = 5
            failure_threshold = 3
          }
          liveness_probe {
            tcp_socket { port = 3306 }
            period_seconds    = 10
            timeout_seconds   = 3
            failure_threshold = 6
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

output "password" {
  value     = var.password
  sensitive = true
}

# Connection strings (issue #52). Credentials are percent-encoded; uri carries
# the password and is therefore sensitive. jdbcUrl has no credentials.
output "uri" {
  value     = "mysql://${urlencode(var.username)}:${urlencode(var.password)}@${kubernetes_service_v1.this.metadata[0].name}.${var.namespace}.svc:3306/${urlencode(var.database)}"
  sensitive = true
}

output "jdbcUrl" { value = "jdbc:mysql://${kubernetes_service_v1.this.metadata[0].name}.${var.namespace}.svc:3306/${var.database}" }
