variable "expected_account_id" {
  type = string
  validation {
    condition     = can(regex("^[0-9]{12}$", var.expected_account_id))
    error_message = "Set the expected 12-digit AWS account."
  }
}
variable "region" { type = string }
variable "vpc_id" { type = string }
variable "subnet_id" { type = string }
variable "alb_subnet_ids" { type = list(string) }
variable "operator_cidrs" {
  type = set(string)
  validation {
    condition     = length(var.operator_cidrs) > 0 && alltrue([for c in var.operator_cidrs : can(cidrnetmask(c)) && try(tonumber(split("/", c)[1]) >= 24, false)])
    error_message = "Explicit operator IPv4 CIDRs /24 or narrower are required."
  }
}
variable "client_cidrs" {
  type = set(string)
  validation {
    condition     = alltrue([for c in var.client_cidrs : can(cidrnetmask(c)) && try(tonumber(split("/", c)[1]) >= 24, false)])
    error_message = "Client IPv4 CIDRs must be /24 or narrower."
  }
}
variable "enable_alb" {
  type    = bool
  default = false
}
variable "name" {
  type = string
  validation {
    condition     = can(regex("^[a-z][a-z0-9-]{2,25}$", var.name))
    error_message = "Use 3-26 lowercase letters/digits/hyphens, starting with a letter."
  }
}
variable "owner" { type = string }
variable "expires_at" {
  type = string
  validation {
    condition     = can(timecmp(var.expires_at, "2026-01-01T00:00:00Z"))
    error_message = "Provide an RFC3339 expiry for operator tracking."
  }
}
variable "release_commit" {
  type = string
  validation {
    condition     = can(regex("^[a-f0-9]{40}$", var.release_commit))
    error_message = "Pin an exact 40-character Git commit."
  }
}
variable "repository_url" {
  type    = string
  default = "https://github.com/pr3s3nt/poc.git"
  validation {
    condition     = can(regex("^https://github.com/[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+\\.git$", var.repository_url))
    error_message = "Use an unauthenticated GitHub HTTPS repository URL."
  }
}
variable "instance_type" {
  type    = string
  default = "t3.medium"
  validation {
    condition     = contains(["t3.medium", "t3.large"], var.instance_type)
    error_message = "This trial supports x86_64 t3.medium or t3.large."
  }
}
variable "volume_size" {
  type    = number
  default = 40
  validation {
    condition     = var.volume_size >= 30 && var.volume_size <= 100
    error_message = "Choose 30-100 GiB."
  }
}
variable "ssh_public_key" { type = string }
variable "existing_instance_profile" {
  type    = string
  default = ""
}
