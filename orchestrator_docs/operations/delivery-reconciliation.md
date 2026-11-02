---
id: DELIVERY-RECONCILIATION-OPERATIONS
artifact: runbook
status: current
last_reviewed: 2026-10-30
related: [ADR-030, DELIVERY-RECONCILIATION-CONTRACT, INFRASTRUCTURE-DRIFT-CONTRACT]
---

# CD application delivery and infrastructure reconciliation

Read [ADR-030](../architecture/decisions/ADR-030-delivery-reconciliation.md),
[delivery contract](../architecture/contracts/delivery-reconciliation.md) and
[infrastructure contract](../architecture/contracts/infrastructure-drift.md).
[Current state](../CURRENT_STATE.md) records remaining release gates; this runbook
alone does not claim an accepted release or authorize an external deployment.

## Platform prerequisites

Use the current PostgreSQL schema (version 5, including platform defaults and repository allocations; fresh install or versioned owner upgrade) with separate ordinary Console,
State and owner logins. Setup is the explicit owner command `-setup-database`;
API startup only verifies the baseline and exact grants. Keep database URLs and
State encryption key in their existing private file mounts. Retained databases
follow the [versioned upgrade procedure](postgres-upgrades.md); do not reset them, edit baseline markers,
backfill guessed UID/baseline evidence or grant runtime owner privileges.

Real adapters require durable scoped credential/output stores, the existing
State runtime and pinned Terraform/score-k8s binaries. Preserve the encryption
key, verified module/render caches and encrypted recovery volume across restarts.
Application delivery requires a private durable directory outside Git:

```text
-delivery-git-root /var/lib/orchestrator/delivery-git
```

The equivalent environment variable is `ORCHESTRATOR_DELIVERY_GIT_ROOT`. Empty
keeps CD unavailable and blocks real application deployment. The process-wide
`-workload-delivery` flag is removed; each Environment requires its own binding.

Install Fleet **v0.15.2** controller/gitjob/agent on explicitly authorized
infrastructure. Register the downstream target and verify its Cluster UID. The
v1 adapter accepts the verified same-cluster `local` registration; a separate
management/downstream topology is currently rejected. Fleet owns only
workload/route objects, never the Terraform namespace, PostgreSQL/Redis resources
or credentials. Use a separate release-storage namespace and a workload
ServiceAccount with permissions only for authorized workload namespaces and
release Secrets. Provision those platform permissions after Terraform creates
its namespace; do not precreate/adopt the application namespace to bypass it.

Configure GitHub owner and private HTTPS writer credentials on the platform. The
credential must create private initialized repositories in that owner and write
Git commits. A personal owner must match the authenticated identity; an organization
requires active membership and organization policy allowing repository creation.
Read-only verification checks identity/membership; creation permission is finally
proven by Deploy. GitHub Enterprise requires explicit HTTPS API and clone roots.
Redirects are refused. Git CA configuration applies to Git/Fleet; the GitHub API
uses the host system CA trust (install an enterprise CA there if needed).

Create the Fleet Git reader Secret `orchestrator-git-reader` in the workspace,
using a credential that can read the generated private repositories. Prepare
`orchestrator-delivery` in the discovered agent namespace and `fleet-releases`
with the scoped permissions described above. These are platform prerequisites;
this feature does not install Fleet or manufacture READY verification.

## Console flow

1. Platform Engineer/Admin opens **Cấu hình CD**. Enter a name, choose the
   management cluster account, click **Tìm cấu hình trên cluster**, and enter
   GitHub owner, Git username and token. Advanced fields retain platform
   topology/defaults and support a different setup. Save, then verify to READY.
2. Click **Dùng cho ứng dụng mới** on the verified configuration. This selects
   an organization default with a revision guard; it affects new Applications
   only. Editing creates an immutable version and does not move existing apps.
3. Create an Application using name/subdomain. Its two empty Environments pin
   the platform default. No Git request happens during creation or Preview.
   Application Settings has no repo, branch, path or CD selection form.
4. Configure workloads, Preview and Deploy normally. First admitted Deploy
   reserves a durable allocation, creates/observes one private initialized
   `orch-app-<Application UUID without hyphens>` repository and acknowledges its
   actual remote ID/default branch. Environments publish under
   `environments/staging/manifests` and `environments/production/manifests`.
   Terraform provisions dependencies; Fleet delivers prerequisites → Job →
   rollout → routes. Placement must still match Definition-derived accounts.
5. Home reads stored synchronization/health/drift. Observation/removal resolves
   recorded destinations without repository creation. Cleanup retains the remote
   repository and its history, shared reader Secret and platform registration.

If creation loses its response, retrying Deploy observes the same allocation and
creation marker. Do not create a new repo name manually or edit private database
records. An existing same-name foreign repo or replaced acknowledged remote ID is
refused. A failed allocation is retained for diagnosis/retry. A lost set-default
response requires **Tải lại** to read the authoritative default before another write.

Kubernetes Jobs reject TTL and immutable template changes; an acknowledged
unchanged Job is reused. CronJob children belong to Kubernetes, not Git inventory.
HPA targets must omit fixed replicas. Unsupported/missing ownership evidence
blocks deployment rather than falling back to a competing direct writer. See
[release semantics](../architecture/contracts/delivery-reconciliation.md#cdr-06--fleet-adapter-v1).

## Existing test Applications — ADR-033

Owner setup upgrades recognized schemas through migration 005 and preserves data.
No old-Application migration, Direct adoption or destination handoff is provided in
this product flow. Existing test Applications are kept for inspection; create a new
Application after setting the platform default to use the managed repository flow.
Do not reset the database or delete test Applications as part of upgrading.

New deployments publish nonsecret auxiliaries and scoped VSO declarations through
CD. Select a usable Secret Store when dependency outputs contain secrets, even
without Application secret keys. VSO destination Secrets must exist, have the
expected owner and contain every required key before Jobs/rollout proceed.
Vault Agent receives private immutable references instead of a direct env Secret.
CD secret-delivery ServiceAccounts use distinct revision-scoped identities so
old direct auxiliaries are never implicitly adopted. Grant the CD workload
account the required namespaced VSO/ServiceAccount access;
public CA manifests additionally require scoped Secret permissions. No secret
values or private keys belong in Git. Existing non-CD auxiliary cleanup remains
only for historical resources and retains their original deployment identity.
Fresh CD inventories are withdrawn/pruned/deleted through the CD adapter; VSO
children disappear through their owning controller/garbage collection.

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
