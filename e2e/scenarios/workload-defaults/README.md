# Workload defaults v1 acceptance

[WD specification](../../../orchestrator_docs/usecase/UC-16/workload-defaults-specification.md)
and [contract](../../../orchestrator_docs/architecture/contracts/workload-defaults.md).

Requires explicitly authorized task-owned kind/Fleet and Git publication, current
real-adapter Console, a verified default CD binding, READY Kubernetes account,
packaged namespace module, E2E/bootstrap dependencies from [shared guide](../../README.md).
Set `ORCH_PLATFORM_URL`, `ORCH_PLATFORM_USERNAME`, `ORCH_PLATFORM_PASSWORD_FILE`,
`ORCH_HEADLESS=1`, `ORCH_TEST_RUN_DIR` (private, outside Git), `ORCH_TEST_NAME`
(unique), `ORCH_TEST_ACCOUNT`, `ORCH_CLUSTER_KUBECONFIG_FILE`. The run directory
must be separate from the CD setup receipt directory.

From repository root:

```sh
node e2e/scenarios/workload-defaults/run.mjs
node e2e/scenarios/workload-defaults/run.mjs cleanup
```

Setup registers task app-scoped cluster/namespace/template Definitions using
product APIs and current standard template source. Business flow uses browser
import/form Save, Preview and Deploy. Checks legacy reopen/no-op, explicit adoption,
false/zero, effective/provenance Preview, two writable emptyDirs on a read-only
non-root Nginx, PDB enable/prune and namespace/controller identity. Preview checks
resource versions before/after to prove it caused no mutation. Deploy checks real
Fleet commits/readiness and native controller state. No response interception,
fake provider or direct workload apply is used. Logs/screenshots/receipts stay in
`ORCH_TEST_RUN_DIR`. Failures retain evidence and ownership receipts for diagnosis;
do not blindly replay an unknown mutation.

`cleanup` removes the recorded workload via product Preview/Deploy, UID-checks
and deletes only the recorded namespace access Role/RoleBinding, then uses product
Application deletion and verifies namespace absence. Git/Fleet resources follow
the product's original target/inventory/UID cleanup. The isolated Console and
reusable binding remain available for handoff. Task kind setup/teardown uses
[the existing owned-cluster runner](../../setup/reconciliation-kind/README.md).

Additional real renderer/adapter storage acceptance runs with:

```sh
cd backend
ORCH_ISSUE37_KUBECONFIG=/private/task-kind/kubeconfig go test ./internal/adapters/templatedriver -run '^TestRuntimeDefaultsOnOwnedKind$' -count=1 -v
```

This creates a unique labelled namespace in an `issue37` kind only. It verifies
PDB pruning, sequential reuse of one independently owned PVC by two consumers
without cloning/deleting the source, and claim UID/data retention after operational scale down and product
adapter workload removal; product authoring still permits StatefulSet replica 1.
On success it explicitly deletes the owned claim by UID, then the namespace by UID.
On failure it retains the namespace for diagnosis. This adapter acceptance is
separate from the browser/CD flow and does not stand in for it.
