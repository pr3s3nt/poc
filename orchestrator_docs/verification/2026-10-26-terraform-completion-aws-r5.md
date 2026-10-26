---
id: VERIFY-20261026-TERRAFORM-COMPLETION-AWS-R5
artifact: verification-record
status: evidence
last_reviewed: 2026-10-26
---

# Terraform completion — AWS acceptance blocked, cleanup verified

The user authorized real complex happy paths on AWS EKS with Aurora and
ElastiCache, followed by merge to main only after successful acceptance.
**AWS acceptance has not passed and the branch has not been merged.**
The independent [kind acceptance](2026-10-26-terraform-resource-completion.md)
remains 57/57 PASS; it is not substituted for AWS acceptance.

## Execution and source

Latest run: `tcaws1026r5`, scenario A02, source
`7cee6d951b976c1ebbdf4a10d0ba28132819df68`, branch `refactor_production`.
The working tree was clean at launch. Main remained
`72d086d3e39201dbc5099bf1c7380ae6f789f477` at the subsequent fetch.
Evidence is outside Git at
`/home/thanhnt1/.codex-runs/aws-completion-runs/tcaws1026r5/`; source, pricing,
checks and coordination receipts are under
`/home/thanhnt1/.codex-runs/aws-completion-prep/`.

Real components: Chromium/Playwright Console actions, Go backend, PostgreSQL
and encrypted HTTP Terraform state, file-backed Vault, Terraform 1.9.8,
AWS provider 6.60.0, Kubernetes provider 2.38.0 and score-k8s 0.15.0. No fake
adapter, mock service or intercepted successful response establishes a result.
The fast browser mode has no recording or presentation-pause requirement.

Target: account `452025861381`, `us-east-1`, principal `itadminaws`. The
Definitions select their own account/region; Kubernetes consumers reference
the EKS output. Sizing: one ARM64 Spot `t4g.medium` node, 20 GiB disk,
EKS 1.35 in standard support, Aurora PostgreSQL 16.14 at 0–1 ACU,
Redis 7.1 `cache.t4g.micro`, no NAT and three owned ECR repositories.
The executor public API allowlist was `59.153.224.137/32`; the private EKS
endpoint remained enabled. Preflight checked identity, permissions, capacity,
pins, AMI/add-on availability, exact resource counts and predecessor evidence.
The independent cleanup supervisor was armed before the first ECR create.

## Observed result

R5 is **FAIL**, not a partial PASS. Twelve Console onboarding/authoring/Preview
assertions passed before provisioning: the graph, per-Definition targets,
module pins, Git render bundle and mutation-free Preview were verified. VPC
reached READY and EKS control plane became ACTIVE, but the managed node group
returned `CREATE_FAILED / NodeCreationFailure`: its EC2 instance did not join.
The bounded Terraform invocation ended with `OUTCOME_UNKNOWN`; the product
held the instance and Environment rather than claiming readiness or retrying
an uncertain apply. Aurora, ElastiCache and workloads were not created in R5.

The failure was not an intentionally invalid scenario. Read-only diagnosis:

- EC2 instance and system checks were OK; the node boot log showed nodeadm
  configuring and starting kubelet/containerd.
- The same EKS-generated security group was attached to the node and control
  plane, but its inbound rule list was empty. Routes and public addressing
  were present.
- CloudTrail recorded `Client.UnauthorizedOperation` for the EKS service-linked
  role's `AuthorizeSecurityGroupIngress` and `AuthorizeSecurityGroupEgress`
  during cluster creation.
- The existing role currently has `AmazonEKSServiceRolePolicy`. Its relevant
  permissions depend on the security group's Name tag. Simulation with the
  current tag returned allowed. This does not prove the authorization context
  at creation time; the precise cause of that initial denial is unresolved.

The missing internal rule is consistent with the node-join failure: AWS
documents the default self-ingress rule and the required cluster/node traffic.
This is an inference from the native observations, not a claim that a permanent
IAM policy omission or Spot capacity shortage was proven.
See [EKS security-group requirements](https://docs.aws.amazon.com/eks/latest/userguide/sec-group-reqs.html)
and [the service-linked policy](https://docs.aws.amazon.com/aws-managed-policy/latest/reference/AmazonEKSServiceRolePolicy.html).
No shared role, policy or account-wide setting was changed.

## Earlier attempts and harness corrections

R1–R3 failed before infrastructure deployment because the historical AWS
browser helper did not match the current locale/editor/folded parameter UI.
Each result remains FAIL and each run closed clean before the next one.
Complete real catalog/template UI preflight then passed.

R4, source `700774f`, successfully deployed all three workloads on real EKS
with Aurora/ElastiCache. A browser-submitted job reached DONE; PostgreSQL
result/worker identity, cache write/hit, natural TTL expiry, DB refill and
reload assertions passed. Valid TTL 45 → 60 updates passed and retained
infrastructure identities. Five deployment records were SUCCEEDED.

An actual owned backend restart changed PID 1454904 → 2614553. The next
deployment failed because the old AWS runner had not configured a durable Git
render cache: metadata persisted but the bundle became UNAVAILABLE. R4 remains
FAIL. Its original supervisor/native API, SQL reopen, all five DESTROYED
instances and local teardown proved cleanup before R5.

The R5 runner sets a durable render cache from its first start, requires a
real restart hook, verifies PID change/health, tracks the replacement process
for teardown and fails promptly on terminal deployment failure. A separate
real Console Git-inspection/backend-restart smoke passed with unchanged bundle
ID/SHA and AVAILABLE status after restart. Its disposable database/roles,
process and private handles were removed; it created no cloud resource.
R5 failed before reaching its planned restart, so that smoke is not presented
as a completed full AWS restart scenario.

AWS provider 6.60.0 also repeatedly planned an in-place authentication-mode
update for the disabled default Redis user in R4: `no-password` readback
versus configured `no-password-required`. A sanitized plan showed only that
attribute changing. Application credentials and infrastructure identities
were preserved. The resulting provider wait is a latency observation, not
proof of a provisioning failure; pins and assertions were not weakened.

## Independent operator cleanup of R5

The original supervisor stopped at NEEDS_ATTENTION because the product claim
was held after the uncertain apply. That original result is preserved.
The operator did not unlock the claim, edit product statuses, retry apply,
or add a product recovery API.

After confirming no original Terraform/provider process was active, the
operator authenticated/decrypted the original state in owner-only storage,
verified the original immutable module/binary/provider artifacts and original
Connection identity, and reviewed delete-only plans against the original
managed addresses. Private local copies of the original EKS and VPC state
were destroyed with the pinned Terraform CLI: 12 EKS and 13 VPC resources.
Original SQL state/claim/status rows remained unchanged. An initial local
state-path mismatch was rejected before mutation; its private failure was
retained and apply was corrected to use the same reviewed state path.

Fresh native verification then proved all run-tagged/exact linked AWS assets
absent, including EKS/VPC/network/IAM/ECR and uncreated managed data services.
The original node is terminated; its exact ASG and instance profile are absent.
There are no remaining EBS candidates in that node's availability zone/launch
window, and the existing CSI/native volume checks passed. Preexisting
service-linked role ARNs, IDs, trust and attached policies were unchanged.

Only after native proof were the exact disposable database/roles and owned
Vault/processes removed. Public evidence passed secret scanning; private state,
keys and credential directories were removed. `manual-closure.json` records
operator CLOSED_CLEAN while retaining the original scenario FAIL and
supervisor NEEDS_ATTENTION receipts. The task-owned shared PostgreSQL container
and its exact recorded volumes, plus copied AWS/PG credential handles, were
also removed. Static build images/caches remain as artifacts, not running
fixtures. No paid resource from these test runs remains; user stacks, clusters,
credentials and tmux sessions were preserved.

## Validation, cost and remaining decision

Real PostgreSQL package results compose to 1,897 test passes and two explicit
kind opt-in skips. The first full command reached Go's default 10-minute
package timeout without an assertion failure; the complete unchanged tfexec
package then passed 96 tests with 30 minutes, matching CI's timeout. Its
partial first result is excluded from the composed count. Go build and the
earlier full frontend 918 tests/typecheck/lint/build passed; current E2E helper
checks passed 144/144, plus lint, Bash/Node syntax, docs and whitespace checks.
[Product CI](https://github.com/pr3s3nt/poc/actions/runs/40262753020) and
[Documentation](https://github.com/pr3s3nt/poc/actions/runs/40262753007) passed
on the exact R5 source before launch.

The current public-price estimate was USD 1.9942 per five-hour run, including
the finite 180-minute execution and 120-minute cleanup budgets. It is not a
budget ceiling or actual charge. The scoped Cost Explorer response remained
Estimated and cannot establish full attribution; actual charges remain unknown.

The optional AWS `ACCEPTANCE_SECRET` diagnostic is explicitly NOT_CONFIGURED
by the established A02 fixture; it is not claimed PASS. Mandatory database/cache
Secret references were real in R4. This record does not claim every possible
AWS resource type, full R5 app readiness, completed AWS acceptance or merge.

The user was asked whether to implement an EKS module with an owned security
group or retain the module and handle AWS authorization externally. That
security/scope choice is pending. No new cloud run starts before cleanup proof
or while that dependent decision is unresolved.
