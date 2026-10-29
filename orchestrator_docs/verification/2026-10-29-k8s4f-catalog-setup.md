---
id: VERIFY-K8S4F-CATALOG-SETUP-20261029
artifact: verification-record
status: current
last_reviewed: 2026-10-29
---

# K8S-4F account, Types and Definitions through paced UI

User authorized registering the existing K8S-4F cluster and completing the
[configurable setup runner](../../e2e/setup/register-render-bundles/README.md):
reuse an existing matching account without re-uploading kubeconfig, register
Resource Types and Definitions, and choose pacing independently for those steps.
Product contracts: [UC-04](../usecase/UC-04/specification.md),
[UC-02](../usecase/UC-02/specification.md), [UC-03](../usecase/UC-03/specification.md).

## Preparation and retained target

The existing `kind-k8s-4f` context was present and its control-plane node was
Ready, Kubernetes v1.36.1. The default kubeconfig and current context were kept.
A single-context flattened kubeconfig was exported to an owner-only file under
the preview's private `/tmp` directory; only that copy's endpoint was changed to
`https://k8s-4f-control-plane:6443`. No credential bytes were logged or committed.

The retained preview backend `main-a0fe3a4-preview-backend-1` was attached to the
existing Docker network `kind`, as required by the [runbook](../operations/k8s4f-dev.md).
Its private Compose override also retains this network on future recreation.
Console remains `http://100.77.224.24:18596`; product image tags remain
`main-a0fe3a4`, since the change is setup scripts only.

## Actual UI registration

Headed Chromium used the shared Console login helper. Account and Definition
actions paused 800 ms before/after each operation, Types/bundle steps 700 ms;
text was entered sequentially. All writes used the visible forms, including
upload, typed mappings, source selection, class criteria and module inspection.
No Score or advanced JSON field was used.

Account `k8s-4f` registered as INTERNAL/KUBECONFIG and READY with the configured
context and endpoint. Six existing Types matched exactly and were reused:
`k8s-cluster`, `k8s-namespace`, `persistent-storage`, `postgres`, `redis`, `workload`.

Five new infrastructure Definitions were registered and their full persisted
configuration was compared with the configured account key, source pins,
variables, mappings and criteria:

| Definition | Driver/source | Matching |
|---|---|---|
| `k8s4f-cluster` | existing-cluster, account `k8s-4f` | class `internal` |
| `k8s4f-namespace` | installed `k8s-namespace` | class `default` |
| `k8s4f-storage` | Git `backend/terraform-modules/persistent-storage` | class `default` |
| `k8s4f-postgres` | installed `postgres-k8s` | class `default` or `in-cluster` |
| `k8s4f-redis` | installed `redis-k8s` | class `default` or `in-cluster` |

Git module/bundle source uses public project commit
`a0fe3a4a1e3c7ed22ad3dc18fa19744de040ce01`. Terraform Kubernetes providers bind to
the cluster resource of class `internal`. Namespace name comes from environment
context; storage/PostgreSQL/Redis namespace comes from the namespace resource
output. The existing `standard-workload` rendering Definition was reused.

Initial two attempts stopped at a select locator after account registration.
The exact label selector included option text; it was replaced with the
combobox accessibility selector. No Definition POST occurred in those failed
attempts. The corrected creation run completed all five registrations and the
five-template picker/Backend `/data` mount assertions.

## Rerun and checks

A subsequent full invocation omitted `ORCH_KUBECONFIG_FILE` and used faster
200 ms steps for the automated reuse check. It passed: account target and READY
matched without a local file; all six Type schemas and all five infrastructure
Definitions matched and were reused. Git inspect/cache and catalog verification
also passed. No duplicate account, Type or Definition was created. The picker
still showed frontend/backend/worker/stateful/scheduled 1.0.0; Backend volume
mount was enabled after choosing the storage dependency and `/data` was added
only on the local form.

`cd e2e && npm test`: 21/21 pass, zero skipped. Tests cover phase pacing and
precedence, expected account identity and no-file reuse, source/mapping pins,
account-key mismatch, schema flags and old configuration compatibility. Node
syntax/help, docs checker and diff whitespace checks pass. Product Go/frontend
code and dependencies did not change; existing build evidence was reused.

The account, metadata/cache and Definitions are deliberately retained for manual
testing; browser sessions signed out and closed. No Application draft was saved,
no Preview/Deploy or Terraform apply was run, and no namespace/PVC/workload/cloud
resource was created/deleted. Existing cluster workloads and other Compose
projects were preserved. Registration success does not claim workload deployment.
