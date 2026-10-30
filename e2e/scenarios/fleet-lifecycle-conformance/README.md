# Fleet lifecycle conformance

Technical acceptance of the pinned Fleet controller's retained-uninstall behavior:
GitRepo reads an exact commit, creates a ConfigMap carrying Helm `keep`, then
GitRepo withdrawal must leave that object's original UID. This is not product
browser/business E2E. It supports the [CDR-06 release gate](../../../orchestrator_docs/architecture/contracts/delivery-reconciliation.md#cdr-06--fleet-adapter-v1).

Prerequisites: authorized disposable cluster created by
[reconciliation-kind setup](../../setup/reconciliation-kind/README.md), Git with
`git-daemon`, Docker/kubectl/Helm, unused host bridge port 19418. The private run
directory and exact cluster/container/namespace incarnation must match. No shared
cluster, cloud resource or authenticated external Git service is used.

From repository root:

```bash
ORCH_RECON_RUN_DIR=/tmp/orch-reconciliation-fleet-run bash e2e/scenarios/fleet-lifecycle-conformance/run.sh
```

Side effects: private bare Git fixture and checkout, read-only Git daemon bound to
Docker bridge 172.18.0.1:19418, an owned GitRepo in fleet-local, one namespace and
ConfigMap. The Git daemon terminates on exit. Existing fixture directories are
refused. GitRepo observations, workload UID and remaining Helm releases are saved
privately outside Git. A failure retains the fixture for diagnosis; inspect
without rerunning the same fixture or weakening assertions.

Assertions: GitRepo applies from a pinned commit within three minutes; withdrawal
completes; original ConfigMap UID survives; Helm release metadata is recorded for
subsequent product cleanup design. This low-level fixture deliberately uses
read-only git://; it does not prove HTTPS/private-Git adapter behavior.

Cleanup: the entire run-owned cluster is removed by the setup's UID/container
checked cleanup script after scenario inspection. Never run this fixture in a
shared or preexisting cluster. Successful setup/this conformance test alone does
not prove workload deployment, revision health, Job idempotency, repair or product
ownership cleanup.
