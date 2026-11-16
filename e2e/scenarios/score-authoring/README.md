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
