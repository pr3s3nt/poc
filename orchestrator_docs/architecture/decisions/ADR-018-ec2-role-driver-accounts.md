---
id: ADR-018
artifact: architecture-decision
status: current
last_reviewed: 2026-10-20
---

# ADR-018 — Explicit EC2 instance-role Driver Accounts

Accepted for [issue #6](https://github.com/pr3s3nt/poc/issues/6). The user can run
backend/executor on an EC2 instance with an attached provisioning role, and cannot
obtain an IAM User. Extend ADR-009/015 with an explicit AWS_EC2_ROLE method;
AWS_ACCESS_KEY continues resolving only its scoped Vault object.

The PE registers the expected canonical IAM Role ARN and verification regions.
The source is IMDSv2 at the fixed link-local endpoint only: no IMDSv1, proxy,
redirect, environment credential, shared profile or client-supplied endpoint.
STS verifies the account and assumed role; IAM GetRole verifies canonical ARN
(including path) and unique RoleId against STS UserId. Pin both ARN and RoleId.
A recreated role or another instance role must fail closed. Region probes and
existing read-only policy simulation use the canonical IAM role principal.
Simulation is registration-time evidence, not a guarantee of future authorization.

EC2 accounts have no secret ref and never store temporary credentials. They use
the existing Connection aggregate, Organization ownership and version CAS.
Rename/reverify are supported; credential replacement and auth-method changes
are rejected. Register a new account for another role. Multiple executor hosts
must have the same pinned role or fail; workload Pods receive no account credentials.

For long-running Terraform, the binder starts a per-invocation authenticated
loopback credential endpoint implementing the AWS container credential protocol.
Each retrieval revalidates the scoped READY account and IMDS/STS role identity;
role ARN/RoleId are pinned for the invocation. Expiration enables provider refresh.
The subprocess has private empty shared config, no static keys/host chain and
IMDS disabled. The endpoint is internal, not a public credential API, and closes
with ProviderEnv.Close. Readiness reads this same bounded authenticated endpoint;
EKS target token generation uses newly resolved credentials. No role credential
is persisted in a plan, logical state, SQL, Vault or logs.

Persistence uses the existing fresh-baseline procedure: update its authentication
constraint, no startup migration or reset. A previous SQL baseline is incompatible;
operators must retain the previous release for retained resources until an
explicitly authorized baseline transition. This feature does not authorize reset.

The user subsequently authorized a temporary EC2 and live VPC verification with
complete run-owned cleanup (account 452025861381/us-east-1). Local tests use
HTTP seams for metadata/STS/IAM and subprocess credential refresh. OIDC, Roles
Anywhere, cross-account AssumeRole and creating/managing the EC2 role are excluded.

See [UC-04](../../usecase/UC-04/specification.md),
[credential design](../connection-credentials.md),
[schema](../database/schema.md) and [AWS runbook](../../operations/aws.md).
