---
id: VERIFY-DEFINITION-VERSIONS-LIVE-20261103
artifact: verification-record
status: current
last_reviewed: 2026-11-03
---

# Console 3001 — authorized schema-6 installation

After the implementation handoff, the user explicitly requested backup and applying
the change to Console port 3001. Source commit: `b2a537e4`. This grants the retained
Console migration/restart; no workload, Kubernetes or cloud deployment was run.

## Backup and restore rehearsal

The existing `thieu-staging` Compose project, all its overrides, environment
values, network configuration and credential/cache volumes were identified before
mutation. No unresolved Environment operation existed. New backend/frontend images
were built from an isolated archive of the committed source, excluding the user's
staged TemplatePicker/locale changes; the backend binary was built with CGO disabled
and checked inside its actual runtime image before downtime.

Frontend and backend writers were stopped. A PostgreSQL custom-format backup,
role backup, database ACL script and private credential/encryption-key/module/render
cache archive were saved outside Git. The database archive is approximately 11 MB;
its SHA-256 was checked after installation. Vault and PostgreSQL data volumes were
retained. Vault remained running; this backup is not a Vault/workload data snapshot.

The actual database archive and original roles were restored to an owned PostgreSQL
16 container with `--network none`. All 55 original table fingerprints matched.
Initial rehearsal identified the new database's default PUBLIC privileges; the
original database ACL was captured/replayed and the migration rehearsal then passed.
The live database already had the correct privileges and required no ACL repair.

## Live migration and runtime

Owner setup used the new image and existing owner-only credential mount. It
completed the supported schema-5 → 6 upgrade before the new runtime started.
Installation UUID, creation time and schema identity were retained. All 53 existing
domain-table row fingerprints matched before/after migration; only the schema
marker and migration history changed among the original tables.

New records: 7 immutable initial revisions, 7 policies and 32 frozen Environment
configuration baselines. None is falsely marked successfully applied. The State
runtime has no SELECT privilege on the revision table. Backend/frontend health
checks passed with `orchestrator-{backend,frontend}:definition-versions-b2a537e4`.
Runtime mounts still exclude the owner credential; existing PostgreSQL, Vault,
state key, connection tokens and module/render cache volumes are preserved.

After runtime startup/login, application/Environment/Definition/resource/deployment
and Terraform instance/state fingerprints still match. Only sessions and telemetry
tables changed as expected from login and resumed observation. Counts remain
16 Applications, 32 Environments and 7 Definitions. No workload deployment operation
was added; the prior 21 operations remain SUCCEEDED.

## Browser handoff

Verified the exact Tailscale origin in an authenticated host browser:
`http://desktop-bdjp2rq-1.tail342836.ts.net:3001`.
The actual Resource Definitions page and Versions panel show migrated r1 Active;
the Environment Settings panel loads frozen selections with pending=false and
appliedVerified=false; Rendering Templates loads. Relevant real API requests
returned 200, before/after application keys and Definition versions match, and no
browser page error occurred. No requests were mocked and no mutation was used to
prove the UI. Other tailnet clients/ACLs were not tested.

Private backup, build artifacts, Compose wrapper, fingerprints, screenshots and
before/after browser inventories are retained at
`/home/thanhnt1/.local/state/orch-acceptance/definition-versions-live-20261103`.
The directory is private and is not committed. Its `README.md` describes backup
boundaries and recovery; `compose.sh up -d --no-deps --no-build --pull never --wait backend frontend`
restarts the installed images. The isolated restore container and temporary owner
URL file were removed after successful verification. Runtime and Tailscale routing
remain active for the user.

There is no automatic downgrade. Returning to schema-5 images requires a reviewed
stopped-writer restore with the original state key/Vault data and role/database
privileges, per the [upgrade runbook](../operations/postgres-upgrades.md).
