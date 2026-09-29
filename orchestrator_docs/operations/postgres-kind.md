---
id: RUNBOOK-POSTGRES-KIND
artifact: operations-runbook
status: current
last_reviewed: 2026-09-29
---

# Orchestrator PostgreSQL on kind

This is a dedicated, persistent PostgreSQL 16 instance in namespace
`orchestrator-system` on `kind-idp-internal`. It does not use the `idp` or
`harbor` databases. Installation is an explicit cluster mutation:

```bash
bash scripts/install-orchestrator-postgres-kind.sh
```

The script requires current context `kind-idp-internal`, creates an owner-labeled
namespace, an owner-only local password file outside the repository, Kubernetes
Secret `postgres-auth`, Service and a 1 GiB PVC-backed StatefulSet. It refuses
to rotate an existing credential or adopt an unowned namespace. It is safe to
rerun without deleting data. Neither the password nor Secret content should be
printed or committed. The local file needs backup in an owner-controlled secure
location; the PVC alone is not a backup or HA solution. Do not delete the
namespace/PVC as test cleanup.

Read-only health checks (no credential disclosure):

```bash
kubectl --context kind-idp-internal -n orchestrator-system get pod,svc,pvc
kubectl --context kind-idp-internal -n orchestrator-system exec statefulset/postgres -- \
  psql -U orchestrator -d orchestrator -Atc 'select current_database(),current_user;'
```

For a backend process outside kind, temporarily forward the Service to localhost:

```bash
kubectl --context kind-idp-internal -n orchestrator-system port-forward svc/postgres 15432:5432
```

The backend currently **does not use this database**. Its system store remains
the in-memory/JSON snapshot adapter. PostgreSQL schema migration and adapter,
Terraform `pg` state backend, async reconciliation, UC-10 rollback and UC-11
resource cleanup are separate work; a Ready Pod does not complete any of them.
