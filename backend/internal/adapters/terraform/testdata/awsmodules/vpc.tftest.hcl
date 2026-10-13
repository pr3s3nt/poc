mock_provider "aws" {
  mock_data "aws_availability_zones" {
    defaults = {
      names = ["us-east-1a", "us-east-1b", "us-east-1c", "us-east-1d"]
    }
  }
}

variables {
  region = "us-east-1"
  name   = "orch-test"
  tags   = { "orchestrator-instance" = "0123456789abcdef", "project" = "poc" }
}

run "default_network" {
  command = plan

  assert {
    condition     = length(aws_subnet.public) == 2 && length(aws_route_table_association.public) == 2
    error_message = "two public subnets and associations expected"
  }
  assert {
    condition     = alltrue([for s in aws_subnet.public : s.map_public_ip_on_launch])
    error_message = "node subnets hand out public IPv4 (no NAT Gateway)"
  }
  assert {
    condition     = aws_subnet.public[0].cidr_block == "10.42.0.0/20" && aws_subnet.public[1].cidr_block == "10.42.16.0/20"
    error_message = "public subnets use the low /20 slots"
  }
}

run "shared_private_network" {
  command = plan

  assert {
    condition     = length(aws_subnet.private) == 2 && length(aws_route_table_association.private) == 2
    error_message = "one private subnet and association per zone, created once"
  }
  assert {
    condition     = aws_subnet.private[0].cidr_block == "10.42.128.0/20" && aws_subnet.private[1].cidr_block == "10.42.144.0/20"
    error_message = "private subnets use slots 8 and 9"
  }
  assert {
    condition     = !aws_subnet.private[0].map_public_ip_on_launch && !aws_subnet.private[1].map_public_ip_on_launch
    error_message = "private subnets hand out no public IPv4"
  }
  assert {
    condition     = aws_subnet.private[0].availability_zone == aws_subnet.public[0].availability_zone && aws_subnet.private[1].availability_zone == aws_subnet.public[1].availability_zone
    error_message = "private subnets follow the node zones"
  }
}

run "four_zones" {
  command = plan
  variables {
    az_count = 4
    cidr     = "10.50.0.0/16"
  }
  assert {
    condition     = length(aws_subnet.public) == 4 && aws_subnet.public[3].cidr_block == "10.50.48.0/20" && length(aws_subnet.private) == 4 && aws_subnet.private[3].cidr_block == "10.50.176.0/20"
    error_message = "four zones use slots 0 to 3"
  }
}

run "one_zone_refused" {
  command = plan
  variables {
    az_count = 1
  }
  expect_failures = [var.az_count]
}

run "wide_cidr_refused" {
  command = plan
  variables {
    cidr = "10.0.0.0/8"
  }
  expect_failures = [var.cidr]
}

run "bad_name_refused" {
  command = plan
  variables {
    name = "Bad_Name"
  }
  expect_failures = [var.name]
}
