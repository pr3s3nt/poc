---
id: VERIFY-2026-09-29-POSTGRES-SYSTEM-STORE
artifact: verification-evidence
status: evidence
last_reviewed: 2026-09-29
related: I06-11, D04, D08, IMP-001, IMP-013
---

# 2026-09-29 — Normalized PostgreSQL system store

## Scope

- Cluster: `kind-idp-internal`.
- Database: dedicated PostgreSQL 16 StatefulSet in `orchestrator-system`.
- Backend image: `orchestrator-backend:postgres-normalized-v2-20260929`.
- Self-hosted backend namespace:
  `app-e9728c4b-751e-4c7c-80c7-82b5c89ff02c-staging`.

No credential value was printed or stored in repository files. The existing
database was empty before verification; only test data created in this run was
reset before the final rollout.

## Results

| Check | Result |
|---|---|
| Full Go tests and build | Pass |
| Migration from empty database plus identity migration | Pass; `schema_migrations.version = 1, 2`; User Account IDs are UUID |
| Normalized repository integration test | Pass |
| Snapshot with missing Deployment | Rejected by FK |
| More than one Snapshot per Deployment | Protected by UNIQUE |
| `PROVISIONING` Deployment without Snapshot | Rejected at transaction commit by deferred constraint trigger |
| Seed recovery after partial bootstrap | Pass after per-record idempotency fix |
| Backend rollout | Pass; new Pod Ready |
| Application creation | Pass; generated ID `013e5192-ccd7-43c8-9254-6dc043da0618` |
| Backend restart | Pass; Application and its `staging`/`production` Environments remained queryable |
| `pg_dump`/`pg_restore` into disposable database | Pass; Application count matched (`2`) |
| Disposable restore database cleanup | Pass |

## Remaining operational work

The one-off backup/restore path is verified. Automated schedules, encrypted
off-cluster storage, retention and restore drills remain deployment operations
work; Terraform physical state remains outside this database by design.
