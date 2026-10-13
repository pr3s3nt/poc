mock_provider "aws" {
  mock_data "aws_caller_identity" {
    defaults = {
      arn        = "arn:aws:iam::123456789012:user/executor"
      account_id = "123456789012"
    }
  }
  mock_data "aws_iam_session_context" {
    defaults = {
      issuer_arn = "arn:aws:iam::123456789012:user/executor"
    }
  }
  mock_data "aws_iam_policy_document" {
    defaults = {
      json = "{\"Version\":\"2012-10-17\",\"Statement\":[{\"Effect\":\"Allow\",\"Action\":\"sts:AssumeRole\",\"Principal\":{\"Service\":\"eks.amazonaws.com\"}}]}"
    }
  }
}

variables {
  region             = "us-east-1"
  name               = "orch-test"
  subnet_ids         = ["subnet-0a1", "subnet-0b2"]
  kubernetes_version = "1.35"
  # RFC 5737 documentation addresses: test fixtures only.
  public_access_cidrs = ["203.0.113.10/32", "198.51.100.7/32"]
  node_instance_type = "t4g.small"
  node_capacity_type = "SPOT"
  node_disk_size     = 20
  node_ami_type      = "AL2023_ARM_64_STANDARD"
  tags               = { "orchestrator-instance" = "0123456789abcdef", "project" = "poc" }
}

run "cluster_policy" {
  command = plan

  assert {
    condition     = aws_eks_cluster.this.version == "1.35" && aws_eks_cluster.this.upgrade_policy[0].support_type == "STANDARD"
    error_message = "standard-support Kubernetes version and upgrade policy"
  }
  assert {
    condition     = aws_eks_cluster.this.access_config[0].authentication_mode == "API" && aws_eks_cluster.this.access_config[0].bootstrap_cluster_creator_admin_permissions == false
    error_message = "API-only access with no implicit creator admin"
  }
  assert {
    condition     = aws_eks_access_entry.execution.principal_arn == "arn:aws:iam::123456789012:user/executor" && aws_eks_access_entry.execution.type == "STANDARD"
    error_message = "the execution principal has an explicit access entry"
  }
  assert {
    condition     = aws_eks_access_policy_association.execution.policy_arn == "arn:aws:eks::aws:cluster-access-policy/AmazonEKSClusterAdminPolicy" && aws_eks_access_policy_association.execution.access_scope[0].type == "cluster"
    error_message = "cluster-scoped admin access policy for the execution principal"
  }
  assert {
    condition     = toset(aws_eks_cluster.this.vpc_config[0].public_access_cidrs) == toset(["203.0.113.10/32", "198.51.100.7/32"]) && aws_eks_cluster.this.vpc_config[0].endpoint_private_access && aws_eks_cluster.this.vpc_config[0].endpoint_public_access
    error_message = "public API only for the allowlisted hosts, private endpoint kept"
  }
  assert {
    condition     = length(aws_eks_cluster.this.enabled_cluster_log_types) == 0
    error_message = "no control-plane log export (no CloudWatch charges)"
  }
  assert {
    condition     = aws_eks_node_group.this.scaling_config[0].desired_size == 1 && aws_eks_node_group.this.scaling_config[0].max_size == 1
    error_message = "one node"
  }
  assert {
    condition     = aws_eks_node_group.this.capacity_type == "SPOT" && aws_eks_node_group.this.ami_type == "AL2023_ARM_64_STANDARD"
    error_message = "node sizing follows the variables"
  }
}

run "extended_support_version_refused" {
  command = plan
  variables {
    kubernetes_version = "1.33"
  }
  expect_failures = [var.kubernetes_version]
}

run "one_subnet_refused" {
  command = plan
  variables {
    subnet_ids = ["subnet-0a1"]
  }
  expect_failures = [var.subnet_ids]
}

run "bad_capacity_refused" {
  command = plan
  variables {
    node_capacity_type = "RESERVED"
  }
  expect_failures = [var.node_capacity_type]
}

run "al2_refused" {
  command = plan
  variables {
    node_ami_type = "AL2_x86_64"
  }
  expect_failures = [var.node_ami_type]
}

run "ebs_csi_off_by_default" {
  command = plan
  assert {
    condition     = length(aws_eks_addon.ebs_csi) == 0 && length(aws_eks_addon.pod_identity_agent) == 0 && length(aws_iam_role.ebs_csi) == 0
    error_message = "no addon or extra role unless the Definition asks for the EBS CSI driver"
  }
}

run "ebs_csi_on_request" {
  command = plan
  variables {
    ebs_csi_driver = true
  }
  assert {
    condition     = aws_eks_addon.ebs_csi[0].addon_name == "aws-ebs-csi-driver" && aws_eks_addon.pod_identity_agent[0].addon_name == "eks-pod-identity-agent"
    error_message = "EBS CSI and Pod Identity addons"
  }
  assert {
    condition     = aws_iam_role_policy_attachment.ebs_csi[0].policy_arn == "arn:aws:iam::aws:policy/service-role/AmazonEBSCSIDriverPolicy"
    error_message = "the driver role only has the AWS-managed EBS CSI policy"
  }
}

run "addon_builds_are_pinned_for_every_supported_version" {
  command = plan
  variables {
    ebs_csi_driver     = true
    kubernetes_version = "1.37"
  }
  assert {
    condition     = aws_eks_addon.ebs_csi[0].addon_version == "v1.66.0-eksbuild.1" && aws_eks_addon.pod_identity_agent[0].addon_version == "v1.4.0-eksbuild.4"
    error_message = "add-on builds are exact"
  }
}

run "addon_builds_are_pinned_on_1_34" {
  command = plan
  variables {
    ebs_csi_driver     = true
    kubernetes_version = "1.34"
  }
  assert {
    condition     = aws_eks_addon.ebs_csi[0].addon_version == "v1.66.0-eksbuild.1" && aws_eks_addon.pod_identity_agent[0].addon_version == "v1.4.0-eksbuild.4"
    error_message = "add-on builds are exact"
  }
}

run "empty_allowlist_refused" {
  command = plan
  variables {
    public_access_cidrs = []
  }
  expect_failures = [var.public_access_cidrs]
}

run "open_to_all_refused" {
  command = plan
  variables {
    public_access_cidrs = ["0.0.0.0/0"]
  }
  expect_failures = [var.public_access_cidrs]
}

run "wide_prefix_refused" {
  command = plan
  variables {
    public_access_cidrs = ["203.0.113.0/24"]
  }
  expect_failures = [var.public_access_cidrs]
}

run "single_host_without_prefix_refused" {
  command = plan
  variables {
    public_access_cidrs = ["203.0.113.10"]
  }
  expect_failures = [var.public_access_cidrs]
}

run "invalid_octet_refused" {
  command = plan
  variables {
    public_access_cidrs = ["999.1.1.1/32"]
  }
  expect_failures = [var.public_access_cidrs]
}

run "ipv6_refused" {
  command = plan
  variables {
    public_access_cidrs = ["2001:db8::1/128"]
  }
  expect_failures = [var.public_access_cidrs]
}

run "unspecified_host_refused" {
  command = plan
  variables {
    public_access_cidrs = ["0.0.0.0/32"]
  }
  expect_failures = [var.public_access_cidrs]
}

run "duplicate_refused" {
  command = plan
  variables {
    public_access_cidrs = ["203.0.113.10/32", "203.0.113.10/32"]
  }
  expect_failures = [var.public_access_cidrs]
}
