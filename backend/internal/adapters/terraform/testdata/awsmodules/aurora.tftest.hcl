mock_provider "aws" {
  mock_data "aws_subnet" {
    defaults = {
      cidr_block        = "10.42.0.0/20"
      availability_zone = "us-east-1a"
    }
  }
}

variables {
  region          = "us-east-1"
  name            = "orch-app-env"
  vpc_id          = "vpc-0123"
  subnet_ids      = ["subnet-0a1", "subnet-0b2"]
  private_subnet_ids = ["subnet-0p1", "subnet-0p2"]
  database        = "acceptance"
  username        = "app"
  engine_version  = "16.14"
  min_capacity    = 0
  max_capacity    = 1
  master_password = "pw-0123456789abcdef0123456789abcdef"
  tags            = { "orchestrator-instance" = "0123456789abcdef", "project" = "poc" }
}

run "private_database" {
  command = plan

  override_data {
    target = data.aws_subnet.node[1]
    values = {
      cidr_block        = "10.42.16.0/20"
      availability_zone = "us-east-1b"
    }
  }

  assert {
    condition     = aws_rds_cluster.this.cluster_identifier == "orc-0123456789abcdef" && aws_rds_cluster_instance.writer.identifier == "orc-0123456789abcdef-writer"
    error_message = "names derive from the stable instance tag"
  }
  assert {
    condition     = toset(aws_db_subnet_group.this.subnet_ids) == toset(["subnet-0p1", "subnet-0p2"])
    error_message = "the subnet group only references the shared private subnets"
  }
  assert {
    condition     = length(aws_security_group.this.ingress) == 1 && one(aws_security_group.this.ingress).from_port == 5432 && one(aws_security_group.this.ingress).to_port == 5432 && toset(one(aws_security_group.this.ingress).cidr_blocks) == toset(["10.42.0.0/20", "10.42.16.0/20"])
    error_message = "ingress is PostgreSQL from the node subnets only"
  }
  assert {
    condition     = aws_rds_cluster.this.storage_encrypted && aws_rds_cluster.this.engine == "aurora-postgresql" && aws_rds_cluster.this.engine_version == "16.14" && aws_rds_cluster.this.storage_type == "aurora"
    error_message = "encrypted Aurora PostgreSQL 16.14 standard storage"
  }
  assert {
    condition     = aws_rds_cluster.this.serverlessv2_scaling_configuration[0].min_capacity == 0 && aws_rds_cluster.this.serverlessv2_scaling_configuration[0].max_capacity == 1 && aws_rds_cluster.this.serverlessv2_scaling_configuration[0].seconds_until_auto_pause == 3600
    error_message = "smallest Serverless v2 range with auto-pause"
  }
  assert {
    condition     = aws_rds_cluster_instance.writer.publicly_accessible == false && aws_rds_cluster_instance.writer.instance_class == "db.serverless" && aws_rds_cluster_instance.writer.monitoring_interval == 0
    error_message = "one private serverless writer, no enhanced monitoring"
  }
  assert {
    condition     = aws_rds_cluster.this.deletion_protection == false && aws_rds_cluster.this.skip_final_snapshot == true && aws_rds_cluster.this.backup_retention_period == 1
    error_message = "destroy policy: deletable by a confirmed delete, one-day backup"
  }
}

run "min_capacity_floor" {
  command = plan
  variables {
    min_capacity = 0.5
  }
  assert {
    condition     = aws_rds_cluster.this.serverlessv2_scaling_configuration[0].min_capacity == 0.5
    error_message = "a nonzero minimum is accepted"
  }
}

run "public_db_names_refused" {
  command = plan
  variables {
    username = "rdsadmin"
  }
  expect_failures = [var.username]
}

run "bad_engine_refused" {
  command = plan
  variables {
    engine_version = "15.7"
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

run "oversize_capacity_refused" {
  command = plan
  variables {
    max_capacity = 64
  }
  expect_failures = [var.max_capacity]
}

run "min_above_max_refused" {
  command = plan
  variables {
    min_capacity = 8
    max_capacity = 1
  }
  expect_failures = [var.max_capacity]
}

run "fractional_min_refused" {
  command = plan
  variables {
    min_capacity = 0.3
  }
  expect_failures = [var.min_capacity]
}

run "fractional_max_refused" {
  command = plan
  variables {
    max_capacity = 1.2
  }
  expect_failures = [var.max_capacity]
}

run "max_below_one_refused" {
  command = plan
  variables {
    min_capacity = 0
    max_capacity = 0.5
  }
  expect_failures = [var.max_capacity]
}

run "equal_min_max_accepted" {
  command = plan
  variables {
    min_capacity = 2
    max_capacity = 2
  }
  assert {
    condition     = aws_rds_cluster.this.serverlessv2_scaling_configuration[0].min_capacity == 2 && aws_rds_cluster.this.serverlessv2_scaling_configuration[0].max_capacity == 2
    error_message = "equal min and max is a valid range"
  }
}

run "half_acu_step_accepted" {
  command = plan
  variables {
    min_capacity = 0.5
    max_capacity = 1.5
  }
}

# MOCK SEAM: two distinct Aurora instances (different stable instance tags)
# planned against the SAME shared private subnet IDs. Each owns its subnet group,
# security group and cluster; neither creates a subnet or a CIDR.
run "first_instance" {
  command = plan
  assert {
    condition     = aws_rds_cluster.this.cluster_identifier == "orc-0123456789abcdef" && aws_db_subnet_group.this.name == "orc-0123456789abcdef"
    error_message = "first instance names"
  }
}

run "second_instance_shares_subnets" {
  command = plan
  variables {
    tags = { "orchestrator-instance" = "fedcba9876543210", "project" = "poc" }
  }
  assert {
    condition     = aws_rds_cluster.this.cluster_identifier == "orc-fedcba9876543210" && aws_db_subnet_group.this.name == "orc-fedcba9876543210" && aws_security_group.this.name == "orc-fedcba9876543210-db"
    error_message = "second instance has its own identity"
  }
  assert {
    condition     = toset(aws_db_subnet_group.this.subnet_ids) == toset(["subnet-0p1", "subnet-0p2"])
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
