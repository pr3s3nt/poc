---
id: VERIFY-20261028-APPLICATION-DELETION-AWS-CLEANUP
artifact: verification-record
status: evidence
last_reviewed: 2026-10-28
related: [UC-18, ADR-025]
---

# Application deletion AWS attempt and operator cleanup

Run `xdaws1027r1`, A02, source `f542766` based on `4764b11`.
**Original acceptance FAIL; operator cleanup independently verified clean.**
The original runner log ends with `scenario=A02 status=1` and `exit=1`.
The tmux background-command completion message was not an acceptance receipt.

## Failure

The first backend deploy failed before app/jobs/update or Application deletion.
Native EKS `describe-nodegroup` reported `CREATE_FAILED`,
`AsgInstanceLaunchFailures`, `UnfulfillableCapacity`: AWS could not fulfill the
Spot request for the configured node group. No successful AWS UC-18 deletion
is established by this run. The original supervisor reported `NEEDS_ATTENTION`;
EKS and VPC plus three run-owned ECR repositories remained.

## Authorized continuation

User authorization on 2026-10-28 requested push, cleanup, integration and UI
acceptance. STS/account/principal/region guards were rechecked through the
original pinned `v2ctl`, limited to account `452025861381`, `us-east-1`.

- Original PostgreSQL retained VPC/EKS state and immutable attempts. The deploy
  operation was `INTERRUPTED`, the EKS instance `NEEDS_ATTENTION`; guards were
  preserved. No SQL status, claim, state or ownership was changed to force a retry.
- Authenticated the encrypted original states with their original state key/AAD
  and confirmed instance/run/bundle/account/region identity. Verified cached
  module content, provider lock and Terraform binary against recorded digests.
- Reviewed delete-only plans from private copies of that state using the
  original modules: 12 EKS resources, then 13 VPC resources. No create/update
  action or address outside the original managed state was allowed.
- Deleted the three exact manifest-owned ECR repositories through the existing
  guarded cleanup tool. Derived CSI reconciliation and native verification ran
  separately; no volume identity or ownership guard was bypassed.

At **2026-10-27 18:15:47 UTC** (2026-10-28 local), all six original native
layers passed absence verification: delivery, app-resources, namespace, EKS,
VPC and aux. Aux checks cover exact recorded/linkage/run-tag assets, including
EC2/EBS/EIP/network, ECR and IAM. Shared/preexisting service-linked roles were
preserved. The original failed supervisor/result receipts remain unchanged.

## Local teardown and evidence

Archived the original database privately, then dropped its disposable database
and roles with the existing manifest-based tool. Verified the PostgreSQL
container ID against the saved handle and `poc.run-id=xdaws1027`, then removed
that dedicated container/volume. Verified the original Vault PID/executable
arguments against its private run directory before stopping it; absence proven.
The original xoaap test PostgreSQL was also matched to its session creation
record and exact container ID, checked for no client connections, and removed.
The run marker is `CLOSED_CLEAN` with an operator reason, independently of the
original failed acceptance result.

Private receipts, original encrypted state/material and database archive remain
outside Git under `/home/thanhnt1/.codex-runs/aws-appdel-runs/xdaws1027r1/`.
The old kind run `xoaappk1` also has `final_status=0` with Compose/Vault/cluster/
image absence proved. No unrelated retained installation or tmux session was removed.

## Limits

This records recovery and absence, not successful AWS product deletion. No new
AWS deployment was started. Integrated form/deletion acceptance uses a separate
owned kind run; [the procedure](../operations/application-deletion-acceptance.md)
and [AWS guards](../operations/aws.md) still apply to future runs.
