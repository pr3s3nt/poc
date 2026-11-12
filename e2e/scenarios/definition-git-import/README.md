# Definition Git file import

Verifies [UC-03 GFILE](../../../orchestrator_docs/usecase/UC-03/specification.md#nguồn-git-trong-file-definition--issue-79)
and the [prepare/export API](../../../orchestrator_docs/usecase/UC-03/ui/api-mapping.md#git-document-preparationexport--issue-79).

Prerequisites: disposable feature Docker stack with issue #79, Node 22+,
`npm ci --prefix scripts/bootstrap`, `npm ci --prefix e2e`, Playwright Chromium,
public GitHub access from the backend, and PE/Admin credentials in a private file.
Use the [Tailscale runbook](../../../orchestrator_docs/operations/tailscale-handoff.md)
to verify the exact feature Console URL. This runner uses the shared login helper
and real API/Git inspection; no request mocking and no Terraform apply/deploy.

```sh
ORCH_PLATFORM_URL=http://<verified-feature-host>:8088 \
ORCH_PLATFORM_USERNAME=admin ORCH_PLATFORM_PASSWORD_FILE=/private/password \
ORCH_HEADLESS=1 ORCH_EVIDENCE_DIR=/private/evidence \
  node e2e/scenarios/definition-git-import/run.mjs
```

Optional `ORCH_IMPORT_PREFIX` sets a unique Definition key. The runner registers
the approved MongoDB Type through the UI if absent; an existing different Type
causes a failure without changing it. It imports a MongoDB Definition whose
source is only URL/ref/path; uses an existing real image digest as an offline
module input, without pulling or creating MongoDB. Assertions cover no Definition
write during check, full commit review, invalid mapping retention, edit
invalidation, CREATED, portable current/revision exports, YAML round-trip REUSED
and CONFLICT with unchanged revision history.

Side effects: appends run-scoped catalog metadata and verified source/cache in
the disposable feature database. No Applications, namespaces, credentials,
cluster/cloud resources or workload database are created. Keep catalog data for
review; cleanup removes only this feature Docker stack/volumes after acceptance.
Never point this runner at a shared installation because catalog deletion is not
part of the public API. Evidence/screenshots stay outside Git; the password is
read only by the shared helper and never printed.
