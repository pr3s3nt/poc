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
Internal account, schema prerequisites and a Git Catalog source. The former
Resource Definitions/Rendering Templates editors and their HTTP authoring routes
are retired. Definitions and templates are Git documents in an immutable release.

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
Source changes go through Catalog with CAS; Definition changes require a new tag.

Open each Application's Environment Settings, select the desired Catalog release,
Save, then Preview and Deploy. An unselected Environment is blocked with
CATALOG_REQUIRED. Do not infer adoption of existing infrastructure from matching
module names. The planner preserves identity and enforces compatibility and
retention guards; unsupported data/storage migrations remain blocked.

For live verification use the [Catalog-only UI scenario](../../e2e/scenarios/catalog-only/README.md)
on an explicitly authorized cluster. A screenshot of bootstrap alone does not
prove workload deployment. Keep evidence outside Git and preserve unrelated
Applications, resources, credentials and volumes.
