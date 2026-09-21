terraform {
  required_version = ">= 1.5"
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = ">= 5.80"
    }
  }
}

provider "aws" {
  region = var.region
  default_tags {
    tags = var.tags
  }
}

variable "region" { type = string }
variable "name" { type = string }
variable "vpc_id" { type = string }
variable "vpc_cidr" { type = string }
variable "subnet_ids" { type = list(string) }
variable "database" { type = string }
variable "username" { type = string }
variable "master_password" {
  type      = string
  sensitive = true
}
variable "engine_version" { type = string }
variable "min_capacity" { type = number }
variable "max_capacity" { type = number }
variable "seconds_until_auto_pause" {
  type    = number
  default = 3600
}
variable "tags" {
  type    = map(string)
  default = {}
}

resource "aws_db_subnet_group" "this" {
  name       = var.name
  subnet_ids = var.subnet_ids
  tags       = { Name = var.name }
}

resource "aws_security_group" "this" {
  name        = "${var.name}-db"
  description = "Aurora PostgreSQL access from inside the application VPC"
  vpc_id      = var.vpc_id

  ingress {
    description = "PostgreSQL from the VPC"
    from_port   = 5432
    to_port     = 5432
    protocol    = "tcp"
    cidr_blocks = [var.vpc_cidr]
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }

  tags = { Name = "${var.name}-db" }
}

# One Aurora Serverless v2 writer on Aurora Standard storage. No read replica,
# no I/O-Optimized, lowest capacity range the engine accepts.
resource "aws_rds_cluster" "this" {
  cluster_identifier           = var.name
  engine                       = "aurora-postgresql"
  engine_version               = var.engine_version
  database_name                = var.database
  master_username              = var.username
  master_password              = var.master_password
  db_subnet_group_name         = aws_db_subnet_group.this.name
  vpc_security_group_ids       = [aws_security_group.this.id]
  backup_retention_period      = 1
  skip_final_snapshot          = true
  apply_immediately            = true
  deletion_protection          = false
  storage_type                 = "aurora"
  performance_insights_enabled = false

  serverlessv2_scaling_configuration {
    min_capacity             = var.min_capacity
    max_capacity             = var.max_capacity
    seconds_until_auto_pause = var.seconds_until_auto_pause
  }
}

resource "aws_rds_cluster_instance" "writer" {
  identifier                   = "${var.name}-writer"
  cluster_identifier           = aws_rds_cluster.this.id
  instance_class               = "db.serverless"
  engine                       = aws_rds_cluster.this.engine
  engine_version               = aws_rds_cluster.this.engine_version
  publicly_accessible          = false
  performance_insights_enabled = false
  monitoring_interval          = 0
}

output "host" { value = aws_rds_cluster.this.endpoint }
output "port" { value = aws_rds_cluster.this.port }
output "database" { value = aws_rds_cluster.this.database_name }
output "username" { value = aws_rds_cluster.this.master_username }

output "password" {
  value     = var.master_password
  sensitive = true
}
output "cluster_identifier" { value = aws_rds_cluster.this.cluster_identifier }
output "writer_identifier" { value = aws_rds_cluster_instance.writer.identifier }
output "security_group_id" { value = aws_security_group.this.id }
