---
id: VERIFY-ENVIRONMENT-RETRY-ROLLBACK-20261107
artifact: verification
status: current
last_reviewed: 2026-11-07
related: [ADR-040, UC-06, UC-08]
---

# Environment retry and rollback — issue #49, work in progress

## Implemented slice

Terraform resource execution permits three total confirmed transient invocations.
It verifies acknowledged state before another Apply, rejects unknown outcomes and
unconfirmed process stops, and retains the Environment claim when needed. Resource
attempt deadline and state capability lifetime cover the additional invocation.
No automatic rollback was added.

## Verification

Focused Go integration tests passed in a disposable Docker environment with real
PostgreSQL 16, separate owner/runtime roles and real Terraform. REQUIRE_POSTGRES=1
and REQUIRE_TERRAFORM=1 prohibited silent skips. Exact command:

```sh
go test ./internal/application/tfexec -count=1 -run 'TestBoundedTransientRetry|TestTransientPlanFailureSucceedsOnThirdAttempt|TestSecondTransientFailureWithUnaccountedCreateHolds'
```

Result: pass (56.397s). Coverage includes third-invocation success, exhaustion at
three invocations, permanent/unknown errors without replay, and a second Apply
whose unaccounted creation prevents a third Apply. Documentation checker and
`git diff --check` passed. Docker PlantUML syntax checks passed for the three
changed sequence/state diagrams.

The entire tfexec package run is still pending at this checkpoint. No full product
acceptance or runtime upgrade is claimed.

## Existing Boutique target

Read-only verification found the authorized kind context in the Ubuntu WSL
distribution. The node is Ready. Boutique has seven Ready Deployments and Redis;
payment has a Service but no Deployment or Pod. No cluster objects were mutated.

## Remaining work

Environment release snapshots, user-selected rollback execution, Console actions,
rollback tests and the real Boutique deployment test remain incomplete. Existing
per-workload history must not be presented as a successful Environment release.
