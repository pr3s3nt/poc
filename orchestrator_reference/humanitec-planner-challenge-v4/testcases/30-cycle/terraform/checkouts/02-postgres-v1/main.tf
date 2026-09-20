variable "size" {
  type = string
}

variable "region" {
  type = string
}

variable "subnet_id" {
  type = string
}

output "host" {
  value = "fixture-host"
}

output "port" {
  value = "fixture-port"
}

output "name" {
  value = "fixture-name"
}
