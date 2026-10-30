---
id: OPS-POSTGRES-UPGRADES
artifact: runbook
status: current
last_reviewed: 2026-10-28
---

# Upgrade Orchestrator PostgreSQL without deleting data

[ADR-027](../architecture/decisions/ADR-027-postgres-upgrades.md) replaces the
old fresh-only PostgreSQL policy. Supported sources: baseline files at `1516dec`
and `2a111db`; their checksums and exact schema must match. Unknown versions
require a reviewed migration. Never delete a retained database or edit markers
just to start the new application. Workload databases/JSON snapshots are outside
this upgrade path. No deployment or external mutation is authorized by this text.

## Compose deployment

Identify the exact project and compose overrides first. The script below builds
images, stops Console writers/frontend, takes a private custom-format dump,
checks the archive can be listed, runs owner setup, and starts the new application
only after successful upgrade. PostgreSQL/Vault volumes, state key and credential
volumes remain. Backup contains private metadata/state: keep it outside Git with
restricted access. Archive inspection detects incomplete archives but does not
replace a rehearsed restore. Rehearse restore on an isolated server before a
production upgrade. Stop any additional writers, workers or external clients too.

```bash
bash deploy/local/upgrade-database.sh /private/backups/orchestrator-unique.dump \
  -p YOUR_PROJECT -f docker-compose.yml -f YOUR_OVERRIDE.yml
```

Provide the same project/options/environment used to start that installation;
omit the override flag if none. Compose must support `up --wait`. Select the new
source/image first. A failed build leaves services running. Backup/setup failures
leave application writers stopped; inspect and fix before retry. Use a new backup
path each run. The script does not restore or delete data automatically.

`db-setup` automatically initializes an empty schema or upgrades a supported
retained schema through `-setup-database`, using only its owner credential mount.
The backend depends on its successful completion. For existing installations use
the wrapper above so old writers are stopped and setup is always rerun; a bare
restart or reusing an old successful setup container is not an upgrade procedure.

## Other deployments and recovery

Quiesce all writers, back up the database and preserve the existing state key,
Vault data and bundle cache. Run the new binary's `-setup-database` with the
existing owner URL file and both runtime role names; then start the new runtime.
No owner credential is mounted in runtime. Verify setup exit status, application
health, login, saved configuration and Preview before resuming normal deployments.

Owner setup locks the upgrade transaction and commits schema, migration history,
grants and marker together. Failure rolls them back; correct the reported cause
and rerun. Do not start an older binary against an upgraded database. If reverting
the release is necessary, restore the pre-upgrade backup to an isolated target
with the original owner/runtime role names, verify structure/data/installation
identity and original state key, then cut over with writers stopped. Coordinate
restoring database metadata with any external resource/state changes since backup;
restoring metadata alone cannot undo cloud/Kubernetes changes. Keep the failed DB
for diagnosis. Unknown or damaged source errors require investigation, not reset.

## Developer checks

Follow [AGENTS.md](../../AGENTS.md). Add a new immutable SQL migration and complete
version reference, register its ordered transition and update active schema,
grants and documentation. Do not edit existing files under `postgres/migrations/`.
Run `python3 scripts/check_migrations.py --base origin/main`, docs checker and real
PostgreSQL integration tests with `REQUIRE_POSTGRES=1`. CI checks immutable files,
version additions and structural equivalence; it cannot establish migration
semantics without populated-source tests. Add preservation, rollback, concurrent
execution, history integrity and least-privilege runtime assertions.

For backup/restore integration tests, install matching `pg_dump`/`pg_restore`
clients, or set `ORCHESTRATOR_POSTGRES_TEST_CONTAINER` to the disposable PostgreSQL
container ID (local trust authentication inside that container only). CI passes
its service container ID. This setting must never name a retained database server.
`TestUpgradeBackupRestore` restores a populated pre-upgrade archive to a separate
owned test database, compares row fingerprints and reruns the upgrade there.

Use the CI `go test -timeout 30m ./...` timeout for the full backend suite. If
local `/tmp` is a crowded tmpfs, set `TMPDIR` to a new task-owned directory on a
disk with enough space: Terraform executor fixtures copy multiple real binaries.
Do not clear another session's temporary files to make tests pass.

## Current version: 2 — platform Git settings

The current runtime also accepts the frozen version-1 schema as an upgrade
source (marker `orchestrator-schema-v1`). Owner setup validates its history and
catalog, applies `002-git-settings.sql`, grants the new table to Console only,
and records version 2 atomically. Original migrations/references remain frozen.
Legacy supported sources first transition through version 1. Run the same
backup/quiesce/owner-setup procedure above; do not reset existing data.
