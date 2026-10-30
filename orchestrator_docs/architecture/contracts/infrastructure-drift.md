---
id: INFRASTRUCTURE-DRIFT-CONTRACT
artifact: operation-contract
status: current
last_reviewed: 2026-10-28
related: [ADR-030, ADR-014, UC-08, UC-09]
---

# Infrastructure drift scanning and reviewed repair

Required design under [ADR-030](../decisions/ADR-030-delivery-reconciliation.md).
UC-08 INF requirements own behavior. [State contract](terraform-state.md) still
owns encryption, identity, locks, owner fencing, process-stop and cleanup rules.

## INF-C01 — Baseline and eligibility

Scan only acknowledged managed Terraform instances in READY or UNREFERENCED;
preexisting external clusters, unknown outcomes, destroyed instances and orphan
objects are excluded with a reason. Infrastructure scanning is independent of
the optional Environment CD selection. A platform-level scanner flag defaults
off until private baseline persistence and credential access are configured;
when enabled, default cadence is 15 minutes, jittered and configurable.

Persist each successful execution's immutable input baseline: module/provider/
binary pins, normalized variable values or immutable secret version refs,
provider/target identities, dependency instance/output revisions, output mapping
and state lineage/serial. Full input values are encrypted using the State runtime
boundary. Do not expose hashes of secret values or copy raw input into public
plan JSON. Resolve the same secret version, never a mutable latest value.
If a dependency's lifecycle/state version advances, resolve its original
acknowledged baseline by the exact captured versions. The newer acknowledgement
is compatible only when private outputs (including type/sensitivity), identity,
module, output mapping and provider/target pins remain equal and state lineage
is unchanged. Compare immutable encrypted baselines; never substitute today's
Definition, variables or live values into a consumer scan. Changed outputs or
missing historical proof produce `BASELINE_UNAVAILABLE`.
An old instance without this baseline is `BASELINE_UNAVAILABLE`; establish it on
the next authorized successful execution. Do not reconstruct it from today's
Definition, uncommitted draft or live resources.

## INF-C02 — Scan algorithm and results

1. Queue/deduplicate by Environment and baseline version. Skip busy/deleting
   Environments; scans never steal execution locks. One active scan per Environment.
2. Claim `DRIFT_SCAN` Environment operation, then the original state lock with
   normal owner/fence. Re-read instance versions and baseline before invocation.
3. In an isolated private workspace, load the pinned verified module and copied
   decrypted state, bind original scoped credentials and baseline inputs.
   Run a normal `terraform plan` with refresh enabled. Native HTTP state remains
   unchanged: the scan works on a local copy while the product lock fences writes.
4. Parse configuration changes **and all** `resource_drift` actions, including
   updates/deletes, plus output changes and coverage failures. Do not reuse the
   current cleanup parser's delete-only drift list as a complete report.
5. Persist safe findings: resource address/type, action and field path, never raw
   before/after values or provider diagnostics. Keep `externalDrift` distinct from
   `proposedActions`: an external edit can be accepted by ignore_changes and need
   no corrective action. Outputs-only/provider-computed changes are not silently
   hidden or automatically adopted. Report no-change only after a complete plan.
6. Terminate/confirm process exit, delete disposable private artifacts, unlock in
   the established order. If process outcome is unknown retain the operation hold.

Statuses: `QUEUED`, `RUNNING`, `SUCCEEDED`, `FAILED`, `INTERRUPTED`, `SKIPPED`.
Result: `IN_SYNC`, `DRIFTED`, `UNKNOWN`, `BASELINE_UNAVAILABLE`, `NOT_APPLICABLE`.
Partial findings never produce IN_SYNC. Default scan timeout ten minutes; one
transient retry within the same deadline, none for auth/permission/invalid pins.
Use the same global concurrency budget as background delivery observations, with
Terraform scans limited to one invocation per Environment and two globally.
Report last observation and freshness; a skipped scan is not a fresh clean result.
Persist safe reports for 30 days; retain reports referenced by repair/audit.

`plan -refresh-only` can show external state changes, but does not calculate all
actions to restore configured desired state. `apply -refresh-only` would accept
observed changes into state and is not a remediation action in this feature.
See [Terraform planning modes](https://developer.hashicorp.com/terraform/cli/commands/plan).

## INF-C03 — Reviewed repair

Repair is a separate PE/Admin operation, not UC-05's pure Score Preview. It uses
only the last acknowledged baseline and never includes pending desired changes.
Prepare captures instance/dependency versions, state lineage/serial, account and
module pins; creates a private encrypted saved Terraform plan and a redacted
review of actions. The review expires after ten minutes. State lock is released
after planning finishes; the plan's baseline must be rechecked on confirmation.

Confirm binds actor, Environment, exact selected instances, report/plan digests,
requestId and versions, acquires `DRIFT_REPAIR` and state locks. Re-run plan with
refresh and require equivalent redacted action/field set **and private canonical
plan change digest**, excluding nondeterministic metadata. Changed values/actions
invalidate review even if the public summaries match. Apply the freshly verified
saved plan only after this comparison; no unreviewed expanded action set.

V1 repair permits in-place updates to existing acknowledged resources only.
Create, delete, replace, import, missing-resource recreation, identity change or
provider/module upgrade is blocked and routed to a separate existing reviewed
deploy/replacement/cleanup procedure where supported. No automatic state import.
Validate output contracts and dependency impact before admission; if outputs used
by consumers would change or are unknown, require a normal deployment review
that includes those consumers instead of applying a partial repair. Stable-output
repair acknowledges state and updates observations without rewriting the current
Deployment Set or old Deployment status. Failure retains existing v2 recovery hold.

External changes between review and apply remain possible, as with normal
Terraform execution; ownership checks/provider errors and saved-plan guards must
fail safely. This feature does not claim an atomic transaction with cloud APIs.

## INF-C04 — HTTP and RBAC

Use Environment prefix and error conventions from
[CDR-08](delivery-reconciliation.md#cdr-08--public-api).

| Method/path | Contract |
|---|---|
| POST `E/drift-scans` | PE/Admin, requestId + expectedEnvironmentVersion; queues read-only scan; 202 scanId |
| GET `E/drift-scans` | All app roles, stored summaries, cursor/default50/max200 |
| GET `E/drift-scans/{scanId}` | All app roles, safe results, completeness and timestamps |
| POST `E/drift-repairs/preview` | PE/Admin, scanId, instanceIds, requestId and expected versions; 202 preparation operation |
| GET `E/drift-repairs/{reviewId}` | PE/Admin, safe plan summary, blockers, expiry and review token when ready |
| POST `E/drift-repairs` | PE/Admin, reviewId/token + requestId; fresh admission; 202 operationId |

Safe error codes: `DRIFT_BASELINE_UNAVAILABLE` (422), `DRIFT_REPAIR_BLOCKED`
(422), `DRIFT_REPAIR_STALE` (409). Unknown process/state acknowledgement uses the
existing recovery hold and `RECOVERY_INCOMPLETE`; no provider diagnostics or
private plan comparison digest is returned. Repeat confirmation with the same
requestId returns the original consumed operation, never applies again.

Repair uses the freshly verified saved plan in a private local-state workspace
under the original product state lock. State acknowledgement goes through the
existing State service's scoped UPDATE capability, running attempt, lineage,
serial and fence checks; the scan's PLAN capability cannot acknowledge state.
A failed apply/output/inventory/readiness/acknowledgement keeps private recovery
artifacts and the Environment hold. Compare stable root outputs and original
physical inventory before acknowledging; no Deployment Set/history rewrite.

Background scan capability authorizes provider reads only; it cannot be used to
submit apply/destroy or write Terraform state. A repair gets its own scoped
execution capability under the existing State service. No plaintext saved plan,
state, secret ref or provider output is returned to Console. Discard expired
private plans after 24 hours; preserve unresolved recovery artifacts per ADR-014.

Retention runs as an explicitly scheduled database-owner maintenance command,
separate from the Console/State processes. It validates the installed baseline
and catalog, processes bounded batches, and preserves active locks, unresolved
attempts/operations and audit-referenced scans. Runtime roles gain no DELETE,
DDL or function privileges. Private expired plan IDs in retained safe reviews
are historical references; confirmation already refuses them after ten minutes.
