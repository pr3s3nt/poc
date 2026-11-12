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
  type    = string
  default = "app"
  validation {
    condition     = can(regex("^[A-Za-z_][A-Za-z0-9_]{0,31}$", var.username)) && !contains(["root", "mysql", "mariadb.sys"], lower(var.username))
    error_message = "username must be a non-root application identifier."
  }
}
# Executor-owned: stable passwords drawn once per instance and kept in the
# platform secret store (issue #52). password is the application user's,
# master_password the admin account's; admin is never an output.
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
  name = "mongodb-${substr(var.tags["orchestrator-instance"], 0, 40)}"
  selector = {
    "app.kubernetes.io/name"     = "mongodb"
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
      name        = "mongodb"
      port        = 27017
      target_port = 27017
    }
  }
}
# Only code, no credentials. Environment supplies secret values inside the Pod.
resource "kubernetes_config_map_v1" "init" {
  metadata {
    name        = "${local.name}-init"
    namespace   = var.namespace
    labels      = local.labels
    annotations = var.tags
  }
  data = {
    "10-app-user.js" = <<-JS
      const appDb = db.getSiblingDB(process.env.MONGO_INITDB_DATABASE);
      appDb.createUser({user: process.env.MONGO_APP_USERNAME, pwd: process.env.MONGO_APP_PASSWORD, roles: [{role: "readWrite", db: process.env.MONGO_INITDB_DATABASE}]});
    JS
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
          name              = "mongodb"
          image             = var.image
          image_pull_policy = "IfNotPresent"
          port { container_port = 27017 }
          env {
            name  = "MONGO_INITDB_ROOT_USERNAME"
            value = var.root_username
          }
          env {
            name = "MONGO_INITDB_ROOT_PASSWORD"
            value_from {
              secret_key_ref {
                name = kubernetes_secret_v1.auth.metadata[0].name
                key  = "root-password"
              }
            }
          }
          env {
            name  = "MONGO_INITDB_DATABASE"
            value = var.database
          }
          env {
            name  = "MONGO_APP_USERNAME"
            value = var.username
          }
          env {
            name = "MONGO_APP_PASSWORD"
            value_from {
              secret_key_ref {
                name = kubernetes_secret_v1.auth.metadata[0].name
                key  = "user-password"
              }
            }
          }
          volume_mount {
            name       = "data"
            mount_path = "/data/db"
          }
          volume_mount {
            name       = "init"
            mount_path = "/docker-entrypoint-initdb.d"
            read_only  = true
          }
          volume_mount {
            name       = "configdb"
            mount_path = "/data/configdb"
          }
          resources {
            requests = { cpu = "100m", memory = "256Mi" }
            limits   = { cpu = "1", memory = "1Gi" }
          }
          startup_probe {
            exec { command = ["mongosh", "--quiet", "--host", "127.0.0.1", "--eval", "const a=db.getSiblingDB(process.env.MONGO_INITDB_DATABASE); if (!a.auth(process.env.MONGO_APP_USERNAME,process.env.MONGO_APP_PASSWORD)) quit(1); quit(a.runCommand({ping:1}).ok===1?0:1);"] }
            period_seconds    = 5
            timeout_seconds   = 5
            failure_threshold = 120
          }
          readiness_probe {
            exec { command = ["mongosh", "--quiet", "--host", "127.0.0.1", "--eval", "const a=db.getSiblingDB(process.env.MONGO_INITDB_DATABASE); if (!a.auth(process.env.MONGO_APP_USERNAME,process.env.MONGO_APP_PASSWORD)) quit(1); quit(a.runCommand({ping:1}).ok===1?0:1);"] }
            period_seconds    = 5
            timeout_seconds   = 5
            failure_threshold = 3
          }
          liveness_probe {
            tcp_socket { port = 27017 }
            period_seconds    = 10
            timeout_seconds   = 3
            failure_threshold = 6
          }
        }
        volume {
          name = "data"
          persistent_volume_claim { claim_name = kubernetes_persistent_volume_claim_v1.data.metadata[0].name }
        }
        volume {
          name = "init"
          config_map { name = kubernetes_config_map_v1.init.metadata[0].name }
        }
        volume {
          name = "configdb"
          empty_dir {}
        }
      }
    }
  }
}
output "host" { value = "${kubernetes_service_v1.this.metadata[0].name}.${var.namespace}.svc" }
output "port" { value = 27017 }
output "database" { value = var.database }
output "username" { value = var.username }

output "password" {
  value     = var.password
  sensitive = true
}

# Connection string (issue #52), the Humanitec mongodb `connection` output.
# The app user lives in the application database, hence authSource.
output "connection" {
  value     = "mongodb://${urlencode(var.username)}:${urlencode(var.password)}@${kubernetes_service_v1.this.metadata[0].name}.${var.namespace}.svc:27017/${urlencode(var.database)}?authSource=${urlencode(var.database)}"
  sensitive = true
}

variable "root_username" {
  type    = string
  default = "admin"
  validation {
    condition     = can(regex("^[A-Za-z_][A-Za-z0-9_]{0,31}$", var.root_username))
    error_message = "root_username must be a valid identifier."
  }
}
