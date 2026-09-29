---
id: VERIFY-2026-09-29-ORCH-POSTGRES-KIND
artifact: verification-record
status: current
last_reviewed: 2026-09-29
---

# Orchestrator PostgreSQL kind installation — 2026-09-29

- Context preflight: `kind-idp-internal`.
- Installed namespace `orchestrator-system` with owner label, Secret
  `postgres-auth`, Service `postgres`, StatefulSet `postgres` and PVC
  `data-postgres-0` (1 GiB, Bound).
- `postgres-0` became `Running` and `1/1 Ready`, no restart at verification.
- In-Pod `psql` returned database/user `orchestrator` and PostgreSQL 16.
- No existing `idp` or `harbor` namespace/database was modified.
- Secret content was not read or recorded. Local credential file is outside the
  repository and mode-restricted by the installer; backup/rotation not tested.
- No backend adapter or Terraform state use was asserted in this check.
