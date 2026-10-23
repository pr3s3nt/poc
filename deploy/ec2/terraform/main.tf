provider "aws" {
  region              = var.region
  allowed_account_ids = [var.expected_account_id]
  default_tags {
    tags = local.tags
  }
}
locals {
  tags = {
    project      = "orchestrator"
    owner        = var.owner
    environment  = "ec2-trial"
    "run-id"     = var.name
    "expires-at" = var.expires_at
    "managed-by" = "orchestrator-ec2-terraform"
  }
  create_role = var.existing_instance_profile == ""
}
data "aws_vpc" "existing" { id = var.vpc_id }
data "aws_subnet" "host" { id = var.subnet_id }
data "aws_subnet" "alb" {
  for_each = var.enable_alb ? toset(var.alb_subnet_ids) : toset([])
  id       = each.value
}
data "aws_ssm_parameter" "ami" {
  name = "/aws/service/ami-amazon-linux-latest/al2023-ami-kernel-default-x86_64"
}
data "aws_iam_instance_profile" "existing" {
  count = local.create_role ? 0 : 1
  name  = var.existing_instance_profile
}
resource "aws_iam_role" "executor" {
  count = local.create_role ? 1 : 0
  name  = var.name
  assume_role_policy = jsonencode({ Version = "2012-10-17", Statement = [{
    Effect = "Allow", Principal = { Service = "ec2.amazonaws.com" }, Action = "sts:AssumeRole"
  }] })
}
resource "aws_iam_role_policy" "executor" {
  count = local.create_role ? 1 : 0
  name  = "module-provisioning"
  role  = aws_iam_role.executor[0].id
  policy = jsonencode({ Version = "2012-10-17", Statement = [
    {
      Sid       = "RegionalModuleActions", Effect = "Allow", Resource = "*"
      Action    = jsondecode(file("${path.module}/actions.json")).regional
      Condition = { StringEquals = { "aws:RequestedRegion" = var.region } }
    },
    {
      Sid    = "GlobalModuleAndVerificationActions", Effect = "Allow", Resource = "*"
      Action = jsondecode(file("${path.module}/actions.json")).global
    }
  ] })
}
resource "aws_iam_instance_profile" "executor" {
  count = local.create_role ? 1 : 0
  name  = var.name
  role  = aws_iam_role.executor[0].name
}
resource "aws_key_pair" "operator" {
  key_name   = var.name
  public_key = var.ssh_public_key
}
resource "aws_security_group" "alb" {
  count       = var.enable_alb ? 1 : 0
  name        = "${var.name}-alb"
  description = "Trial HTTP from explicit client CIDRs only"
  vpc_id      = var.vpc_id
  ingress {
    from_port   = 80
    to_port     = 80
    protocol    = "tcp"
    cidr_blocks = sort(tolist(setunion(var.client_cidrs, var.operator_cidrs)))
  }
  egress {
    from_port   = 3001
    to_port     = 3001
    protocol    = "tcp"
    cidr_blocks = [data.aws_subnet.host.cidr_block]
  }
}
resource "aws_security_group" "host" {
  name        = "${var.name}-host"
  description = "SSH operators and frontend from the trial ALB only"
  vpc_id      = var.vpc_id
  ingress {
    from_port   = 22
    to_port     = 22
    protocol    = "tcp"
    cidr_blocks = sort(tolist(var.operator_cidrs))
  }
  dynamic "ingress" {
    for_each = var.enable_alb ? [1] : []
    content {
      from_port       = 3001
      to_port         = 3001
      protocol        = "tcp"
      security_groups = [aws_security_group.alb[0].id]
    }
  }
  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }
}
resource "aws_instance" "host" {
  ami                         = nonsensitive(data.aws_ssm_parameter.ami.value)
  instance_type               = var.instance_type
  subnet_id                   = var.subnet_id
  associate_public_ip_address = true
  vpc_security_group_ids      = [aws_security_group.host.id]
  key_name                    = aws_key_pair.operator.key_name
  iam_instance_profile        = local.create_role ? aws_iam_instance_profile.executor[0].name : var.existing_instance_profile
  user_data_replace_on_change = false
  user_data = templatefile("${path.module}/user-data.sh.tftpl", {
    repository_url = var.repository_url
    release_commit = var.release_commit
    deployment     = var.name
    owner          = var.owner
    enable_alb     = var.enable_alb
  })
  metadata_options {
    http_endpoint               = "enabled"
    http_tokens                 = "required"
    http_put_response_hop_limit = 2
    http_protocol_ipv6          = "disabled"
    instance_metadata_tags      = "disabled"
  }
  root_block_device {
    volume_type           = "gp3"
    volume_size           = var.volume_size
    encrypted             = true
    delete_on_termination = true
    tags                  = local.tags
  }
  credit_specification { cpu_credits = "standard" }
  tags       = { Name = var.name }
  depends_on = [aws_iam_role_policy.executor]
  lifecycle {
    precondition {
      condition     = data.aws_subnet.host.vpc_id == var.vpc_id
      error_message = "Host subnet must belong to the selected VPC."
    }
    precondition {
      condition     = !var.enable_alb || (length(var.client_cidrs) > 0 && length(data.aws_subnet.alb) >= 2 && length(toset([for s in data.aws_subnet.alb : s.availability_zone])) >= 2 && alltrue([for s in data.aws_subnet.alb : s.vpc_id == var.vpc_id]))
      error_message = "ALB needs explicit clients and two subnets/AZs in the selected VPC."
    }
  }
}
resource "aws_lb" "console" {
  count                      = var.enable_alb ? 1 : 0
  name                       = var.name
  internal                   = false
  load_balancer_type         = "application"
  subnets                    = var.alb_subnet_ids
  security_groups            = [aws_security_group.alb[0].id]
  drop_invalid_header_fields = true
}
resource "aws_lb_target_group" "console" {
  count                = var.enable_alb ? 1 : 0
  name                 = var.name
  port                 = 3001
  protocol             = "HTTP"
  vpc_id               = var.vpc_id
  deregistration_delay = 10
  health_check {
    path                = "/healthz"
    matcher             = "200"
    interval            = 15
    healthy_threshold   = 2
    unhealthy_threshold = 3
  }
}
resource "aws_lb_target_group_attachment" "console" {
  count            = var.enable_alb ? 1 : 0
  target_group_arn = aws_lb_target_group.console[0].arn
  target_id        = aws_instance.host.id
  port             = 3001
}
resource "aws_lb_listener" "console" {
  count             = var.enable_alb ? 1 : 0
  load_balancer_arn = aws_lb.console[0].arn
  port              = 80
  protocol          = "HTTP"
  default_action {
    type             = "forward"
    target_group_arn = aws_lb_target_group.console[0].arn
  }
}
