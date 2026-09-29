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

The backend can use this database for durable single-process staging state by
passing `-database-url-file <path>`, or in its container by supplying
`ORCHESTRATOR_DATABASE_URL` from a Kubernetes Secret. The entrypoint writes the
value to an owner-only file and unsets the environment variable before starting
the Go process. Do not put the URL in a manifest or command argument.

The PostgreSQL adapter writes normalized aggregate tables directly. Foreign
keys protect ownership, Environment current-set updates use optimistic version
checks, and `UnitOfWork` maps to one PostgreSQL transaction. Migrations are
serialized and recorded in `schema_migrations`. One-off `pg_dump`/`pg_restore`
has been verified; automated schedules, encrypted off-cluster storage and
retention remain operational release work. Terraform `pg` state backend, async reconciliation, UC-10 rollback and
UC-11 cleanup are separate work.
