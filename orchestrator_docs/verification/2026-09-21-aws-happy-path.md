---
id: VERIFY-2026-09-21-AWS
artifact: verification-evidence
status: evidence
last_reviewed: 2026-09-21
---

# 2026-09-21 — AWS cloud happy path

## Environment and cost

- Region: `us-east-1`; identity resolved through the local default credential
  chain without persisting credentials.
- Latest full cloud run: `aws-20260921052038`.
- Configuration: EKS 1.34, one `t4g.small` SPOT arm64 node, 20 GiB gp3, Aurora
  PostgreSQL Serverless v2 min 0/max 1 ACU, two public subnets, no NAT Gateway or
  public load balancer.
- Pre-run estimate was approximately USD 0.1725/hour; resources existed for
  roughly 45 minutes in the earlier measured run.

## Result

- Deployments were submitted through `POST /api/v1/deployments` to an
  `-adapters aws` process.
- Orchestrator enriched application-scoped VPC/EKS, provisioned Aurora, passed
  outputs to backend/worker and deployed all three workloads.
- End-to-end job flow returned `processed:AWS-1789969672664387632`.
- Deployment view showed `SUCCEEDED`, graph/batches/resources/workloads and
  redacted postgres password.
- Web Console live test passed; no Kubernetes Service of type LoadBalancer was
  created.

## Cleanup proof

Namespace, temporary ECR repositories and Terraform resources were removed in
reverse order. Seventeen tag/run-ID queries for networking, compute, storage,
EKS, RDS, ECR, load balancers and IAM resources returned empty. Independent
regional checks found no test EKS/RDS/ECR/EC2/EBS/VPC resources; the pre-existing
default VPC remained.

## Limitation

This full AWS run predates the final shared-planning/conformance changes. Unit,
conformance and kind verification cover those changes, but `aws-verify.sh` must
run again before release-ready status.
