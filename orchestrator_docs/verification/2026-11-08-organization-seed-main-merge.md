---
id: VERIFY-2026-11-08-ORGANIZATION-SEED-MAIN-MERGE
artifact: verification
status: current
last_reviewed: 2026-11-08
---

# Organization seed integration and retained Console upgrade

The user accepted integration and authorized merging without repeating stopped
application scenarios. Main merge `0b180b33` contains organization seed, compatible
security templates, the previously integrated retry/rollback work, retained-node
progress, Catalog restrictions and workload authoring controls. Test maintenance
commit `6b376a73` retires historical label/source-ID scans; runner safety and actual
missing/failed evidence checks remain. Full frontend passes 986/986 (104 files);
focused historical tests pass 16/16. Lint/docs/whitespace checks pass with the
existing lint warning and unavailable local PlantUML noted.

No backend product code changed after the
[complete-tooling kind comparison](2026-11-08-retained-progress-catalog.md): its
15 failing packages match the prior main comparison, so this is not a claim of
full backend green. No repeat application Deploy or user-stopped/deferred runtime
scenario was included. [Rollback evidence and remaining gap](2026-11-07-environment-deploy-rollback.md)
and [stopped frontend/backend trial](2026-11-08-frontend-backend.md) remain valid
limitations; issue #49 remains open for its unverified live scenario.

## Retained Console upgrade

The exact previously built/tested product images were tagged for merge `0b180b33`
and imported into the existing kind node. Backend code is unchanged since its
progress image and frontend product code since its workload authoring image;
subsequent changes are tests/docs. This preserves the embedded template artifacts
and uses the already verified images rather than silently changing render pins.
The checked-in Console manifests were applied to its existing installer-owned
namespace. No workload namespace, source pin, cloud resource or shared service
was rewritten.

Writers/frontend and Vault were quiesced for backup. A private PostgreSQL dump
was restored successfully into a separate temporary database, then that database
was dropped. Vault data, generated credentials, state key and backend files were
archived privately; archive integrity was checked. Owner setup completed before
runtime startup. Backend, frontend, PostgreSQL and Vault are Ready. The temporary
read-only backup Pod was removed using exact UID/resourceVersion preconditions;
absence is verified. Persistent volumes are retained.

Real browser accessed the exact main Console Tailscale URL with its existing
PLATFORM_ENGINEER account. Applications and Environment metadata compare unchanged
before/after upgrade. Application home, Definitions and the new-workload editor
load; Cancel/Escape/reopen/custom selection work, Service and dependency groups
are visible without advanced disclosure, and editor Cancel preserves staging and
unchanged workloads. Definitions GET succeeds and Catalog navigation is absent.
No business write was performed. The first request during forward reconnection
returned an empty response; subsequent actual browser/API checks pass. A shared
ADMIN-only scenario refused this account as expected; final checks used its
actual role without bypassing authorization. Endpoint form creation/editing was
already verified on the feature Console and is not claimed as rerun here.

The visual reference remains the existing Console shell/editor and shared UI
patterns. Browser screenshots confirm the same shell and authoring sections.
Private receipts/backups/screenshots remain outside Git. The checked main URL is
provided in the user report, not in this public record.

## Cleanup and handoff

The feature branch is deleted locally and remotely. Feature frontend/backend
containers are stopped. PostgreSQL volumes and the feature Vault required by
existing deployed workloads are retained, as are their endpoint forwards; no
application or secret is removed as a side effect of integration. The main
Console remains running, with its task-owned Tailscale/cluster forward sessions.
Other user stacks, branches and tmux sessions are untouched. See the retained
[Console runbook](../../deploy/kind-k8s-4f/README.md) for restart/upgrade.
