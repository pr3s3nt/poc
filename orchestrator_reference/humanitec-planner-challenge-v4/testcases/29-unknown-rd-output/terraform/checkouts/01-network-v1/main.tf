variable "cidr" {
  type = string
  default = "10.0.0.0/16"
}

output "subnet_id" {
  value = "fixture-subnet_id"
}
