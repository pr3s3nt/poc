---
id: DELIVERY-RECONCILIATION-OPERATIONS
artifact: runbook
status: current
last_reviewed: 2026-10-30
related: [ADR-027, DELIVERY-RECONCILIATION-CONTRACT, INFRASTRUCTURE-DRIFT-CONTRACT]
---

# Optional CD and infrastructure reconciliation

Read [ADR-027](../architecture/decisions/ADR-027-delivery-reconciliation.md),
[delivery contract](../architecture/contracts/delivery-reconciliation.md) and
[infrastructure contract](../architecture/contracts/infrastructure-drift.md).
[Current state](../CURRENT_STATE.md) records remaining release gates; this runbook
alone does not claim an accepted release or authorize an external deployment.

## Platform prerequisites

Use the matching **fresh** PostgreSQL baseline with separate ordinary Console,
State and owner logins. Setup is the explicit owner command `-setup-database`;
API startup only verifies the baseline and exact grants. Keep database URLs and
State encryption key in their existing private file mounts. Retained databases
need a separately reviewed upgrade; do not reset them, edit baseline markers,
backfill guessed UID/baseline evidence or grant runtime owner privileges.

Real adapters require durable scoped credential/output stores, the existing
State runtime and pinned Terraform/score-k8s binaries. Preserve the encryption
key, verified module/render caches and encrypted recovery volume across restarts.
Optional delivery adds a private durable directory outside Git:

```text
-delivery-git-root /var/lib/orchestrator/delivery-git
```

The equivalent environment variable is `ORCHESTRATOR_DELIVERY_GIT_ROOT`. Empty
keeps CD unavailable and the normal direct runtime. Process-wide
`-workload-delivery fleet` is rejected; each Environment selects its own provider.

Install Fleet **v0.15.2** controller/gitjob/agent on explicitly authorized
infrastructure. Register the downstream target and verify its Cluster UID. The
v1 adapter accepts the verified same-cluster `local` registration; a separate
management/downstream topology is currently rejected. Fleet owns only
workload/route objects, never the Terraform namespace, PostgreSQL/Redis resources
or credentials. Use a separate release-storage namespace and a workload
ServiceAccount with permissions only for authorized workload namespaces and
release Secrets. Provision those platform permissions after Terraform creates
its namespace; do not precreate/adopt the application namespace to bypass it.

Provide HTTPS Git with a dedicated writable branch/subtree, CA when needed and
separate reader/writer credentials outside Git. Existing company GitRepo mode
requires the registered parent UID/repo/branch/target/path pins and a dedicated
child subtree. Parent configuration and siblings remain independently owned.
Do not reuse a subtree already managed by another writer.

## Console flow

1. As Platform Engineer/Admin, open **Cấu hình CD**, add a binding, choose the
   management/workload accounts and discover the registered target. Enter Git,
   release namespace, workload ServiceAccount and private credential fields.
   Verify to READY. Editing creates a new immutable binding version; usage shows
   existing desired/applied references rather than silently moving consumers.
2. Open Application Settings and select staging or production. Tick **Dùng CD
   provider**, choose the verified binding and Save. The other Environment keeps
   its own choice. Saving does not deploy. Leave **Tự sửa sai lệch** off unless
   autonomous restoration of admitted continuous configuration is intended.
3. Use the normal workload forms, Preview and Deploy. Review provider transition,
   manifests/phase order and routes. Dependencies provision through Terraform;
   GitRepo publication then delivers prerequisite → completed Job → rollout →
   route phases. A live application changes provider through the explicit
   transfer review, preserving UID/data/templates and completion receipts.
4. Home shows stored synchronization, health, drift and observation time.
   Unavailable/stale observations become UNKNOWN; a healthy app can still drift.
   Refresh reads stored reports and does not trigger a deployment.
5. Remove a workload or delete the Application through Preview/confirmation.
   The product withdraws controller/release authority before UID-checked native
   cleanup. Retained parent GitRepo/cluster registration/reader credentials and
   siblings are not application cleanup targets.

Kubernetes Jobs reject TTL and immutable template changes; an acknowledged
unchanged Job is reused. CronJob children belong to Kubernetes, not Git inventory.
HPA targets must omit fixed replicas. Unsupported/missing ownership evidence
blocks deployment rather than falling back to a competing direct writer. See
[release semantics](../architecture/contracts/delivery-reconciliation.md#cdr-06--fleet-adapter-v1).

## Infrastructure scan and repair

Independently opt in with `-infrastructure-drift` or
`ORCHESTRATOR_INFRASTRUCTURE_DRIFT=true`. Default is off; optional
`-infrastructure-drift-interval 15m` controls cadence (minimum 1m, stable jitter).
No CD selection is required. Existing instances without an immutable encrypted
execution baseline report BASELINE_UNAVAILABLE until their next authorized
successful execution; the scanner never reconstructs inputs from a draft.

On Application Home choose **Kiểm tra sai lệch**. Scans refresh a private copy of
acknowledged state with original pinned inputs/credentials. They cannot apply or
adopt state. Reports distinguish external drift from proposed corrective actions;
provider-computed changes can remain visible with no proposed action.

For an actionable in-place update choose **Xem phương án sửa**, select instances,
prepare and inspect the safe action/field summary. Tick the review checkbox and
confirm within ten minutes. Confirmation is actor-bound, single-use and re-plans
before apply; a changed private value invalidates the review even when visible
actions match. Creates/deletes/replacements, output changes and unknown consumer
impact require a separate normal deployment review. Successful repair keeps the
current Deployment Set/history and physical identity, acknowledges State through
its scoped capability, then queues a fresh scan.

Stop and inspect an interrupted/unknown operation. Establish that the previous
worker/process has stopped before using Settings recovery. Exact persisted
receipts/private artifacts and a new owner fence are required. Missing proof
stays held. Never clear locks, reset intents to pending, re-push guessed content,
force Helm takeover or manually delete live resources to make a retry pass.
An uncertain Terraform repair retains its private recovery workspace/state and
the existing v2 hold; automatic apply retry is not a recovery mechanism.

## Retention and maintenance

Schedule the following once daily as a separate owner maintenance job, using an
owner-only URL file. The API containers must not receive that credential:

```bash
orchestrator -prune-drift-history -database-owner-url-file /run/private/owner-url
```

Each invocation validates the exact baseline/catalog and deletes at most 1000
expired private plans and 1000 old safe scans across at most 1000 eligible idle
Environments. Repeat until counts are zero if a backlog exceeds a batch. Plans
expire after 24 hours; unreferenced terminal scan reports after 30 days. Referenced
safe audit reviews/scans, immutable execution baselines/delivery payloads and
unresolved process/state/operation evidence remain. The command refuses runtime
or superuser logins and adds no runtime DELETE/DDL/function privileges.

## Verification

[Delivery UI runner](../../e2e/scenarios/delivery-reconciliation/README.md),
[infrastructure UI runner](../../e2e/scenarios/infrastructure-drift/README.md)
and [owned setup](../../e2e/setup/reconciliation-kind/README.md) describe precise
prerequisites, side effects and cleanup. Use a disposable authorized target and
private evidence directory; setup/conformance is distinct from application
acceptance. The [execution record](../verification/2026-10-29-delivery-ui-progress.md)
distinguishes actual UI results, automated guards and remaining semantic gates.
AWS verification needs its own authorization and the existing AWS runbook.
