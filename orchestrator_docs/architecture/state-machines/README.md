---
id: STATE-MACHINE-INDEX
artifact: state-machine-index
status: current
last_reviewed: 2026-09-21
---

# State Machines

- [Deployment](deployment.puml): planning through successful commit.
- [Active Resource](active-resource.puml): provision/reconcile/readiness/unreferenced.
- [Workload Instance](workload-instance.puml): apply/readiness/update/removal.
- [Connection](connection.puml): verification before use.

`FAILED` records terminal execution failure so status does not remain misleading. Generic automated retry/resume/rollback remain outside MVP. Explicit Environment transition compensation/recovery/source cleanup are scoped by ADR-012.

- [Environment destinations and operation admission](environment-target.puml) — editable versioned selections, temporary operation claim and interrupted recovery; [ADR-012](../decisions/ADR-012-environment-stores-and-transitions.md).
