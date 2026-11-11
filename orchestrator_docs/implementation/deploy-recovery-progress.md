---
id: IMP-DEPLOY-RECOVERY-PROGRESS
artifact: implementation-handoff
status: current
last_reviewed: 2026-11-11
---

# Detailed Deploy and Recovery progress — issue #69

Feature branch: `features/deploy-recovery-progress`. Not merged or user accepted.

Canonical route: [UC-09 DP](../usecase/UC-09/specification.md#deploy-and-recovery-progress--issue-69),
[realization](../usecase/UC-09/realization.md#persisted-execution-progress--issue-69),
[contract](../architecture/contracts/deployment-progress.md),
[UI](../usecase/UC-09/ui/screens.md#detailed-deployrecovery-progress--issue-69).

## Implementation

The operation's existing JSONB detail stores a generic parent/step timeline and
Recovery check list. The lease serializes snapshots with heartbeats and retains
progress during Stage replacement. Deployment queries correlate the exact
operation, including failures before planning; legacy uncorrelated records retain
the old view. No schema migration or released baseline is changed.

Deploy emits claim, planned workload, plan, resource/structured Terraform stages,
secret preparation, rendering, Git publication, Fleet apply and readiness facts.
Publication never reports apply success prematurely; known Fleet permission/auth
errors are classified without publishing controller text. Fleet diagnostics
are read inside its existing observation; only exact revision/release controllers
and current-revision Pods contribute evidence. Pod/events use safe fields, never
raw error messages. VSO and Job observations remain governed by existing readiness.

Recovery retains the original heartbeat, prerequisite acknowledgements, publication,
controller, object ownership and settlement checks. Failure does not bypass any
ownership/fence guard or release an unresolved operation. All existing readers
can inspect the conclusion and the collapsible check list.

Console detail shows the generic timeline grouped by workload, elapsed/remaining
time, pod image/reason/restarts, latest three events and safe structured Terraform
logs. Environment home and Settings reuse OperationBanner for current step,
warning, detail link and Recovery. Polling reads stored data every three seconds;
no SSE or separate backend observer is added.

## Verification and remaining acceptance

Focused backend suites, race checks, scoped real PostgreSQL reopen/correlation
checks and focused Console checks have passed. Migration immutability, documentation
validation and the changed PlantUML sequence have passed. Full suites run in an
isolated kind namespace and are compared with the main baseline when failures
occur; final counts and cleanup are recorded in the
[dated verification report](../verification/2026-11-11-deploy-recovery-progress.md).

The real trial has separate Docker volumes and real kind/Fleet connections. Its
preparation uses bootstrap APIs only for platform configuration. A workload and
PostgreSQL draft were authored through Console and produced a valid Preview.
Automated browser observations are technical regression checks, not the user
acceptance specified by this issue. All six user-driven Console cases remain a
separate acceptance gate; do not close the issue or merge before acceptance.

## Trial restart

Use the feature URL runbook, including `deploy/local/compose.bootstrap.yml` before
`deploy/local/compose.feature-url.yml` in the Compose invocation. The bootstrap
overlay enables durable CD publication; omitting it disables Fleet. Keep the same
feature project/image names and volumes. Existing run-scoped credential files,
cluster API/Vault forwards and the shared feature proxy must remain available.
Never put those credential contents or the trial's infrastructure URLs in Git or
the public issue. The user handoff carries the verified Tailscale Console URL.
