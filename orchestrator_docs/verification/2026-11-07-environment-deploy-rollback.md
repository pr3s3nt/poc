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

## Acceptance review — not passed

The full tfexec package run reported a failure in
`TestDestroyUsesRetainedHistoricalReleaseNeverTheCurrentOne`, then reached the
10-minute package timeout while another cleanup test was running. It did not pass.

An isolated comparison used the same disposable PostgreSQL/Terraform environment,
REQUIRE_POSTGRES=1, REQUIRE_TERRAFORM=1 and a three-minute test timeout:

```sh
go test ./internal/application/tfexec -count=1 -timeout=3m -run '^TestDestroyUsesRetainedHistoricalReleaseNeverTheCurrentOne$'
```

- `origin/main` at `b2d605f9`: failed (24.340s).
- Feature at `f08ef8aa`: failed identically (24.303s).
- Both report that destroy succeeded without the required retained release,
  contrary to the test expectation. This isolated failure predates the task;
  the full-package timeout was not independently compared with main.

The feature is not accepted: Environment rollback/API/UI are incomplete and the
existing Boutique failure has not been exercised with the new feature. The user
requested merge into the prior working branch only if acceptance passes; therefore
no merge into `features/organization_seed` or main was performed. No runtime
upgrade or complete product acceptance is claimed.

## Existing Boutique target

Read-only verification found the authorized kind context in the Ubuntu WSL
distribution. The node is Ready. Boutique has seven Ready Deployments and Redis;
payment has a Service but no Deployment or Pod. No cluster objects were mutated.

## Remaining work

Environment release snapshots, user-selected rollback execution, Console actions,
rollback tests and the real Boutique deployment test remain incomplete. Existing
per-workload history must not be presented as a successful Environment release.


## Successful Environment checkpoint slice

Pending Deploy now checkpoints the entire resulting Deployment Set, configuration
revision, Definition snapshots and delivery/target pins before marking its
Environment Operation successful. A reader verifies scope, metadata digest and
immutable document hash, and refuses missing historical checkpoints.

`TestSuccessfulEnvironmentReleaseSurvivesPartialDeploy` and
`TestEnvironmentReleaseSurvivesPartialDeployAndPostgresReopen` passed together
(12.292s), with REQUIRE_POSTGRES=1 for real PostgreSQL metadata persistence.
The test deploys two workloads, succeeds on the first update and fails on the
second, then proves the earlier complete release remains selected despite the
new hybrid current Deployment Set. A fresh PostgreSQL connection sees the same
checkpoint. Workload execution in these checkpoint tests is explicitly fake;
this is not live Kubernetes rollback acceptance.

A broader pending-package run failed preexisting renderer-fixture expectations;
it also exposed the new test's initial expectation for uncheckpointed seed history,
which was corrected to expect a refused historical baseline. The focused rerun
above passed; the package-wide renderer failures remain unresolved.

Read-only Boutique Operation history contains no SUCCEEDED Deploy operation;
previous Deploy operations are Failed/Recovered and payment remains Interrupted.
Seven successful workload deployments do not establish a successful Environment
release. The requested initial rollback target is therefore empty; no workload
or resource was deleted by this checkpoint slice.

## Existing Boutique recovery passed through UI

Shared-controller recovery now checks whether Git rendered the admitted commit
before waking its BundleDeployment. It requests at most one guarded Git render
per intent/commit after verifying receipt/controller identity and all sibling
trees; active Job units block this render request. Full Fleet package tests passed
(15.659s), including bounded render and Job/foreign-controller rejection tests.

The existing cluster API forward had stopped. It was restored using the existing
trial's port and destination. No RBAC change was made; an earlier hypothesis about
missing patch permission was not substantiated and was discarded.

The existing `online-boutique/recover.mjs` runner passed via actual Console UI.
The original payment operation is now RECOVERED and staging has no active claim.
Fleet GitJob is Current. Before/after exact namespace Service/Deployment/Pod UID
inventories match; the payment Service, seven workload Deployments/Pods and Redis
were preserved. No new workload deploy or rollback was triggered by recovery.

Runtime uses the task-built backend in the retained isolated Boutique Console,
with a private pre-upgrade database backup. This proves the Busy blocker recovery;
whole-Environment rollback/UI and full issue acceptance remain incomplete.

## Redeploy after recovery

UI Preview passed with all eleven workloads. The renderer was explicitly
reinspected/selected through Console after the runtime rebuild. The first new
Deploy failed without retaining the Environment claim. A runner admission race
was found: it interpreted empty history before Operation admission as terminal,
then closed the browser. The runner now requires the exact reviewRequestId
Operation before reporting completion. Node syntax/docs checks passed.

With the corrected runner and a host-specific private kubeconfig (the container
kubeconfig's host.docker.internal does not resolve on the host), the next real UI
Deploy was admitted and ended FAILED at ad. Namespace Role/RoleBinding were reused.
No subsequent retry was launched without diagnosing this new failure. The original
Busy blocker is resolved; successful complete redeploy and rollback acceptance are
not yet achieved. This new pre-provision failure remains to be diagnosed.

## Pre-provision failure diagnosis

The managed application repository had become public. Its remote ID, allocation
marker, clone URL and branch still matched the persisted allocation, but the
repository-hosting adapter correctly refuses public repositories. The user's
original private-repository requirement was restored on that exact existing repo;
identity and marker were rechecked without replacing it. No actor or time for the
visibility change is established by this check.

Explicit Console renderer repinning and a fresh UI Deploy then passed the previous
pre-provision failure and advanced through ad to cart. Runtime completion is still
pending. Fixed-code deployment diagnostics now classify known repository, delivery,
resource and template failures without logging provider text or secret values.
Focused deployment-service tests passed (17.677s). No successful full application
acceptance or rollback implementation is inferred from this checkpoint.

## Boutique deployment and business acceptance passed — 2026-11-08

After restoring the existing managed repository to the originally requested
private visibility and explicitly selecting the rebuilt renderer revision through
Console, the real UI Deploy completed SUCCEEDED for all eleven workloads. It
advanced past the original payment failure and recorded RELEASE_READY followed
by a successful Environment Operation. No direct API/seed mutation replaced the
Console Deploy steps.

The runtime observer passed: eleven Ready application Pods/controllers, Redis
Ready, correct pinned images and ordinary environment values, compatible security
defaults in app/init containers, ready Service endpoints and VSO-synchronized Cart
Redis Secret reference. Namespace UID was checked against the retained run.

The actual workload browser scenario passed product → Add to Cart → Place Order
and observed the completed-order confirmation on `/cart/checkout`. It used the
Tailscale workload origin; screenshots and private runtime evidence are retained
outside Git. Console credentials and repository/cluster/private URLs are excluded
from public evidence. The shopping runner uses plain `playwright` to avoid loading
two separate copies of the `@playwright/test` singleton via shared helpers; Node
syntax validation passed and the real browser run verified the fix.

The deployment/runtime/checkout acceptance for the existing Boutique app is now
successful. This does not complete the whole issue #49: explicit Environment
rollback execution/API/UI and their acceptance remain unfinished. No merge into
`features/organization_seed` or main is authorized by a partial feature result.
The trial stack, API forward and workload forward remain running for user review.
