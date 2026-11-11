# One Fleet bundle per Environment, versioned secrets — issue #67

Goal: accept [ADR-042](../../../orchestrator_docs/architecture/decisions/ADR-042-environment-bundle-versioned-secrets.md)
through the Console as real users, and deploy the full acceptance application
(frontend, backend, worker, scheduled) with PostgreSQL, Redis and a PVC. Related:
[UC-12](../../../orchestrator_docs/usecase/UC-12/specification.md),
[UC-06](../../../orchestrator_docs/usecase/UC-06/README.md),
[ADR-040 rollback](../../../orchestrator_docs/architecture/decisions/ADR-040-environment-deployment-retry-and-rollback.md),
[secret store change runbook](../../../orchestrator_docs/operations/secret-store-change.md).

## Rules

Every business action is a click or an entry in the Console: the Admin prepares
the `developer` account; the developer creates the Application, selects the first
Environment secret store, Variables/Secret, authors every workload in the form
(template card, image, Service port, dependencies, variable sources, PVC, cron),
previews, deploys once per step, rolls back and deletes. No workload file
import, no direct API mutation, no fake adapter. Scoped API GETs, `kubectl get`
and Vault metadata reads are assertions only. Fault injection: deleting one
verified Pod with a UID precondition and forcing a Fleet resync. The Fleet agent
Role in the product-created namespace is the same technical setup as earlier
Fleet scenarios.

## Prerequisites

- A Console built from the branch, installed in its own namespace on
  `kind-k8s-4f` per [kind manifests](../../../deploy/kind-k8s-4f/README.md), seeded
  with `scripts/bootstrap/seed-api.mjs` (Git, cluster account, Vault store, Fleet
  CD) and `seed-definitions.mjs` (cluster, namespace, PostgreSQL `in-cluster`,
  Redis `in-cluster`, persistent storage, and a workload Definition whose template
  source is `backend/template-sources/standard` at a branch commit, so the
  `Backend API có migrate` card exists).
- Acceptance images `acceptance-{frontend,backend,worker,scheduled}:<tag>` and a
  second backend/worker tag loaded into the kind node.
- `npm ci --prefix scripts/bootstrap && npm ci --prefix e2e`, Chromium.

## Run

```sh
export KUBECONFIG=<private kubeconfig> ORCH_KUBE_CONTEXT=kind-k8s-4f
export ORCH_TEST_RUN_DIR=<private dir outside Git> ORCH_CONSOLE_NAMESPACE=<console namespace>
export ORCH_PLATFORM_URL=<console> ORCH_PLATFORM_USERNAME=admin ORCH_PLATFORM_PASSWORD_FILE=<file> ORCH_HEADLESS=1
# optional: ORCH_ACCEPTANCE_IMAGE_TAG, ORCH_ACCEPTANCE_IMAGE_TAG2, ORCH_SECRET_STORE_NAME, ORCH_APP_NAME
# ORCH_WORKLOAD_CLASS=<class> when the shared workload Definition matches a class (shared Console: git-dev);
# ORCH_ENVIRONMENT=production to run in another Environment (default staging)
for s in users prepare deploy image secret fail rollback resync; do node e2e/scenarios/environment-bundle/run.mjs $s || break; done
node e2e/scenarios/environment-bundle/run.mjs cleanup   # when the trial is over
```

## Assertions (issue #67 criteria)

| Stage | Checks |
|---|---|
| `prepare` | Developer selects the first store and then sees it read-only; keys saved; four workloads authored |
| `deploy` | One Deploy of four workloads succeeds; one GitRepo watching `app` and `migrate`; migration Job Complete before the backend Deployment exists; `<workload>-config-v1` VSS/Secret at Vault version 1; app checks PASS, job DONE, PVC value survives Pod recreation, scheduled receipt (1, 6, 9) |
| `image` | Backend image only: same VSS/Secret/ServiceAccount/VaultAuth UIDs and Vault version (2) |
| `secret` | Secret and its hash changed: Vault version 2, `-config-v2`, Pods on v2, v1 retained (3) |
| `fail` | Secret change + missing worker image: FAILED, Console names worker, backend succeeded, Environment released, old worker Pod restarts on v2, frontend untouched (4, 7) |
| `rollback` | Rollback restores `-config-v2` without a new Vault version; v3 VSS/Secret garbage collected; checks PASS (5, 8) |
| `resync` | Forced Fleet resync re-runs no migration Job (6) |
| `cleanup` | Application deletion removes namespace, Fleet GitRepo and workload Vault paths (10) |

## Side effects and cleanup

Creates one Application, its namespace, PostgreSQL/Redis/PVC and a delivery Git
repository through the product. `cleanup` deletes the Application through the
Console; the delivery repository history is retained per ADR-033. Evidence
(`*-result.json`, reports, screenshots) and the developer password stay in
`ORCH_TEST_RUN_DIR`, outside Git.
