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

## Rollback implementation checkpoint — 2026-11-08

Whole-Environment rollback now has Preview/token, typed confirmation, one fenced
execution, configuration/immutable Secret-reference restoration, retained
Definition selection and full target-hash verification. Partial rollback retains
an interrupted claim. Initial-empty rollback admits an exact resource inventory,
removes failed workload receipts outside the current Set, and uses the existing
Terraform engine under the same claim for owned-resource deletion.

Focused pending tests passed (34.039s), including PostgreSQL reopen, stale token,
wrong confirmation, restoring configuration/Secret references and partial rollback
hold. Real PostgreSQL/Terraform empty-cleanup tests passed (15.981s), including
concurrent Deploy refusal and unexpected version/status changes blocking deletion.
UI component and Application Home tests passed: 26 tests. HTTP scope/body guards,
resource cleanup tests, backend build, frontend typecheck/lint/build passed.
Lint retains one existing warning in PlatformSettingsPage.

A full frontend run had 978 passing and three failing tests (authoring restriction
VPC selection, legacy Terraform scenario labels and retired import evidence ID).
These require baseline comparison; no full-suite pass is claimed. Existing
RemoveDelivered tests fail on both pre-change HEAD and this working tree; the
single-test baseline reproduction failed before any removal. This is retained as
an unresolved verification limitation, not waived as a pass.

Actual Console rollback preview passed. Actual confirmation exposed an unavailable
historical renderer after the backend rebuild. Runtime rejected execution without
substituting its current renderer; workloads were not rolled back. The interrupted
preflight operation was recovered through UI. Historical-renderer validation now
runs before claiming/changing desired state, and planning failures roll back the
desired-state transaction before releasing a confirmed nonmutating failure.
Successful live rollback and full acceptance remain pending. Boutique's prior
successful deploy and checkout do not count as live rollback acceptance.


## Transaction and guard verification — 2026-11-08

Restoration of desired configuration/drafts and the normal deployment Preview
now share one transaction. A preflight refusal rolls back all desired writes and
ends the operation as FAILED without retaining an unnecessary Busy claim. An
in-memory nested-transaction deadlock discovered during verification was removed;
the focused memory/PostgreSQL release/rollback suite passed again (33.167s).
Configuration, Definition and catalog revisions are rechecked inside that
transaction after admission, including the initial-empty cleanup claim path.

Historical successful renderer selections are checked before claiming or changing
desired state. A missing retained engine is refused; the implementation never
substitutes the current renderer for the original pin. A fresh successful release
using the current renderer is being established through Console UI for live
rollback verification. This is still not a live rollback success claim.

Additional checks: HTTP rollback guards passed (0.262s); resourcecleanup package
passed (0.022s); real PostgreSQL/Terraform rollback cleanup tests passed (16.456s);
backend build passed. Frontend typecheck/lint passed with one preexisting lint
warning. Rollback UI tests passed (2); AuthoringRestrictions passed separately
(4), so its full-suite failure is not reproduced in isolation. The two legacy
scenario failures reproduce on the pre-rollback HEAD; full frontend remains red
and is not represented as passed. Documentation and diff checks passed.


## Interrupted update publication pair

The fresh UI release succeeded for ad/cart but stopped at checkout Prerequisite:
the workload content commit existed remotely, paired metadata had not been
published, and the shared controller still held the prior exact pin. The stopped
operation stayed INTERRUPTED. Existing read-only receipt recovery correctly
refused to treat that prior pin as an acknowledgement of the new revision.

Recovery now receives the exact retained prior provider artifact/receipt from the
coordinator. Under explicit stopped-owner recovery it may complete only the
missing paired metadata after verifying the prior pin/controller UID, native UID
and revision inventory, original metadata, exact new remote content and siblings.
Controller admission uses the existing UID/resourceVersion CAS and normal Fleet
observation. No workload rerender, arbitrary Git overwrite or lock bypass is used.

Full Fleet and delivery package tests passed (19.827s/0.609s). The focused real-Git
transport test passed (3.979s), covering exactly one metadata commit, idempotent
receipt observation, and refusal without Git/controller changes for foreign
metadata, foreign native UID, absent/tampered source and changed controller.
This transport fixture is not a real Fleet acceptance claim. The new live recovery
has reached controller observation; its final result is still being checked.

PostgreSQL Definition/selection tests passed (6.781s). The four RemoveDelivered
tests still fail with the same fake-transport ObjectRemover mismatch observed on
the pre-rollback baseline; this package is not reported as passed.


## Live publication recovery and initial-empty Console checks

The guarded checkout publication recovery completed through Console UI. Its
Prerequisite revision was actually acknowledged by Fleet, the operation became
RECOVERED and the Environment claim was released. No manual database or controller
repair was used. A new release using the final renderer is now deploying.

A separate owned Application with no successful deployment and one pending draft
passed explicit typed initial-empty rollback through Console. The resulting
workload list was empty and the Environment idle. No Deploy was submitted, so
this particular UI case created no Kubernetes resources and is not evidence of
failed-rollout resource destruction. The temporary Application was subsequently
cleaned through the normal Settings deletion UI after its preview proved both
Environments contained no resources, workloads or drafts.

The first full-backend attempt used an incomplete test image/database: missing
redis-server and PostgreSQL trust authentication made specific fixture checks
invalid. It was stopped. A replacement disposable image includes Redis and a
separate password-enforcing PostgreSQL server; the full check is running again.
No product change was made to weaken those tests.


## Full checks and subsequent diagnostics

The full backend check completed with 18 failed packages; the same command on
origin/main completed with 20 failed packages. No package failed only on the
feature. Its one failure name absent from the main full-run output,
TestVersionSixCapturesPopulatedFiveAndProtectsSnapshots, reproduced separately on
main with the same retained-template-sources assertion. The main comparison also
reproduced the cluster-ready timeout, stale subdomain schema fixture, unsafe
source expectation and four RemoveDelivered fixture failures. Package timeouts
and missing Node/Bash in the initial tool image prevent an all-tests-passed claim.
After adding Node/Bash, the affected legacy runner checks still failed identically
on feature/main, including the package timeout. Logs stay outside Git.

The final frontend check after scope-guarded rollback refresh passed 979 tests;
only the two legacy scenario assertions remained failed. Typecheck, lint (one
existing warning), build and 26 focused UI tests passed. The source templates and
shared Dialog/Button patterns were visually checked in the real Console.

Additional real UI deployments stopped at payment Rollout and frontend
Prerequisite; each stopped operation was recovered through the existing UI
without database/controller edits. The latter stopped before a new content
commit. Deployment runtime diagnostics were insufficient to distinguish the
failing stage, so fixed-enum Git/Fleet/Kubernetes categories were added without
logging stderr, URLs or credentials. Read-only Git commands may retry recognized
DNS/transport errors at most three invocations; mutations and permanent errors
are not replayed. Focused Git/Fleet/delivery tests and credential-leak tests passed.
The actual transient cause remains to be observed; no root-cause guess is claimed.

The independent eleven-Pod/Redis/endpoint/security observer passed again, and the
actual browser checkout passed after replacing the fragile trial forward with an
owned reconnecting forward. The final diagnostic runtime is being installed only
after the previous operation was RECOVERED. A new renderer selection/release and
whole-Environment rollback are still required before acceptance/merge.
