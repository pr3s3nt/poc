# Score authoring acceptance — issue #83

Contract: [Score admission](../../../orchestrator_docs/architecture/contracts/score-workload.md).
Access and private inputs: [Tailscale handoff](../../../orchestrator_docs/operations/tailscale-handoff.md).

Prerequisites: a dedicated feature Console with a real PostgreSQL/Vault backend,
verified Git template source advertising backend/scheduled profiles, workload,
namespace, cluster and PostgreSQL Definitions, a verified kind account and CD
binding using platform Git. Setup is not workload acceptance. Credentials and
browser evidence stay outside Git.

Install dependencies with `npm ci --prefix scripts/bootstrap` and `npm ci --prefix e2e`.
Use the shared ORCH_PLATFORM_URL, ORCH_PLATFORM_USERNAME,
ORCH_PLATFORM_PASSWORD_FILE and ORCH_HEADLESS variables. ORCH_EVIDENCE_DIR must
be a private directory outside the repository. The scenario creates its own
Application through Console, imports Score/extension for all workloads, saves
all drafts, previews once and explicitly deploys the Environment once.

Assertions: dependency-preserving import, pinned templates, export/reimport,
Deployment and CronJob desired/applied configuration and real completed operation.
Runtime object and database checks supplement browser assertions. Never lower
assertions or replace the backend with mocks to claim acceptance.

Side effects: only the run-owned Application, generated Git delivery repository,
workload namespace and resources. Keep a private receipt with Application ID,
namespace UID and operation IDs. Cleanup through the Application deletion API/UI;
verify exact ownership/UID and absence before claiming completion. Do not delete
preexisting resources or shared platform controllers. Retain failed evidence.

Run authoring and deployment separately against the same private receipt:

```sh
ORCH_SCORE_PHASE=author node e2e/scenarios/score-authoring/run.mjs
ORCH_SCORE_PHASE=deploy node e2e/scenarios/score-authoring/run.mjs
```

Fixtures assume the setup's workload class `git-dev`; adapt that explicit class
in both extension files to the installation's selected Definition criteria.
The profile does not change matching criteria. The development Fleet subject also
needs app-scoped namespace permissions: use the existing
`scripts/bootstrap/lib/namespace-access.mjs` helper with the bounded acceptance
rules and explicit `dev-test` profile after product namespace creation. This is
setup-owned Role/RoleBinding, not provisioning implemented by this task. Keep
its namespace UID/application label receipt and remove it only after CD withdrawal.

A receipt-owned Application can resume authoring after a read-only parse failure;
already confirmed saved workloads are kept. The runner refuses another deployment
submission after `deployStarted`: inspect server operations before retry/recovery.

For a development target requiring app-scoped Fleet permissions, configure
[the namespace setup helper](../../setup/score-authoring-namespace/README.md)
before starting the deploy phase; the runner performs this setup as soon as the
product namespace exists, before waiting for the deploy response.


After a successful initial deployment, exercise a valid update and native checks:

```sh
ORCH_SCORE_PHASE=update node e2e/scenarios/score-authoring/run.mjs
ORCH_SCORE_PHASE=deploy-update node e2e/scenarios/score-authoring/run.mjs
node e2e/scenarios/score-authoring/verify-kind.mjs
```

Native verification requires explicit ORCH_SCORE_KUBECONFIG and
ORCH_SCORE_CONTEXT and writes only nonsecret identity/assertion evidence.
ORCH_SCORE_BEFORE_IDENTITIES optionally points to a private array of resource
kind/name/UID records to check preservation. No native workload mutation occurs.

`retry` is only for a recorded ROLLOUT_NOT_READY failure after verifying every
operation terminal; it keeps failed reports and requires a fresh Preview. An
unchanged Preview may correctly be empty after a delivered-but-not-ready failure;
inspect applied state and make an explicit valid update rather than resetting
receipts or replaying an unknown operation.

Before cleanup, suspend the owned CronJob through Save/Preview/Deploy. Then use
Application deletion, remove only receipt-owned setup RBAC after withdrawal and
resume product cleanup if namespace guards stopped at that setup. Verify
namespace, controller and PV absence; a successful DELETE response alone is not
proof that cleanup finished.

## Manual document entry

Run from the repository root with the shared Console variables and a private
`ORCH_EVIDENCE_DIR`:

```sh
node e2e/scenarios/score-authoring/manual-entry.mjs
```

This uses the real feature backend and the same `git-dev` fixtures/source as
above. It creates a separate Application, types Score and extension without
selecting files, checks through the real parse API, verifies that edits block
stale Save/export, retains invalid text, loads a file into editable text, then
exports/reimports and saves the edited draft. VI/EN labels and keyboard activation
of Check are covered. No Preview, Deploy or workload infrastructure is created.

The runner records its Application identity before saving and deletes only that
Application through deletion-preview/deletions in `finally`, after checking its
name. PASS requires both assertions and terminal SUCCEEDED cleanup. Each run
creates a new Application; keep the private receipt and failed screenshots when
diagnosing a failure. If cleanup fails, use the receipt to inspect and resume
product deletion rather than deleting another Application. Logs, downloaded YAML,
screenshots and receipts stay in the private evidence directory outside Git.

## Form/document tab synchronization

```sh
node e2e/scenarios/score-authoring/tab-sync.mjs
```

Use the same shared Console variables, dependencies and private
`ORCH_EVIDENCE_DIR` as manual entry. This scenario creates a separate Application
and uses the real `git-dev` fixture/template source. It checks YAML-to-form on
switch without explicit Check, unsaved form-to-Score image changes, YAML variable
and extension replica changes back to form, template/resource-reference
preservation, invalid text retention and disabled stale writes, no implicit Save,
explicit Save and reopening equivalent documents, plus VI/EN and keyboard use.

Only the owned Application and one workload draft are created; no Preview/Deploy
or workload infrastructure. The same receipt/name-checked product deletion in
`finally` must reach SUCCEEDED. Preserve failed evidence and inspect the receipt
before resuming interrupted cleanup. Existing user Applications are never saved
or deleted by this scenario. Screenshot/receipt/data files remain outside Git.
