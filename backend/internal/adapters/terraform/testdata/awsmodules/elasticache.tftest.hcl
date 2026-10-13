mock_provider "aws" {
  mock_data "aws_subnet" {
    defaults = {
      cidr_block        = "10.42.0.0/20"
      availability_zone = "us-east-1a"
    }
  }
}

variables {
  region     = "us-east-1"
  vpc_id     = "vpc-0123"
  subnet_ids = ["subnet-0a1", "subnet-0b2"]
  private_subnet_ids = ["subnet-0p1", "subnet-0p2"]
  password       = "pw-0123456789abcdef0123456789abcdef"
  engine_version = "7.1"
  node_type      = "cache.t4g.micro"
  tags       = { "orchestrator-instance" = "0123456789abcdef", "project" = "poc" }
}

run "private_tls_auth_cache" {
  command = plan

  override_data {
    target = data.aws_subnet.node[1]
    values = {
      cidr_block        = "10.42.16.0/20"
      availability_zone = "us-east-1b"
    }
  }

  assert {
    condition     = aws_elasticache_replication_group.this.replication_group_id == "orc-0123456789abcdef" && aws_elasticache_user_group.this.user_group_id == "orc-0123456789abcdef"
    error_message = "names derive from the stable instance tag"
  }
  assert {
    condition     = toset(aws_elasticache_subnet_group.this.subnet_ids) == toset(["subnet-0p1", "subnet-0p2"])
    error_message = "the subnet group only references the shared private subnets"
  }
  assert {
    condition     = length(aws_security_group.this.ingress) == 1 && one(aws_security_group.this.ingress).from_port == 6379 && toset(one(aws_security_group.this.ingress).cidr_blocks) == toset(["10.42.0.0/20", "10.42.16.0/20"])
    error_message = "ingress is Redis from the node subnets only, no egress"
  }
  assert {
    condition     = aws_elasticache_replication_group.this.transit_encryption_enabled && aws_elasticache_replication_group.this.at_rest_encryption_enabled
    error_message = "TLS and encryption at rest"
  }
  assert {
    condition     = aws_elasticache_replication_group.this.engine == "redis" && aws_elasticache_replication_group.this.engine_version == "7.1" && aws_elasticache_replication_group.this.node_type == "cache.t4g.micro"
    error_message = "Redis OSS 7.1 on the smallest burstable node"
  }
  assert {
    condition     = aws_elasticache_replication_group.this.num_cache_clusters == 1 && aws_elasticache_replication_group.this.automatic_failover_enabled == false && aws_elasticache_replication_group.this.snapshot_retention_limit == 0
    error_message = "one node, no failover, no snapshot"
  }
  assert {
    condition     = aws_elasticache_user.default.access_string == "off ~* -@all" && aws_elasticache_user.default.user_name == "default" && aws_elasticache_user.app.user_name == "app" && aws_elasticache_user.app.access_string == "on ~* +@all"
    error_message = "default user off, application user on"
  }
  assert {
    condition     = length(aws_elasticache_user_group.this.user_ids) == 2
    error_message = "the user group holds exactly the two users"
  }
  assert {
    condition     = output.username == "app" && output.tls == true
    error_message = "Redis username and tls outputs"
  }
}

run "default_username_refused" {
  command = plan
  variables {
    username = "default"
  }
  expect_failures = [var.username]
}

run "big_node_refused" {
  command = plan
  variables {
    node_type = "cache.r7g.4xlarge"
  }
  expect_failures = [var.node_type]
}

run "old_engine_refused" {
  command = plan
  variables {
    engine_version = "6.2"
  }
  expect_failures = [var.engine_version]
}

run "missing_instance_tag_refused" {
  command = plan
  variables {
    tags = { "project" = "poc" }
  }
  expect_failures = [var.tags]
}

run "nonexistent_t3g_refused" {
  command = plan
  variables {
    node_type = "cache.t3g.micro"
  }
  expect_failures = [var.node_type]
}

run "x86_burstable_refused" {
  command = plan
  variables {
    node_type = "cache.t3.micro"
  }
  expect_failures = [var.node_type]
}

run "t4g_small_accepted" {
  command = plan
  variables {
    node_type = "cache.t4g.small"
  }
  assert {
    condition     = aws_elasticache_replication_group.this.node_type == "cache.t4g.small"
    error_message = "t4g.small is a valid small node"
  }
}

# MOCK SEAM: two distinct ElastiCache instances planned against the SAME shared
# private subnet IDs; each owns its subnet group, security group, users and group.
run "second_instance_shares_subnets" {
  command = plan
  variables {
    tags = { "orchestrator-instance" = "fedcba9876543210", "project" = "poc" }
  }
  assert {
    condition     = aws_elasticache_replication_group.this.replication_group_id == "orc-fedcba9876543210" && aws_elasticache_user_group.this.user_group_id == "orc-fedcba9876543210" && aws_elasticache_user.app.user_id == "orc-fedcba9876543210-app"
    error_message = "second instance has its own identity"
  }
  assert {
    condition     = toset(aws_elasticache_subnet_group.this.subnet_ids) == toset(["subnet-0p1", "subnet-0p2"])
    error_message = "second instance references the same shared subnets"
  }
}

run "one_private_subnet_refused" {
  command = plan
  variables {
    private_subnet_ids = ["subnet-0p1"]
  }
  expect_failures = [var.private_subnet_ids]
}
