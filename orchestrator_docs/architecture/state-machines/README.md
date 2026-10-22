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

`FAILED` records terminal execution failure so status does not remain misleading. Retry transient tối đa một lần trong deadline thuộc state contract.
Automatic full resume/rollback vẫn ngoài scope. Explicit Environment transition compensation/recovery/source cleanup are scoped by ADR-012.

- [Historical Environment destination selector](environment-target.puml) — deployment selection/transition retired by ADR-017/V06.08. Current per-resource placement follows ADR-017; Secret Store and operation owner/recovery contracts remain in UC-01/UC-12 and ADR-012.

## Terraform v2

- [Terraform resource instance lifecycle](terraform-instance.puml) — write-ahead
  state/ownership, bounded retry, interruption hold and separate verified cleanup.
- [State/locking contract](../contracts/terraform-state.md) — Environment operation
  + backend state lock, owner/fencing and no automatic expiry unlock.

Retry/cleanup dùng một bộ luật ở state contract/UC-07; full automatic
recovery/import/backup nằm ở D12–D14.
Existing ADR-012 transition behavior is not removed by this scoped deferral.

- [Review node progress projection](review-node-progress.puml) — ADR-019 display states from persisted facts; not new lifecycle enums.
