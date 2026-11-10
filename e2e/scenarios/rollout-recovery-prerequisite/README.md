# Recovery of a first Rollout interrupted after its Prerequisite — issue #48

Goal: prove the first-Rollout absence recovery in
[UC-06](../../../orchestrator_docs/usecase/UC-06/specification.md) and the
[delivery reconciliation contract](../../../orchestrator_docs/architecture/contracts/delivery-reconciliation.md):
when the owning worker stops after the Prerequisite (Service) is acknowledged and
before the Rollout is published, Recover settles only the Rollout as FAILED,
keeps the Prerequisite, its Service and the Fleet controller, releases the
Environment and replays nothing. Redeploy is a separate user action.

## Topology

One Application `prereq-issue48` with one workload `payment` (template "Xử lý nền",
class `git-dev`) whose Service port `http` 8080 makes the Service a separate
Prerequisite unit. The container is the digest-pinned `postgres:16.14-alpine`
already present on the kind node, running `sleep`.

## Prerequisites

- An isolated Console built from the branch under test in its own namespace on
  `kind-k8s-4f` per [kind Console manifests](../../../deploy/kind-k8s-4f/README.md),
  seeded with `scripts/bootstrap/seed-api.mjs`.
- `npm ci --prefix scripts/bootstrap && npm ci --prefix e2e`, Chromium for Playwright.
- `ORCH_TEST_RUN_DIR`: private directory outside Git with `config.json`
  (`clusterKubeconfigFile`, `secretStoreKey`, `consoleNamespace`). Console variables
  `ORCH_PLATFORM_URL`, `ORCH_PLATFORM_USERNAME`, `ORCH_PLATFORM_PASSWORD_FILE`,
  optionally `ORCH_HEADLESS=1`.

## Run

```sh
node e2e/scenarios/rollout-recovery-prerequisite/run.mjs prepare
node e2e/scenarios/rollout-recovery-prerequisite/run.mjs interrupt
node e2e/scenarios/rollout-recovery-prerequisite/run.mjs recover
node e2e/scenarios/rollout-recovery-prerequisite/run.mjs deploy
node e2e/scenarios/rollout-recovery-prerequisite/run.mjs cleanup
```

`interrupt` resumes from its recorded stop; it never deploys twice.

## Fault injection

`interrupt` clicks Deploy once and watches the Console database read-only
(`psql \watch 0.2` through `kubectl exec`). At the first observation of the
Prerequisite intent ACKNOWLEDGED and the Rollout intent RUNNING without a
receipt it force-deletes the Console backend Pod: the worker stops between the
two units, as in the Boutique payment Deploy held with `RECOVERY_INCOMPLETE`. If
the Rollout progressed first, the run fails and is repeated with a new run
directory and Application name.

## Assertions

- `interrupt`: one Deploy request; Operation INTERRUPTED and the Environment held;
  Prerequisite acknowledged and published; Rollout not published.
- `recover` (Settings banner with stopped-worker confirmation): Operation
  RECOVERED, Environment released; the Rollout intent FAILED without publication;
  the Prerequisite intent unchanged (status, version, receipt, controller UID);
  the Service UID unchanged; no Rollout Deployment applied.
- `deploy`: Operation SUCCEEDED with the same Fleet controller UID and Service UID.
- `cleanup`: deletion SUCCEEDED and the Application namespace is absent.

## Side effects and cleanup

Creates the Application, its namespace and Fleet delivery tree; grants the Fleet
access Role/RoleBinding for the namespace (technical setup); restarts the
isolated Console backend once. `cleanup` removes that access, deletes the
Application through the Console and checks the namespace is gone. Run state,
screenshots and evidence stay in `ORCH_TEST_RUN_DIR`. The isolated Console
namespace is removed separately by the operator.
