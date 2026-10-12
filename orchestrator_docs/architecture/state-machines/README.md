---
id: STATE-MACHINE-INDEX
artifact: state-machine-index
status: current
last_reviewed: 2026-10-13
---

# State Machines

- [Deployment](deployment.puml): planning through successful commit.
- [Active Resource](active-resource.puml): provision/reconcile/readiness/unreferenced.
- [Workload Instance](workload-instance.puml): apply/readiness/update/removal.
- [Connection](connection.puml): verification before use.

`FAILED` records terminal execution failure so status does not remain misleading. Generic automated retry/resume/rollback remain outside MVP. Explicit Environment transition compensation/recovery/source cleanup are scoped by ADR-012.

- [Environment destinations and operation admission](environment-target.puml) — editable versioned selections, temporary operation claim and interrupted recovery; [ADR-012](../decisions/ADR-012-environment-stores-and-transitions.md).

## Terraform v2

- [Terraform resource instance lifecycle](terraform-instance.puml) — write-ahead
  state/ownership, bounded retry, interruption hold and separate verified cleanup.
- [State/locking contract](../contracts/terraform-state.md) — Environment operation
  + backend state lock, owner/fencing and no automatic expiry unlock.

V2 retry/cleanup requirements supersede blanket retry-out-of-scope for new
Terraform execution; full automatic recovery/import/backup remain D12–D14.
Existing ADR-012 transition behavior is not removed by this scoped deferral.
