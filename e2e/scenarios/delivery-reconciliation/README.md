# Optional CD through ordinary Console forms

Scope: [ADR-030](../../../orchestrator_docs/architecture/decisions/ADR-030-delivery-reconciliation.md),
[UC-04 CD configuration](../../../orchestrator_docs/usecase/UC-04/ui/cd-providers.md),
[Environment CD](../../../orchestrator_docs/usecase/UC-01/ui/delivery-reconciliation.md).
This scenario is in implementation; no successful application acceptance is claimed.

The acceptance gate uses real Fleet v0.15.2, PostgreSQL/Vault, Git HTTPS writer
and reader, Terraform and Kubernetes. Product mutations use ordinary Console
forms only, including source ZIP file inputs, typed module/rendering selectors,
application/workload/dependency forms, CD discovery, Preview/Deploy and deletion.
No JSON/Score/YAML editor or direct product mutation API substitutes for UI.
Read-only technical checks verify commit/tree identity, object UIDs, data and
cleanup; platform prerequisites (Fleet, Git server and scoped RBAC) are setup.

Happy paths: managed GitRepo setup and verification; staging-only CD selection;
application with dependencies and public routes; valid image/config update and
reuse; self-heal policy selection; direct ↔ Fleet transfer without Job replay;
existing GitRepo with a dedicated generated subtree and retained sibling content;
application/workload removal and native absence. Scenarios are run where supported
by the approved forms; a missing form is an implementation gap, not API fallback.

Prerequisites: Node dependencies in `e2e/`, Playwright Chromium, Docker Compose,
kind/kubectl/Git, the task-owned Fleet platform from
[setup](../../setup/reconciliation-kind/README.md), run-tagged product images and
a fresh task-owned Compose project using `deploy/local/compose.reconciliation.yml`.
All credentials, kubeconfig, evidence and mutable repositories stay outside Git.
Console connection uses `e2e/lib/console.mjs` variables documented in
[the E2E guide](../../README.md). No shared cluster/cloud mutation is authorized.

The fixture Git server is `backend/test/integration/gitfixture`: anonymous reads
for Fleet; optional `-writer-auth-file` enables only authenticated pushes to its
one task-owned `repo.git`. Supply a 0600 file with username newline password;
never put credentials in URLs/arguments or evidence. It creates TLS files only
in its private certificate directory. Stop only its recorded task-owned process.

Cleanup goes through product deletion first and verifies owned UIDs/absence.
Retain preexisting Fleet/GitRepo/sibling resources until test platform teardown.
Delete only the exact Compose project/container/volumes and kind incarnation
recorded for this run. Failed/held runs retain private recovery evidence; do not
rerun against uncertain ownership or bypass guards to obtain PASS.


Run from repository root after the platform/account/store setup:

```bash
ORCH_PLATFORM_URL=http://127.0.0.1:<console-port> \
ORCH_PLATFORM_USERNAME=platform-engineer \
ORCH_PLATFORM_PASSWORD_FILE=<private-password-file> \
ORCH_HEADLESS=1 ORCH_RECON_RUN_DIR=<private-run-dir> \
ORCH_ACCOUNT_NAME='Reconciliation cluster' \
node e2e/scenarios/delivery-reconciliation/run.mjs
```

`ORCH_RECON_STATE_FILE` defaults to `ui-state.json` inside the private run directory;
`ORCH_RECON_APP_NAME` can set a distinct new application name. Persisted phase
flags resume acknowledged work without republishing it. An active/unknown operation
must be inspected/recovered before resumption. `ORCH_RECON_OBSERVATIONS=1` adds
stored-status UI checks and explicitly authorized controlled replica drift on the
owned backend Deployment (no HPA), then opt-in healing through Settings and proof
that Fleet restores replicas after a second controlled edit. This is the feature
assertion, not an intentional failure path. Evidence and failed images remain
private; progress GETs and native inspection never replace UI mutations.

After lifecycle assertions, run `cleanup.mjs` with the same connection/run/state
environment. It marks workloads for deletion through UI, deploys removal, removes
only recorded setup RBAC by UID, confirms Application deletion through its dialog,
and verifies native namespace/PV absence plus retained parent/sibling identity.
A failed product deletion retries through the existing UI dialog after remediation;
it never manually removes renderer/workload objects. Setup platform teardown is
separate and follows the cluster/Compose incarnation proofs.

Observed-run recovery helpers are `resume-policy.mjs` (self-heal policy) and
`recover.mjs` (existing-provider transfer; set `ORCH_RECON_RECOVERY_KIND=removal`
for the observed backend removal hold). They require the same private current
run state, exact cluster/namespace incarnation, and a recorded prior worker
container ID which must be stopped/removed and different from the current worker.
They open Settings, wait for stale ownership to become INTERRUPTED, require the
ordinary stopped-worker checkbox and click the existing recovery button. They
do not call mutation APIs directly or edit workloads/receipts. Original UIDs,
templates and parent/sibling identity are checked; removal permits only the
original backend withdrawal before the normal cleanup runner continues.
These helpers remediate actual failed runs, not deliberate failure scenarios.
They retain screenshots/logs outside Git. Do not reuse them for a different
project, scope or uncertain prior process, or mark cleanup passed from recovery
alone.

`ORCH_RECON_FOCUS=semantics` uses a separate new application on the owned platform
with no other active test applications. A platform author uploads a ZIP through
Mẫu dựng ứng dụng, updating the owned staging renderer to add a real PostgreSQL
migration Job, scheduled SQL-check CronJob and frontend HPA. Application workloads
and a `/` public path still use ordinary forms. Source fixtures extend the
standard bundle; no manifest/Score/JSON editor or product mutation API is used.
The migration has fixed approved database inputs, so an unrelated application
configuration update does not change its immutable Pod template. The HPA owns
replicas (renderer omits a fixed value), with minReplicas=2. Install the owned Metrics Server prerequisite and wait for
ScalingActive before testing provider handoff; the controller must be healthy. CronJob child completion
is checked independently from the application's queued business jobs.

Assertions include native Job UID/template/completion preservation across valid
update and provider handoffs, scheduled successful child jobs, HPA replica
authority after explicit self-heal, and removal of a rendered ConfigMap through a
second uploaded template/Preview/Deploy. The route resource is reviewed/applied
and included in identity assertions; public DNS/TLS/ingress provisioning is a
separate platform prerequisite, not claimed by port-forward business checks.
The renderer is task-owned; its catalog changes remain until platform teardown.
Run `cleanup.mjs` with the semantic state afterward, checking all owned workloads
and Terraform dependencies are removed while the existing parent/sibling remain.
