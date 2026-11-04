# Generic Workload → Template Definition → Fleet

Required behavior: [ADR-035](../../../orchestrator_docs/architecture/decisions/ADR-035-generic-workload-template-driver.md),
[contract](../../../orchestrator_docs/architecture/contracts/generic-workload.md).
Use the existing [CD runbook](../../../orchestrator_docs/operations/delivery-reconciliation.md)
and [Console helper](../../README.md). This scenario requires explicit authorization
for its selected Kubernetes/Git/Fleet environment and task-owned resources.

Prerequisites: current real-adapter Console (schema 8), scoped Platform Engineer
login, verified Fleet default, a READY task Kubernetes account, packaged namespace
Terraform module/provider, reachable image and private cluster kubeconfig. Install
E2E and bootstrap dependencies as described in the shared guide. Configuration:

- `ORCH_PLATFORM_URL`, `ORCH_PLATFORM_USERNAME`, `ORCH_PLATFORM_PASSWORD_FILE`, `ORCH_HEADLESS=1`.
- `ORCH_TEST_RUN_DIR`: private directory outside Git; receipts/screenshots stay here.
- `ORCH_TEST_NAME`: unique lowercase run name, also the Application name/subdomain.
- `ORCH_TEST_ACCOUNT`: READY account key on this isolated Console.
- `ORCH_CLUSTER_KUBECONFIG_FILE`: authorized cluster kubeconfig; never printed.
- `ORCH_TEST_ACCESS_NAME`: cleanup-only receipt name for the initial task rehearsal;
  defaults `bootstrap-fleet-access`. The only alternative is `generic-workload-fleet`,
  requiring its exact task-run ownership label and namespace UID.
- `ORCH_TEST_IMAGE`: reachable image serving HTTP on port 80 (default nginx:1.27-alpine).
- `ORCH_DELIVERY_SUBJECT_NAME` / `ORCH_DELIVERY_SUBJECT_NAMESPACE`: verified Fleet
  execution ServiceAccount; defaults orchestrator-delivery / cattle-fleet-system.

Run from repository root:

```sh
node e2e/scenarios/generic-workload/run.mjs
node e2e/scenarios/generic-workload/run.mjs verify
node e2e/scenarios/generic-workload/run.mjs cleanup
```

Setup creates app-scoped cluster/namespace Definitions through product APIs and
uses the existing namespace-access helper to add a Role/RoleBinding only after
the product creates its namespace. Setup is not acceptance of deployment.
Business steps use browser controls to create one workload Definition with Inline
Deployment/Service/ConfigMap templates and a namespace output reference, import a
Generic Workload, Preview, Deploy, then update replicas/env and deploy again.
Assertions check Definition/immutable pins, graph binding, real Deployment/Service/
ConfigMap/Pod readiness, Fleet GitRepo/Bundle reconciliation and namespace reuse.
No response interception, fake executor or direct manifest apply is used.

Receipts prevent duplicate Application creation and preserve the exact namespace
UID. A failed/lost deploy is recorded; the runner stops without automatically
recovering or replaying a mutation. Diagnose first and use product recovery only
when the recorded failed execution is proven stopped. Verification is read-only.
Cleanup removes the workload through Preview/Deploy, removes only recorded,
owner-labelled namespace access with UID-preconditioned API deletion, then uses
Application deletion. Check namespace absence and retain evidence. Connections,
Fleet controller and shared/default resources are never deleted by this runner;
its isolated Console hosting teardown is separate.
