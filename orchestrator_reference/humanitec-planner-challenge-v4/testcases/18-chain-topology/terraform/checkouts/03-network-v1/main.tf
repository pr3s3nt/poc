variable "cidr" {
  type = string
  default = "10.0.0.0/16"
}

variable "region" {
  type = string
}

output "subnet_id" {
  value = "fixture-subnet_id"
}
