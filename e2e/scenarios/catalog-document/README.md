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
