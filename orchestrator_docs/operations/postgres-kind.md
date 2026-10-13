---
id: RUNBOOK-POSTGRES-KIND
artifact: operations-runbook
status: current
last_reviewed: 2026-10-12
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

## Runtime v2 và setup database mới — lựa chọn A

V2 dùng ba role PostgreSQL theo [state contract](../architecture/contracts/terraform-state.md).
Runtime Console dùng `-database-url-file`; State dùng
`-terraform-state-database-url-file` cùng database/schema và key độc lập qua
`-terraform-state-key-file`/`-terraform-state-key-id`. Credentials file chỉ
owner OS được đọc. Trong container, entrypoint có thể chuyển giá trị secret
env sang file hạn chế quyền rồi unset trước chạy Go; không đưa URL vào argv/log.

Database mới phải có owner và hai runtime login riêng. Setup tường minh bằng
`orchestrator -setup-database -database-owner-url-file <private-owner-file>
-console-database-role <console-role> -state-database-role <state-role>`;
owner chỉ khởi tạo schema rỗng/cấp quyền. Runtime chỉ validate, không tự tạo
schema hay fallback owner. Setup lặp chỉ kiểm schema/grants đã có, không regrant
hoặc sửa database retained. Code/scripts lựa chọn A còn đang validate ở I04-01;
chưa coi installer kind phía trên là đã cung cấp đủ roles v2.

Database retained trong runbook này chỉ được health check/port-forward như
đã giao. Không reset, regrant hoặc chạy setup mới vào retained installation.
Verification I04-01 dùng PostgreSQL disposable; setup Compose mới phải dùng
project/volumes riêng, không dùng project `orchestrator-local` đang có dữ liệu.

The PostgreSQL adapter writes normalized aggregate tables directly. Foreign
keys protect ownership, Environment current-set updates use optimistic version
checks, and `UnitOfWork` maps to one PostgreSQL transaction. Owner setup initializes only an empty schema; runtime verifies `schema_baseline` plus schema integrity
on reopen, without old migrations/backfill. Existing incompatible state requires
a separately approved backup/reset before serving; this runbook does not authorize
resetting the retained kind database. See the
[fresh baseline](../architecture/database/schema.md#fresh-persistence-baseline-t28--leg-08).
Historical one-off `pg_dump`/`pg_restore`
has been verified; automated schedules, encrypted off-cluster storage and
retention remain operational release work. Terraform v2 uses its encrypted internal HTTP state backend; async reconciliation, UC-10 rollback and
UC-11 cleanup are separate work.
