variable "size" {
  type = string
}

variable "region" {
  type = string
  default = "us-east-1"
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
