terraform {
  required_version = ">= 1.9.8, < 2.0.0"
  required_providers {
    aws = { source = "hashicorp/aws", version = "5.100.0" }
  }
  backend "local" {}
}
