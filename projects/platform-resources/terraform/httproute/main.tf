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
variable "gateway_name" {
  type = string
  validation {
    condition     = length(var.gateway_name) <= 253 && alltrue([for label in split(".", var.gateway_name) : can(regex("^[a-z0-9]([-a-z0-9]{0,61}[a-z0-9])?$", label))])
    error_message = "gateway_name must be an existing Gateway DNS name."
  }
}
variable "gateway_namespace" {
  type = string
  validation {
    condition     = can(regex("^[a-z0-9]([-a-z0-9]{0,61}[a-z0-9])?$", var.gateway_namespace))
    error_message = "gateway_namespace must be a DNS label."
  }
}
variable "gateway_section_name" {
  type    = string
  default = ""
  validation {
    condition     = var.gateway_section_name == "" || can(regex("^[a-z0-9]([-a-z0-9]{0,61}[a-z0-9])?$", var.gateway_section_name))
    error_message = "gateway_section_name must be an existing listener name, or empty."
  }
}
variable "hostname" {
  type = string
  validation {
    condition     = length(var.hostname) <= 253 && length(split(".", var.hostname)) >= 2 && alltrue([for label in split(".", var.hostname) : can(regex("^[a-z0-9]([-a-z0-9]{0,61}[a-z0-9])?$", label))])
    error_message = "hostname must be an exact lowercase DNS hostname without scheme/path/wildcards."
  }
}
variable "service" {
  type = string
  validation {
    condition     = can(regex("^[a-z]([-a-z0-9]{0,61}[a-z0-9])?$", var.service))
    error_message = "service must name a Service in the route namespace."
  }
}
variable "port" {
  type = number
  validation {
    condition     = var.port >= 1 && var.port <= 65535 && floor(var.port) == var.port
    error_message = "port must be an integer Service port from 1 to 65535."
  }
}
variable "path" {
  type    = string
  default = "/"
  validation {
    condition     = startswith(var.path, "/") && length(var.path) <= 1024 && !can(regex("[?#[:space:]]", var.path))
    error_message = "path must be a bounded absolute PathPrefix without query, fragment or whitespace."
  }
}
variable "tags" {
  type = map(string)
  validation {
    condition     = can(regex("^[0-9a-f]{8,64}$", lookup(var.tags, "orchestrator-instance", "")))
    error_message = "tags must carry the stable orchestrator-instance identity."
  }
}
locals {
  name = "http-${substr(var.tags["orchestrator-instance"], 0, 40)}"
  labels = merge({
    for key, value in var.tags : key => value
    if can(regex("^[A-Za-z0-9]([-A-Za-z0-9_.]{0,61}[A-Za-z0-9])?$", key)) && can(regex("^([A-Za-z0-9]([-A-Za-z0-9_.]{0,61}[A-Za-z0-9])?)?$", value))
  }, { "app.kubernetes.io/managed-by" = "orchestrator" })
  route = {
    apiVersion = "gateway.networking.k8s.io/v1"
    kind       = "HTTPRoute"
    metadata = {
      name        = local.name
      namespace   = var.namespace
      labels      = local.labels
      annotations = var.tags
    }
    spec = {
      parentRefs = [merge({
        group     = "gateway.networking.k8s.io"
        kind      = "Gateway"
        name      = var.gateway_name
        namespace = var.gateway_namespace
      }, var.gateway_section_name == "" ? {} : { sectionName = var.gateway_section_name })]
      hostnames = [var.hostname]
      rules = [{
        matches     = [{ path = { type = "PathPrefix", value = var.path } }]
        backendRefs = [{ group = "", kind = "Service", name = var.service, port = var.port }]
      }]
    }
  }
}
# Creates only this route. Gateway, listener, DNS and certificates stay external.
resource "kubernetes_manifest" "this" {
  manifest = local.route
}
output "name" { value = local.name }
output "namespace" { value = var.namespace }
output "hostname" { value = var.hostname }
