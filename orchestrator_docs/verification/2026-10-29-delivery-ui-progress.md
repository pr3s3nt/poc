---
id: VERIFY-DELIVERY-UI-20261029
artifact: verification-record
status: current
last_reviewed: 2026-10-29
---

# Optional CD UI verification — work in progress

Branch `features/reconciliation`; not merged. This records executed happy paths,
not completion of ADR-030 or R01–R08. Product mutations used ordinary Console
forms, with real task-owned Fleet v0.15.2, HTTPS Git, PostgreSQL, Vault/VSO,
Terraform and Kubernetes. No Score/JSON/YAML editor or product mutation API was
used. Native read checks and platform RBAC preparation are separate from UI work.

Executed in the private `orch-reconciliation-ui-20261029` Compose project and
`orch-reconciliation-20261028` kind incarnation:

- Registered cluster account, Vault store and managed Fleet binding through UI;
  verified binding READY. Staging opt-in left production DIRECT.
- Created frontend/backend/worker and shared PostgreSQL/Redis dependencies through
  ordinary workload forms; Preview/Deploy succeeded. App UI showed five checks
  PASS: backend connection, environment, secret, database and cache. A job entered
  through its form reached DONE through the worker/PostgreSQL/Redis flow.
- Updated backend environment through workload form/Preview/Deploy; verified
  unchanged Deployment, Service, StatefulSet and PVC UIDs.
- Fleet → Direct → Fleet handoff preserved UIDs and Pod templates. First run
  exposed a Console exception for a delivery-only review without graph nodes.
  Fixed the progress reader and added a regression test. A fresh complete round
  trip through the updated UI passed.
- Discovered and verified a preexisting GitRepo through the CD page and transferred
  to its dedicated `existing/orchestrator` subtree. All six BundleDeployments
  became ready; parent UID/spec and independent sibling ConfigMap UID/data stayed
  unchanged. The parent was installed only as task-owned platform setup.
- Marked all three workloads for removal through UI and deployed the removal;
  Deployments disappeared while Terraform dependencies remained for app cleanup.

Application deletion exposed missing direct renderer Secret cleanup on the new
coordinator path. Terraform correctly refused namespace destruction; PostgreSQL
and Redis were destroyed, while the namespace and two renderer Secrets remained.
The fix retires direct auxiliaries after controller withdrawal, and the deletion
retry uses the original persisted target/owner for already retired workloads.
The UI deletion retry passed after the fix: Application GET returned 404,
namespace and PVs were absent, while parent GitRepo UID/spec and sibling
ConfigMap UID/data were retained. The retry image used the original task
database baseline/grants in a private build copy, without an implicit upgrade or
reset; observation workers were disabled in that cleanup fixture. A fresh separate
Compose database is now running the current complete image for independent
observation/self-heal acceptance. Full ADR-030 acceptance is still not claimed.

Independent checks: frontend typecheck/lint/build and 933 historical tests passed
before latest changes; subsequent focused CD/Settings tests 33/33, review/home
28/28 and new reconciliation-panel tests 2/2 passed. New stored observation lease
and persistence checks ran against real PostgreSQL with REQUIRE_POSTGRES=1.
Observation collection uses independent read-only leased/fenced work, exact
admitted receipts, separate dimensions and stale-to-UNKNOWN projection. The
current-image Console displayed all five admitted units in sync, with no drift;
self-heal off then detected a controlled replicas change as
OUT_OF_SYNC/HEALTHY/DETECTED, with the applied off-policy revision retained.
Self-heal opt-in and explicit recovery passed through the current Console.
The recovery retained all native UIDs/Pod templates, activated the selected
policy, and Fleet restored replicas after a second controlled manual edit.
Policy-only updates now
reuse admitted artifacts without Terraform/rendering or completed Job replay.

The policy run exposed a metadata field ownership conflict after historical
direct-to-Fleet adoption: `kubectl-patch` Update owned the platform revision
annotation, conflicting with Fleet's SSA Apply manager. Sharing the manager name
does not resolve Update-versus-Apply ownership. The corrected fenced handoff
relinquishes only platform metadata fields from known handoff Update managers,
preserving all business/spec/status values and field owners. Real Kubernetes
conformance passed a subsequent Fleet SSA revision with unchanged UID/data and
UID-checked fixture cleanup. Explicit recovery now validates the exact persisted
publication/controller receipt, rebinds the interrupted SUBMIT fence without
resetting it to PENDING, and observes instead of submitting again. Unit tests
prove no Job replay, UID change or repeated adoption, including interruption in
the healing phase after handoff. Real PostgreSQL rebind checks passed. The
browser recovery passed after correcting Settings heartbeat polling and the
manual Update ownership conflict described below. Fleet's
controller backoff can outlast the ordinary five-minute deploy deadline, so
explicit receipt recovery allows twenty minutes of observation without force
sync or another publication.

An already drifted replicas field also prevented the initial SSA policy upgrade.
The confirmed policy now relinquishes only declared `kubectl-*` Update ownership
under the exact source UID/release and current fence. It does not change live
business values; Fleet performs the correction. A real ConfigMap conformance
proved that distinction and preserved UID/data through subsequent SSA. Tests
preserve undeclared/status fields, injected sidecars, other Apply managers and
HPA delegation; live HPA collision blocks explicit replicas reclamation. Native
Fleet corrected the second manual replicas edit after policy activation, without
another Orchestrator submission. The existing GitRepo/current-image lifecycle
and cleanup continuation are still in progress.

The independent Terraform private-plan parser covers all external updates/deletes,
proposed actions, changed paths, unknown outputs and secret-dependent comparison
digests (excluded from public JSON), with focused tests. It follows the official
[Terraform plan format](https://developer.hashicorp.com/terraform/internals/json-format).
Infrastructure baseline/scanner and reviewed repair runtime are now implemented;
their current acceptance is recorded below. Full required checks and remaining
semantic gates must pass before merge.

The current delivery application cleanup completed through UI after explicit
UID-proof withdrawal recovery and waiting for downstream BundleDeployment/Helm
history retirement. Workloads and Application were removed; namespace/PVs were
absent, parent GitRepo UID/spec and sibling ConfigMap UID/data retained. The
current and original delivery application runs both have native cleanup proof.

## Infrastructure continuation on a fresh baseline

The independent `orch-reconciliation-drift-20261029` Compose project uses a fresh
database with Terraform execution baselines, safe scan queue/reports and State-only
repair reviews. Previous project databases were not reset/upgraded. Ordinary UI
forms registered its account/Vault/CD binding and deployed the real application;
five business checks, queued job and valid configuration update passed.

E09 passed through the Console. Initial scan covered all three managed instances,
with complete reports, no proposed actions/output changes and namespace IN_SYNC.
Kubernetes/provider computed/defaulted PVC/StatefulSet fields were reported as
external drift without proposed actions, as required by INF-C02; they were not
hidden to force a green aggregate. Canonical encrypted-state rows/serial/lineage
and current Set stayed unchanged. A UID-tested edit of the owned namespace's
declared provenance annotation was then detected as an in-place proposed update,
again without adopting state. Review of all drifted instances correctly blocked
computed-only instances with no repair actions. The UI now offers only instances
with proposed actions in the repair selector; namespace repair confirmation is
still in progress. No E10 completion or final merge is claimed.

New focused evidence: real PostgreSQL scan fences/idempotency and single-use,
actor-bound private review consumption passed; real Terraform private-copy scan
preserved canonical state and encrypted inputs. A real PostgreSQL/Terraform test
with explicitly fake drift facts proved that changed private values invalidate
review before Apply even when public summaries match. This is an admission test,
not real provider repair evidence. Console scan/review tests cover role-specific
actions, explicit confirmation and same-request lost-response retry.

Private screenshots, browser/session files, run state, Git writer credentials,
TLS keys, kubeconfigs and logs live outside Git under the task cache directory.
They are not embedded in this record. The runner preserves failed-attempt images
and resumes acknowledged work without replaying a mutation.

### Infrastructure repair continuation

The fresh `orch-reconciliation-drift-20261029` Console run detected the owned
namespace annotation drift and confirmed an in-place repair through the review
dialog. Its consumed review points to a SUCCEEDED DRIFT_REPAIR operation; the
original namespace UID and current Deployment Set remained unchanged and the
annotation was restored. The follow-up scan exposed a consumer-baseline defect:
PostgreSQL/Redis rejected the provider's advanced lifecycle/state versions even
though namespace outputs were stable. Acceptance remains pending until a new UI
scan and application business checks prove the fix.

A real PostgreSQL/Terraform regression proves historical dependency comparison:
a stable-output acknowledgement permits consumer inspection; changed outputs
reject inspection before invoking Terraform. Runtime continuation preserves the
owned database and its exact SQL baseline hash. The only fixture permission
update is the new least-privilege State SELECT on `environments.version`; no
schema/reset/marker change or retained-database migration claim is made.

The fixed runtime continuation passed: a new UI-requested scan completed for all
three acknowledged instances, with the namespace IN_SYNC and no proposed actions
or output changes. Provider-computed drift on other resources remains visible.
The runner resumed verification from the exact original CONSUMED review and its
SUCCEEDED confirmation operation, without reapplying the repair or repeating the
controlled edit. Application checks and old/new queued jobs passed through the
application UI. E09/E10 are now accepted for this owned target.

`drift-ui-cleanup-1.log` proves subsequent UI workload removal and Application
deletion, namespace/PV absence and unchanged retained GitRepo/sibling identity.
The first post-upgrade browser launch preceded frontend startup and made no
product writes; the next launch waited for HTTP readiness and completed.

Owner-only retention also passed real PostgreSQL tests: busy/locked or unresolved
instances keep private plans; after settlement only an expired plan and old
unreferenced safe scan are removed. Recent plans/scans and immutable execution
baselines remain; runtime logins cannot invoke owner maintenance; a repeat is a
no-op. This is automated persistence evidence, not another provider repair run.

### Final validation and extra object run

The frontend full run exercised 944 tests: 943 passed; one older observation
component expectation still named the internal `ROLLOUT` enum after the UI phase
label was translated. Updating that assertion to the visible Vietnamese label
passed the focused file (2/2); typecheck, lint and production build passed.
No production behavior/assertion was removed. Backend full validation found a
baseline-check false positive: its substring ban on `IF NOT EXISTS` also matched
new PL/pgSQL validation guards. The checker now targets conditional DDL, retaining
the fresh-only/no-upgrade rule. Focused fresh-baseline, retention and review checks
are being completed; full results remain to be recorded.

The extra-object Console run uses an uploaded render bundle and ordinary
application forms. Initial deployment and five application checks/queued job
passed. A real PostgreSQL migration Job completed once, a CronJob child completed,
HPA established two frontend replicas, and the public Ingress was applied.
Configuration update passed; provider handoff/semantic retirement verification
is still running. This supplements rather than substitutes for earlier cleanups.

The first semantic transfer was safely refused before RELEASING because the
fixture lacked Metrics API and Fleet reported the HPA NotReady. No destination
adoption/source withdrawal ran for that operation. The owned kind prerequisite
now installs official Metrics Server v0.8.1, records object UIDs and waits for
metrics availability/ScalingActive. An initial multi-platform Docker import
failed before Kubernetes creation; selecting linux/amd64 fixed the fixture.
The original desired provider choice remained pending and is reviewed/retried
through Preview/Deploy, not marked as already applied.

Final-image fresh provisioning passed: a separate untouched Compose project
`orch-reconciliation-final-20261030` initialized the matching SQL baseline/grants
and its backend/frontend are healthy. No manual permission patch was needed on
this final image. The owned earlier runtime is retained for the semantic app's
completion/cleanup; its baseline is never reset to install the retention schema.

Latest continuation: the complete tfexec package passed with real PostgreSQL in
875.639s using `go test -timeout 30m ./internal/application/tfexec`. The earlier
whole-backend invocation hit Go's default ten-minute package timeout while an
otherwise ordinary test was running; the extended run retained all assertions.
All other packages completed; the sole baseline substring assertion failure was
fixed and its focused baseline/retention/review run passed (39.845s). Backend
`go build ./...` passed. Final-image owner retention command also passed with
zero eligible rows, and fresh image startup verified exact grants.

The semantic Fleet→Direct transfer after Metrics API readiness is INTERRUPTED at
ADOPTING. All original source WITHDRAW intents acknowledged; destination ADOPT
acknowledged for route, migration Job, backend phases and frontend prerequisite.
Frontend ROLLOUT is RUNNING without a receipt; worker remains PENDING. Native
frontend Deployment and HPA already carry direct ownership metadata, indicating
an interruption during/after metadata transfer, before persisted receipt. Do not
retry a new deployment, reset intents or touch business specs. Diagnose the exact
metadata-release failure, then stop/recreate the owned prior worker and use the
original operation's explicit UI recovery. Completed migration UID/template and
transfer snapshot remain in private `ui-state-semantics.json`. Semantic cleanup
and merge are still pending.

Subsequent metadata diagnosis used server-side dry-run only: the current native
Deployment/HPA accept the guarded managedFields patch. Metadata release now retries
at most three times only when a rejected request is followed by proof of the same
UID and a changed resourceVersion, repeating all ownership/revision checks.
Focused Kubernetes/direct adapter checks passed (12.132s / 0.025s). Recovery-image
preparation preserves the exact owned database baseline bytes/hash in a private
compatibility build; canonical product schema/grants are not rewritten to match
an old fixture. The semantic UI recovery runner now checks its own project,
stopped-worker proof, original snapshot and applied Direct selection.

Semantic transfer recovery passed through Settings after the original worker was
stopped and replaced with the byte-matched fixture recovery image. The original
operation is RECOVERED/COMPLETED; all original object UIDs and Pod templates,
completed migration receipt, retained parent GitRepo and sibling survived. The
new UI run resumes from applied Direct and will exercise the reverse handoff and
remaining HPA/retirement gates. Source metadata retries remain bounded and fenced;
no operation reset or business-resource reapply was used for recovery.

The user's latest instruction supersedes the earlier merge authorization:
finish checks/cleanup and commit/push **only `features/reconciliation`**; never
merge this task into main.

Reverse transfer diagnosis identified the failure specifically at Git publisher
submission. The task-owned HTTPS fixture was not listening on port 19443 (native
socket/HTTP connection refused), while the persisted checkout remained clean.
Restarting that exact fixture reused its repository, original certificate and
private writer credential. The prior worker was stopped and recorded again;
explicit reverse UI recovery then passed with stable UIDs/templates/completion
and parent/sibling identity. Temporary stage-only diagnostic logging existed in
a private compatibility build only and was removed by returning to the clean
recovery image before the successful recovery. No such logging is product code.

Bounded metadata retry regression passed all five cases: same UID/new version
retries successfully; unchanged version, replacement UID and permission failure
do not retry; repeated version churn stops after three attempts. No business
fields or UID guards are relaxed.

### Completed acceptance — 2026-10-30

`ui-semantics-5.log` passed all remaining assertions: both handoff directions,
original completed SQL migration Job UID/template/completion, scheduled successful
CronJob children absent from Git inventory, HPA replica authority after explicit
self-heal, and removal of the retired ConfigMap through template ZIP update and
Preview/Deploy. The migration did not rerun; the application retained all five
checks and old/new business jobs. `semantic-ui-cleanup-1.log` then passed workload
removal and Application deletion, namespace/PV absence and unchanged retained
GitRepo/sibling. All four private application state records report cleanupPassed.

Required validation is complete using full runs plus focused correction checks:
Go packages with real PostgreSQL; tfexec full package at CI's 30m timeout; backend
build; frontend typecheck/lint, 944 test coverage with the sole stale-label
assertion corrected and retested, production build; E2E helper 6/6; bounded
metadata-race five-case regression; script syntax, docs/PlantUML and diff checks.
No AWS/shared-cluster tests ran: authorization covered only this owned disposable
platform. Argo CD, separate management topology and Job TTL are not claimed.
UI reference used the actual AppShell, Secret Stores/Settings and shared Dialog/
content-panel patterns. Visual inspection of CD forms, observations and repair
review retained the current shell; summaries/details keep private values absent.
No recording requirement or intentionally failing runtime detour was added.

The task outcome is accepted for the implemented capability set and remains on
`features/reconciliation` as requested; no main merge is authorized.


Platform teardown follows accepted product cleanup and exact private proofs for
all four Compose projects, the PostgreSQL test fixture, task Vault and kind
incarnation. Generated logs/credentials/browser evidence remain outside Git.
The clean final product image was built from canonical source after the metadata
retry change; compatibility/diagnostic images were fixture-only and are not
production deployment instructions.
