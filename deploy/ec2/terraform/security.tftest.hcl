mock_provider "aws" {
  mock_resource "aws_lb" {
    defaults = { arn = "arn:aws:elasticloadbalancing:us-east-1:123456789012:loadbalancer/app/trial/1234567890123456" }
  }
  mock_resource "aws_lb_target_group" {
    defaults = { arn = "arn:aws:elasticloadbalancing:us-east-1:123456789012:targetgroup/trial/1234567890123456" }
  }

  mock_data "aws_vpc" { defaults = { id = "vpc-test" } }
  mock_data "aws_subnet" {
    defaults = { vpc_id = "vpc-test", cidr_block = "172.31.0.0/20", availability_zone = "us-east-1a" }
  }
  mock_data "aws_ssm_parameter" { defaults = { value = "ami-test" } }
}
variables {
  expected_account_id = "123456789012"
  region              = "us-east-1"
  vpc_id              = "vpc-test"
  subnet_id           = "subnet-test"
  alb_subnet_ids      = []
  operator_cidrs      = ["192.0.2.1/32"]
  client_cidrs        = []
  name                = "orch-security-test"
  owner               = "test"
  expires_at          = "2026-10-24T12:00:00Z"
  release_commit      = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
  ssh_public_key      = "ssh-ed25519 TEST"
}
run "private_host" {
  command = plan
  assert {
    condition     = aws_instance.host.metadata_options[0].http_tokens == "required" && aws_instance.host.metadata_options[0].http_put_response_hop_limit == 2
    error_message = "IMDSv2 is required with Docker-compatible hop limit."
  }
  assert {
    condition     = aws_instance.host.root_block_device[0].encrypted && aws_instance.host.credit_specification[0].cpu_credits == "standard"
    error_message = "Encrypt storage and avoid unbounded CPU credit charges."
  }
  assert {
    condition     = length(aws_security_group.host.ingress) == 1 && alltrue([for r in aws_security_group.host.ingress : r.from_port == 22 && toset(r.cidr_blocks) == toset(["192.0.2.1/32"])])
    error_message = "Without ALB only explicitly scoped SSH may be exposed."
  }
}
run "reject_public_ssh" {
  command = plan
  variables { operator_cidrs = ["0.0.0.0/0"] }
  expect_failures = [var.operator_cidrs]
}
run "reject_public_console" {
  command = plan
  variables { client_cidrs = ["0.0.0.0/0"] }
  expect_failures = [var.client_cidrs]
}
run "reject_wrong_vpc" {
  command = plan
  variables { vpc_id = "vpc-other" }
  expect_failures = [aws_instance.host]
}
run "restricted_alb" {
  command = apply
  variables {
    enable_alb     = true
    client_cidrs   = ["198.51.100.2/32"]
    alb_subnet_ids = ["subnet-a", "subnet-b"]
  }
  override_data {
    target = data.aws_subnet.alb["subnet-a"]
    values = { vpc_id = "vpc-test", availability_zone = "us-east-1a" }
  }
  override_data {
    target = data.aws_subnet.alb["subnet-b"]
    values = { vpc_id = "vpc-test", availability_zone = "us-east-1b" }
  }
  assert {
    condition     = alltrue([for r in aws_security_group.alb[0].ingress : r.from_port == 80 && toset(r.cidr_blocks) == toset(["192.0.2.1/32", "198.51.100.2/32"])])
    error_message = "ALB must accept only explicit trial clients and operators."
  }
  assert {
    condition     = length(aws_security_group.host.ingress) == 2 && alltrue([for r in aws_security_group.host.ingress : r.from_port == 22 || (r.from_port == 3001 && try(length(r.security_groups), 0) == 1 && try(length(r.cidr_blocks), 0) == 0)])
    error_message = "Frontend may only be reached from its ALB security group."
  }
}
