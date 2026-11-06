---
id: IMP-WORKLOAD-DEFAULTS
artifact: implementation-handoff
status: current
last_reviewed: 2026-11-06
---

# Complete workload runtime configuration — issue #37

Scope: [WD-01–10](../usecase/UC-16/workload-defaults-specification.md),
[data contract](../architecture/contracts/workload-defaults.md). Baseline main
`493eecea`; task branch `features/bo_sung_default`. Runtime acceptance is recorded
separately; this matrix is not proof of deployment.

| Field/behavior | Contract/domain | Authoring | Render/verification |
|---|---|---|---|
| Pod/container security | `environment/runtime_policy.go`, pointer scalars | `RuntimePolicyFields`, `SecurityFields` | `runtime-policy.tpl`, semantic guard, all controller tests |
| SA/token | `WorkloadExecution`, delivery-owned SA | Managed/read-only identity, inherit/false/true token | Vault exception/explicit false rejection; authorized identity guard |
| Strategy/history | Controller-specific typed policy | Scaling & updates, history reset | Exact strategy guard; no incompatible fields, zero preserved |
| PDB | Explicit enable, int/percent, >=2 stateless replicas | min/max selection, disabled distinct from inherit | `pdb.tpl`, selectors/ownership, inventory prune, server dry-run |
| Stateful claims | Named claim templates/mounts, Retain/Retain | StatefulSet-only storage group | Controller claim ownership, exact mounts/size/class guard, live UID/data retention |
| Versioned defaults | Pure resolver; frozen verified template profile in policy metadata | Backend effective values/sources and reset | Desired stays overrides; snapshots retain policy/pin, Preview effective artifact |
| Compatibility | Omitted policy keeps legacy, strict new-field null checks | Existing import/reopen retains absence/false/zero | Legacy tests, reconstruction, JSONB reopen, no silent adoption |
| Existing runtime features | Existing lifecycle/volume/env/probe model retained | Per-container groups and reuse-first source dialog | Existing focused tests and explicit capabilities |

The standard source advertises four new capabilities. `nginx-readonly` publishes
an image-specific profile, two writable emptyDirs, valid readiness and declared
production replica compatibility. New source publication/Definition selection is
explicit; historical pins are not overwritten. No SQL schema migration is needed:
JSONB workload/module snapshots carry the extension; schema 10 and released SQL
references/migrations remain unchanged.

Read-only resolution is exposed through scoped POST `workloads/resolve-defaults`.
Only backend admission fills verified `defaults.template`; displayed values never
become container/workload overrides. Frozen metadata lets Service references and
validation inspect inherited configuration without rewriting desired documents.
Policy mismatch is rejected by Save and by selected-source planning validation.

Claim inventory is the StatefulSet template plus the controller-created PVC's
namespace/name/UID and inherited workload labels. It is separate from standalone
Terraform resource inventory. Workload removal never includes these claims in
its delete list; namespace cleanup's existing live UID/content guard refuses
untracked retained PVCs. Before mutation, server dry-run checks cluster capability;
a newly created StatefulSet cannot adopt existing same-name retained claims.
V1 has no automatic retained-claim adoption or data migration UI.

Checks and runtime procedures:
[acceptance runner](../../e2e/scenarios/workload-defaults/README.md).

[Verification and runtime limits](../verification/2026-11-06-workload-defaults.md).
