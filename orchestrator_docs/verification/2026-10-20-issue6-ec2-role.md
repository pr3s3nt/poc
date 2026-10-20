---
id: VERIFY-20261020-ISSUE6-EC2-ROLE
artifact: verification-record
status: evidence
last_reviewed: 2026-10-20
---

# Issue #6 — EC2 IAM role authentication

[Issue #6](https://github.com/pr3s3nt/poc/issues/6),
[ADR-018](../architecture/decisions/ADR-018-ec2-role-driver-accounts.md),
[UC-04 EC2-01..05](../usecase/UC-04/specification.md).

User authorized implementation, a temporary EC2, full app Docker startup, live
Terraform verification and cleanup. No IAM User credential is required by the
product flow. Operator preflight used the existing local AWS principal only to
create/manage the isolated verification infrastructure; those credentials were
never copied into EC2, Docker or the account catalog.

## Scope and local checks

- `cd backend && go test ./... && go build ./...`: PASS. Database-dependent
  tests without a configured server follow their existing skip behavior.
- Entire affected PostgreSQL adapter suite separately ran with a disposable local
  PostgreSQL 16 and `REQUIRE_POSTGRES=1`: PASS (214.676s), including EC2 role
  persistence/reopen and version CAS. Disposable container removed.
- Focused connection/awsaccess/provider tests with `-race`: PASS.
- Frontend typecheck/lint, all 56 files / 784 tests and production build: PASS.
  After error-guidance edits, the 17 account tests and frontend checks repeated.
- Shell syntax, Compose merged configuration, documentation/PlantUML checker and
  `git diff --check`: PASS before final handoff; final documentation checks follow.

The browser workload exercises real Console/API/SQL/Vault wiring on EC2. The
[Go workload](../../backend/test/acceptance/ec2-role/main.go) loads the safe
registered Account DTO into a scoped metadata reader, uses the production
CredentialResolver/ProviderBinder and real Terraform AWS provider 6.60.0. Its
fresh private local Terraform state is an acceptance artifact, separate from the
product encrypted PostgreSQL state lifecycle. It plans exactly one tagged VPC,
applies it and always attempts destroy after apply. No EKS/application workload
or full product Deploy lifecycle is claimed by this focused test.

## AWS preflight and estimate

Account `452025861381`, region `us-east-1`.
Run `i6-20261020T110510Z`; private recovery directory
`/var/tmp/poc-issue6-ec2-eGxAigEv` (not a public artifact).

| Resource | Count / sizing | Unit price | Planned 2-hour cost |
|---|---|---|---|
| EC2 Linux shared On-Demand | 1 m7i.large | $0.1008/hour, AWS Pricing API on this run | $0.2016 |
| Root EBS gp3, encrypted/delete-on-termination | 30 GiB | $0.08/GiB-month | about $0.0066 |
| Public IPv4 (no Elastic IP) | 1 | $0.005/hour | $0.0100 |
| Verification VPC | 1, no NAT/Gateway/LB/EKS | no hourly VPC charge | $0 |

Estimated total about $0.2182 for two hours, excluding data transfer/tax. This is
an estimate, not a fixed billing ceiling; actual cost remains unconfirmed.
Sources: [AWS EBS pricing](https://aws.amazon.com/ebs/pricing/),
[AWS VPC IPv4 pricing](https://aws.amazon.com/vpc/pricing/).
The run has project/owner/environment/run-id/expires-at/managed-by tags, a two-hour
independent cleanup supervisor and a provisioning-failure cleanup trap.

Borrowed default VPC `vpc-074a751c57b841c62` / subnet
`subnet-0df6969f2653c3b54` are preexisting and must remain. Only a new run-owned
security group, SSH key pair, IAM role/profile/inline policy, EC2/root volume/ENI
and smoke VPC are cleanup candidates. Ingress only SSH from operator `/32`;
Console/backend/PostgreSQL/Vault remain private/loopback. IMDSv2 required, response
hop limit 2 for Docker bridge networking. No instance snapshot or EIP was created.

## Observations

The initial provisioning script failed on AWS CLI tag syntax before EC2 creation;
its temporary IAM objects were removed and native absence verified before retry.
The instance bootstrap also needed the downloaded Compose filename aligned with
its checksum filename; checksum verification remained enabled.

Role verification initially rejected a missing `eks:ListClusters` read-probe
permission. The run-owned policy was corrected; IAM simulation and CLI reads
passed before the Go probe, which remained denied during AWS permission
propagation. Read-only Go verification eventually passed with unchanged guards.
The first successful browser registration then hit an ambiguous ARN test locator
(the role and principal fields legitimately match); the locator was corrected and
the confirmed existing account resumed by GET, without replaying registration.
Failures remain recorded; they were not converted to passing runtime evidence.

Real browser registration, rename, reverify version 3 and reload: PASS.
Account key `ec2-instance-role`, authentication `AWS_EC2_ROLE`, no secretRef or
key material in Account DTO; no credential replacement controls.

Terraform VPC `vpc-0ce1bb68cdfa0339c`: apply and destroy PASS, repeated retrieval
through the authenticated role credential endpoint PASS. Native
`DescribeVpcs(exact ID)` returned `InvalidVpcID.NotFound` afterwards.

Final-source backend/frontend rebuild and restart: PASS. The same account
persisted at version 3 and explicit reverify reached version 4. SQL read-back:
AWS_EC2_ROLE, version 4, empty secret_ref, unchanged RoleId. All four runtime
services (frontend/backend/PostgreSQL/Vault) were healthy. Desktop/mobile
screenshots were checked; mobile had no horizontal overflow.

EC2 `i-0223a2df41d1bfac0` terminated. Run-tag native API checks returned zero
VPCs, EBS volumes, ENIs, security groups and key pairs; IAM GetRole and
GetInstanceProfile returned NoSuchEntity. The borrowed default VPC remained.
The verification VPC exact-ID absence was proven before EC2 termination.
Cleanup completion: 2026-10-20T11:23:56Z. No paid run resources remain.
The independent local supervisor and task SSH tunnel were stopped after proof.

Safe [receipt](assets/issue6-ec2-role/receipt.json),
[initial permission failure](assets/issue6-ec2-role/initial-failure.json),
[EC2 role form](assets/issue6-ec2-role/ec2-role-form.png),
[desktop](assets/issue6-ec2-role/driver-accounts.png),
[mobile](assets/issue6-ec2-role/driver-accounts-mobile.png). No temporary AWS keys,
metadata tokens, Vault material or Terraform state are published.

## Verification limits

No cross-account AssumeRole, OIDC, Roles Anywhere, full EKS/Aurora/ElastiCache
provisioning or natural multi-hour credential-expiration wait. Local refresh tests
exercise changed credential values and changed-role rejection; the live test
proves real repeated credential resolution and Terraform consumption. The existing
EKS target consumers pass local tests, without a new live EKS cluster.

Database design continues the existing fresh-baseline model. The new auth enum
changes its digest; an older database fails safely, with no migration/reset.
Retained resources must keep the previous release until a separately authorized
baseline transition. No existing user files, sessions or cloud resources removed.
