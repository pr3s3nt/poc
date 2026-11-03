---
id: RUNBOOK-STAGING-CATALOG
artifact: operations-runbook
status: current
last_reviewed: 2026-11-02
---

# Bootstrap Kubernetes staging catalog

Backend staging creates missing Resource Types and configured login identities;
it does not create Driver Accounts, Applications, Definitions or templates.
Existing schema and platform data survive restarts. No database reset is needed.

Use the [bootstrap tool](../../scripts/bootstrap/README.md) to register/reuse an
Internal account, schema prerequisites and a Git Catalog source. Resource Definitions/Rendering Templates also have Console authoring forms.
The bootstrap tool uses Git documents in immutable releases for these artifacts.

1. Copy [platform-catalog](../../platform-catalog/README.md) into the root of the
   company Git repository. Review `catalog.yaml` and `definitions/`, including
   namespace/cluster references, matching criteria and storage class. Set the
   appropriate storage class in Git (for example `rook-ceph-block` on Ceph).
2. Push to the tracked branch and publish a SemVer tag, for example `v1.0.0`.
3. Configure Console auth and private Git access according to the bootstrap
   guide. Keep passwords, PATs, kubeconfig and CA files outside Git.
4. Run from repository root:

```bash
node scripts/bootstrap/run.mjs --target staging \
  --url https://console-staging.example \
  --kubeconfig /private/staging.kubeconfig \
  --git-url https://git.company.example/platform/catalog.git
```

The preset's `catalog` block chooses source ID, branch, tag pattern, manifest path
and symbolic account slots. Defaults are `main`, `v*`, `catalog.yaml`; a placeholder
repository must be replaced before opening the browser. `accountBindings` map
Git slots to existing platform account names/IDs. READY accounts are reused;
a new account requires an uploaded real kubeconfig and verification.

Bootstrap checks existing Type schemas and source configuration before reuse.
It does not overwrite different configuration, select Environment versions or
deploy. The obsolete `--git-ref`/`--update-existing` editor workflow is removed.
Source changes go through Catalog with CAS. Definition changes may be made in
the restored Console form; Catalog modules are selected by explicit release.

Open each Application's Environment Settings, select the desired Catalog release,
Save, then Preview and Deploy. Alternatively, directly authored Definitions use
their own module pins without an Environment Catalog selection. Do not infer adoption of existing infrastructure from matching
module names. The planner preserves identity and enforces compatibility and
retention guards; unsupported data/storage migrations remain blocked.

The [Definition/Catalog browser scenario](../../e2e/scenarios/definition-catalog/README.md)
checks form/module access without saving or deploying. The historical
[Catalog-only deployment run](../verification/2026-11-02-catalog-only-live.md)
records an earlier authorized cluster run; its retired-navigation assertions
do not apply to the restored forms. A screenshot of bootstrap alone does not
prove workload deployment. Keep evidence outside Git and preserve unrelated
Applications, resources, credentials and volumes.

## Full environment bootstrap

For issue #30 use the file-only schemaVersion 2 workflow in the
[bootstrap guide](../../scripts/bootstrap/README.md), with separate dev/staging/
production inputs. It adds Secret Store/CD setup, acceptance deployment,
verification and explicit version upgrades to the reusable Catalog setup.
The historical CLI above remains Catalog-only through legacy presets.

[Bootstrap acceptance scenario](../../e2e/scenarios/bootstrap-acceptance/README.md)
checks unchanged reruns against real server state. A setup PASS alone is not a
workload/business PASS. Namespaced Fleet access can be configured by rulesFile;
shared cluster prerequisites still require separate authorization.
