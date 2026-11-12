# Type/Definition file registration

Verifies [UC-02 FILE](../../../orchestrator_docs/usecase/UC-02/specification.md#nhậpxuất-document--issue-76)
and [UC-03 FILE](../../../orchestrator_docs/usecase/UC-03/specification.md#nhậpxuất-document--issue-76)
against a real Console/backend, using the shared login helper. No mock routes.

Prerequisites: Node 22+, `npm ci --prefix scripts/bootstrap`, Playwright Chromium,
a disposable feature Docker stack, PE/Admin login in a private password file.
Use the [Tailscale runbook](../../../orchestrator_docs/operations/tailscale-handoff.md).

```bash
ORCH_PLATFORM_URL=http://<verified-feature-host>:8088 \
ORCH_PLATFORM_USERNAME=admin ORCH_PLATFORM_PASSWORD_FILE=/private/password \
ORCH_HEADLESS=1 node e2e/scenarios/catalog-document/run.mjs
```

Optional `ORCH_EVIDENCE_DIR` is an existing directory outside Git for screenshots.
Assertions: upload JSON, edit before registration, CREATED, equivalent REUSED,
CONFLICT without clearing draft, syntax error retention, download YAML and re-import
REUSED for Type and endpoint Definition. Endpoint Type is prepared through the API;
all tested registration actions go through the UI. No provisioning is performed.
Terraform/template/Preview/Deploy acceptance requires separate prepared sources,
accounts and delivery setup; this scenario does not prove those gates.

Side effects: creates run-suffixed Type and Definition plus endpoint Type if absent.
Keeps data in the disposable feature database for review; no deletion API exists
for these catalog entities. Cleanup by removing only that feature stack/volumes
after acceptance, as documented in the runbook. Never point at a shared installation.

## Real Terraform/template Preview and Deploy

`runtime.mjs` uses prepared cluster/namespace/PostgreSQL/workload Definitions as
source-pin fixtures, imports new Definitions through the Console with `app_id`
criteria scoped to its newly created Application, authors one workload and two
PostgreSQL dependencies through the existing form, then deploys once for the
Environment. This is a mutating kind scenario: run only on the authorized
`kind-k8s-4f` context with task-owned state and a disposable feature stack.

Prerequisites beyond the round-trip scenario: Git sources already inspected and
available (for example prepared with `scripts/bootstrap/seed-definitions.mjs`),
READY Internal account and default Fleet delivery binding named `dev-fleet`, kind
network access from the feature backend, `compose.bootstrap.yml` delivery storage,
and `KUBECONFIG` pointing to a private host kubeconfig for technical reads and
namespace-scoped Fleet access setup. It does not install controllers or change
shared cluster RBAC. Prepared keys default to `issue76-prepared-*`; override
`ORCH_PREPARED_PREFIX` if needed. They must contain exactly the four resource types
above, and provide the public `image` default in the PostgreSQL module contract.
The existing PostgreSQL Type must require database/username. No Type is modified.

Set the Console variables above, `KUBECONFIG`, and `ORCH_TEST_RUN_DIR` to a new
private directory outside Git. Optional `ORCH_APP_NAME` and `ORCH_IMPORT_PREFIX`
control new identities. Run stages sequentially and inspect a failure before
retrying; never clear a recorded Deploy attempt to replay it blindly:

```bash
node e2e/scenarios/catalog-document/runtime.mjs prepare
node e2e/scenarios/catalog-document/runtime.mjs author
node e2e/scenarios/catalog-document/runtime.mjs deploy
node e2e/scenarios/catalog-document/runtime.mjs verify
node e2e/scenarios/catalog-document/runtime.mjs cleanup
```

Assertions cover the four imported winners in Preview, unchanged source pins and
mappings/bindings, READY namespace/two PostgreSQL resources, Ready workload, two
required username params overriding Definition fallback, and deployed PostgreSQL
image matching the default of the exact inspected pin. Verification imports that
default explicitly and requires REUSED with byte-equivalent revision history.
Required Type inputs still cannot be omitted even when Definition fallback exists.
It reads only the username key from a Kubernetes Secret to check the override;
passwords are never read or printed. Platform cluster resources are deliberately
absent from the Environment resource-list API.

State, Preview receipt and failure screenshots stay in the private run directory.
Cleanup checks namespace UID/ownership, removes only its labelled technical
Role/RoleBinding with UID preconditions, deletes the Application through the
product UI, polls its deletion operation and verifies namespace absence. Catalog
Types/Definitions remain in the disposable feature database for review. A completed
run requires a new state directory; preserve failed-run evidence until resolved.
