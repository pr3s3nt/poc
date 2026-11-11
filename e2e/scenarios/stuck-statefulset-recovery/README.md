# Stuck StatefulSet recovery and Active Definition revision — issue #68

Goal: check [UC-08 BR-11](../../../orchestrator_docs/usecase/UC-08/specification.md),
[UC-06 CD-11](../../../orchestrator_docs/usecase/UC-06/specification.md) and
[UC-03 DV-01/DV-04](../../../orchestrator_docs/usecase/UC-03/definition-versions.md)
on a real kind cluster (Terraform, Fleet, Vault, Kubernetes) through the Console.
This run is a regression check; the issue's acceptance is the user's own Console run.

## Rules

Developer actions are clicks in the Console: create the Application, select the
secret store, author workloads in the form, Preview, Deploy, choose a revision in
Settings and delete the Application. The Platform Engineer's Definition edit and
promotion use the Admin session's Console API as setup (same calls as the
Definition editor). `kubectl get` reads are assertions only. The Fleet agent Role
in the product namespace is the technical setup shared with earlier Fleet scenarios.

## Prerequisites

- A Console built from the branch in its own namespace on `kind-k8s-4f`
  ([kind manifests](../../../deploy/kind-k8s-4f/README.md)), seeded with
  `scripts/bootstrap/seed-api.mjs` (Git, cluster account, Vault store, Fleet CD) and
  `seed-definitions.mjs` (cluster, namespace, PostgreSQL `in-cluster`, Redis,
  storage, workload template from this branch).
- `nginxinc/nginx-unprivileged:1.29-alpine` loaded in the kind node.
- `npm ci --prefix scripts/bootstrap && npm ci --prefix e2e`, Chromium.

## Run

```sh
export KUBECONFIG=<private kubeconfig> ORCH_KUBE_CONTEXT=kind-k8s-4f
export ORCH_TEST_RUN_DIR=<private dir outside Git> ORCH_PLATFORM_URL=<console>
export ORCH_PLATFORM_USERNAME=admin ORCH_PLATFORM_PASSWORD_FILE=<file> ORCH_HEADLESS=1
# optional: ORCH_SECRET_STORE_NAME, ORCH_POSTGRES_DEFINITION, ORCH_WEB_IMAGE
for s in users pg-bad pg-fix pinned guard sts-bad sts-same sts-fix; do
  node e2e/scenarios/stuck-statefulset-recovery/run.mjs $s || break
done
node e2e/scenarios/stuck-statefulset-recovery/run.mjs cleanup
```

## Assertions

| Stage | Issue scenario | Checks |
|---|---|---|
| `pg-bad` | 1 | Wrong PostgreSQL image promoted; Deploy FAILED; operation and Console show `ImagePullBackOff`/`ErrImagePull` with the image; no Pod replaced |
| `pg-fix` | 1 | Fixed image promoted; never-deployed Environment resolves it without selection; Deploy SUCCEEDED; Console shows the replaced Pod; PVC identities unchanged; Pod Ready on the fixed image |
| `pinned` | 4 | New revision after a successful deploy: Environment keeps its revision; Settings shows "Có bản mới"; Preview names the newer default |
| `guard` | 5 | Selecting the changed PostgreSQL revision: Preview refused (migration required); selection discarded |
| `sts-bad` | 2 | StatefulSet workload with a wrong image through Fleet: Deploy FAILED with the reason and image |
| `sts-same` | 3 | Unchanged redeploy (when offered): reason shown, no Pod replaced |
| `sts-fix` | 2 | Image fixed in the form: Deploy SUCCEEDED, Pod replaced and running the fixed image |
| `cleanup` | — | Both Applications deleted through the Console; namespaces gone |

## Side effects and cleanup

Creates two Applications with their namespaces, a PostgreSQL StatefulSet/PVC and
delivery Git repositories; adds revisions to the PostgreSQL Definition (kept, as
Definition history is immutable). `cleanup` deletes the Applications through the
Console; delivery repository history is retained per ADR-033. Evidence
(`*-result.json`, `*-report.json`, screenshots) and the developer password stay in
`ORCH_TEST_RUN_DIR`, outside Git.
