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

`FAILED` records terminal execution failure so status does not remain misleading. Automated retry, resume, rollback and cleanup are not implied and remain outside MVP happy path.
