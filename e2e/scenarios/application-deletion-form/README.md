# Workload forms → application deletion on kind

Verify the integrated [UC-16 workload form](../../../orchestrator_docs/usecase/UC-16/README.md)
and [UC-18 Application deletion](../../../orchestrator_docs/usecase/UC-18/README.md).
The existing owned-kind harness prepares a disposable cluster, Vault/VSO and
Console stack. This scenario registers the catalog through the PE Console,
then reuses [workload-env-form](../workload-env-form/README.md) as Developer.
Application and workload mutations use form controls; no Score files, Score
editor or direct mutation API are used.

## Prerequisites and run

Explicit permission for kind mutations; Docker, kind, Helm, kubectl, Go, Node 24,
Xvfb and Chromium. Install locked dependencies in `frontend/` and `e2e/` with
`npm ci`, and Chromium with `cd e2e && npx playwright install chromium`.
Run from the repository root:

```sh
bash backend/test/integration/workload-manifests-kind.sh \
  --evidence /absolute/new/private/evidence --run-id uniqueid --form-deletion
```

The wrapper supplies shared `ORCH_PLATFORM_*` configuration and a private file
for the isolated platform's fixed test password. Its `WM_*` values identify
the exact owned cluster, images, kubeconfigs, workload Vault and read-only SQL
helper. Do not invoke `run.mjs` against a retained platform. No fake adapters,
intercepted responses or external AWS resources are used.

## Assertions

- UI registration of Internal account, types, verified module Definitions,
  rendering template and Secret Store.
- Form creation of three workloads with direct values, inline Variable/Secret,
  PostgreSQL/Redis outputs and a service reference.
- Preview → deploy, five application checks PASS, submitted job DONE.
- After both initial and post-update jobs: all five UI checks PASS, recorded
  Redis worker writes nonzero, no failed/unverified write or unresolved cache
  lookup failure. A pre-job health check alone is insufficient.
- Valid form update; shared PostgreSQL/Redis and PVC UIDs preserved.
- Developer reviews both Environments and confirms the exact Application name;
  editing controls lock while deletion runs.
- Deletion SUCCEEDED; Application disappears with a notice and reads as 404.
- All three owned instances DESTROYED, native namespace lookup empty, tombstone
  retained, exact Vault Secret/delivery bundle paths return HTTP 404, cluster
  remains available.
- The same name/subdomain creates a new Application with a fresh ID; its empty
  Environments are also deleted through Settings so no replacement is retained.
- No browser JavaScript errors or secret material in public evidence.

## Side effects, cleanup and evidence

Each run owns its new kind cluster, Compose project/volumes, workload Vault and
image tags. The wrapper records identities before creation, runs cleanup on
failure as well as success, and verifies absence of those exact resources.
Other clusters, Applications, Vaults and retained installations are preserved.
On scenario failure, keep failed assertions and private inputs for diagnosis;
do not report teardown as product deletion success.

Evidence lives outside Git: setup assertions, form results, screenshots, browser
videos when enabled, deletion progress and wrapper cleanup/secret-scan receipts.
The scenario exits nonzero on an assertion failure. It establishes kind behavior;
AWS requires separate acceptance under the [AWS runbook](../../../orchestrator_docs/operations/aws.md).
