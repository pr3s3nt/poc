# Disposable Fleet v0.15.2 platform

Prepares a new task-owned kind cluster with pinned, SHA-256-checked Fleet CRD and
controller charts from the official v0.15.2 release. This is platform setup,
not application-deployment acceptance. Scope: [ADR-030](../../../orchestrator_docs/architecture/decisions/ADR-030-delivery-reconciliation.md)
and [delivery contract](../../../orchestrator_docs/architecture/contracts/delivery-reconciliation.md).

Prerequisites: Docker, kind, kubectl, Helm, curl, sha256sum, rg, Git, local
`kindest/node:v1.35.0` image, network to GitHub/container registries, and explicit
permission for isolated local Kubernetes verification. No AWS resources are created.

From repository root:

```bash
ORCH_RECON_RUN_DIR=/tmp/orch-reconciliation-run-unique bash e2e/setup/reconciliation-kind/run.sh
```

The run directory must be new, absolute and outside Git. The cluster name defaults
to `orch-reconciliation-20261028`; `ORCH_RECON_CLUSTER` can name another unique
`orch-reconciliation-*` cluster. Existing clusters are refused. Success checks
Helm rollout, saves controller identities and private kubeconfig. A failure removes
only the newly created cluster. Reruns require a new run directory/cluster or
verified cleanup of the previous run; never adopt a preexisting installation.

The setup creates the cluster, controller/CRDs and Fleet's controller-owned local
resources. Application setup, Git authentication, scoped service accounts and
business assertions belong to the scenario. No plaintext credential or run
artifact is committed. Official chart hashes come from the immutable
[v0.15.2 release](https://github.com/rancher/fleet/releases/tag/v0.15.2).

After the authorized scenarios finish:

```bash
ORCH_RECON_RUN_DIR=/tmp/orch-reconciliation-run-unique bash e2e/setup/reconciliation-kind/cleanup.sh
```

Cleanup verifies container ID and kube-system namespace UID, deletes only the
owned cluster and confirms its container is absent. Private evidence and kubeconfig
remain outside Git for audit; remove them separately when no recovery needs them.

The HPA prerequisite uses Node, Python 3, curl and the existing Docker/kubectl tools.
For the HPA happy path, run `ORCH_RECON_RUN_DIR=<private-run> bash
 e2e/setup/reconciliation-kind/metrics.sh` before the application scenario. It
installs official Metrics Server v0.8.1 on the exact owned kind incarnation only,
refuses every preexisting manifest object, records created UIDs privately and
waits for the metrics API. The disposable kind kubelet uses a self-signed
certificate, so this fixture alone adds `--kubelet-insecure-tls`; no product or
shared-cluster TLS setting changes. See [official release](https://github.com/kubernetes-sigs/metrics-server/releases/tag/v0.8.1).
Cleanup is the existing entire owned-cluster teardown after product cleanup;
never run this prerequisite on a shared/preexisting cluster. Setup is not app
acceptance and cannot replace HPA/controller/business assertions.
