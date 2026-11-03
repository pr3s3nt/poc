# Simplified CD configuration — issue #31

Scope: [UC-04](../../../orchestrator_docs/usecase/UC-04/specification.md),
[ADR-034](../../../orchestrator_docs/architecture/decisions/ADR-034-simplified-cd-configuration.md),
[bootstrap](../../../scripts/bootstrap/README.md) and
[database upgrade procedure](../../../orchestrator_docs/operations/postgres-upgrades.md).

Requires explicit authorization to mutate the selected dev Console and its
GitHub/Fleet/Kubernetes resources. Schema 7 and the current frontend/backend must
already be running with real adapters, existing Fleet, release namespace,
delivery ServiceAccount/RBAC, Vault and acceptance Catalog/images. Install both
packages with `npm ci --prefix scripts/bootstrap` and `npm ci --prefix e2e`;
install Chromium with `npx --prefix e2e playwright install chromium`.

Run from repository root, using a bootstrap configuration whose secrets are
private files outside Git:

```sh
ORCH_SIMPLIFIED_CD_RUN_DIR=/private/issue31-run \
ORCH_SIMPLIFIED_CD_NAME=issue31-unique-run \
node e2e/scenarios/simplified-cd/setup.mjs scripts/bootstrap/targets/dev.json
```

The script reads the configured Console URL/account and existing kubeconfig/Git
credential file paths; it never prints their contents. All product writes use
ordinary browser forms. API reads verify scope, pinned selections and statuses.
The run directory is private and must be outside Git. It stores safe ownership
receipts, screenshots and the original default/Environment selections for recovery.
Reruns observe exact named records first; ambiguous identities stop without writes.

Setup registers Kubernetes and Git Connections inline, discovers Fleet topology,
saves/verifies a managed-reader binding and explicitly selects it as the default.
It asserts that existing Applications retain their desired/applied bindings.
Setup alone is not successful workload deployment evidence. Real application
acceptance uses the existing bootstrap runner with a separate run-owned application
and its own private state/report directories; include baseline, update, rollback,
exact Git commit/Fleet state/YAML reads and product cleanup in the execution record.

After setup, use a private acceptance config selecting the recorded account and
binding name, absolute fixture paths, unique application identity, private state
and report directories, and already available baseline/update images. Keep the
same config and receipts throughout all phases:

```sh
export ORCH_SIMPLIFIED_CD_RUN_DIR=/private/issue31-run
node scripts/bootstrap/run.mjs --config /private/acceptance.json
node e2e/scenarios/simplified-cd/verify.mjs /private/acceptance.json
node e2e/scenarios/simplified-cd/rollback.mjs /private/acceptance.json
node e2e/scenarios/simplified-cd/verify.mjs /private/acceptance.json
node e2e/scenarios/simplified-cd/cleanup.mjs /private/acceptance.json
```

The bootstrap run checks baseline, update and unchanged Preview reuse against
real business data/jobs/PostgreSQL/Redis. Verification reads the actual private
GitHub repository, SHA-pinned GitRepo **and Bundle**, YAML files, managed reader
ownership and applied binding. Its database read is deliberately restricted to
the dev Compose container `deploy-main-postgres-1`. Rollback creates binding
version 2 using the existing Git Connection, changes the default, proves the
existing Application stays on version 1, then deploys the previous workload
images and checks retained data/job identity and unchanged Preview reuse.
These scripts only accept `dev` and the `staging` application environment.

On failure, inspect the response and owned runtime before resuming. Bootstrap
uses its recorded response-confirmed failed operation recovery; unknown writes
must not be replayed. Cleanup requires the recorded successful rollback receipt
and preserves evidence if a product guard holds removal.

Side effects are new reusable Connections/binding versions, a temporary platform
default change and authentication sessions. First admitted application deployment
also creates its private GitOps repository, generated binding reader Secret and
owned application resources. Preserve existing accounts, bindings, controllers,
reader Secrets and workloads. Never delete a resource based only on its name.

Cleanup must restore the recorded platform default through the Console, delete
the test Application through the product and confirm owned workload/namespace
absence by UID. Keep repository history and reusable binding/Connection metadata
per the product lifecycle; record these retained artifacts explicitly. The managed
reader Secret belongs to the reusable binding and stays for subsequent deployments.
Do not manually remove workload/PVC resources to bypass a held product cleanup.
Failed runs retain evidence and ownership receipts for diagnosis before resuming.

Only a dated verification record reports which real steps passed. Fixture/unit
tests, HTTP shell responses and this setup script do not prove GitHub → Fleet →
Kubernetes delivery. Follow the [Tailscale handoff](../../../orchestrator_docs/operations/tailscale-handoff.md)
for any URL offered to a user; check the actual page and its APIs in a browser.
