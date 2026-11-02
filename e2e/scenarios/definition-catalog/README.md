# Definition form with a Catalog module

Verifies [UC-03 CM-01–04](../../../orchestrator_docs/usecase/UC-03/catalog-releases.md#definition-authoring-and-catalog-modules)
on a real Console/backend without API interception. Uses the restored Definition
form, explicit Catalog release and verified Terraform module, criteria tab,
Git/ZIP/registered source switches and Rendering Templates route. Asserts unchanged
Applications/Definitions and no browser runtime errors. Save/CAS/provenance are
covered separately by component and backend service tests.

Prerequisites: current backend/frontend, PE account, one UI-registered Catalog
with a published Terraform component. Install `npm ci --prefix e2e` and
`npm ci --prefix scripts/bootstrap`, then `cd e2e && npx playwright install chromium`.
Follow the [shared Console configuration](../../README.md#dependencies-và-kết-nối-console-dùng-chung).
From repository root:

```bash
ORCH_PLATFORM_URL=http://<verified-tailscale-host>:3001 \
ORCH_PLATFORM_USERNAME=platform-engineer \
ORCH_PLATFORM_PASSWORD_FILE=/private/path/password \
ORCH_HEADLESS=1 ORCH_EVIDENCE_DIR=/private/path/evidence \
node e2e/scenarios/definition-catalog/run.mjs
```

Only authentication/session writes occur. Draft selection is local and no Definition
is saved, source refreshed, Git written, workload deployed or provider invoked.
There are no created domain resources to clean up; the browser context closes.
Screenshots/summary stay outside Git in ORCH_EVIDENCE_DIR. This checks UI access
from the runner host; it does not prove other-device ACLs or Terraform execution.
