---
id: VERIFY-DEFINITION-VERSIONS-20261103
artifact: verification-record
status: current
last_reviewed: 2026-11-03
---

# Controlled Definition upgrades — 2026-11-03

Scope: [UC-03 DV-01–07](../usecase/UC-03/definition-versions.md), with existing
forms and Catalog module sources preserved. [Implementation boundaries](../implementation/definition-versions.md)
remain authoritative. This work does not authorize workload/cloud deployment or a
retained PostgreSQL upgrade.

## Automated checks

- Full `go test ./...` and `go build ./...` passed. Focused catalog, pending,
  deployment, HTTP and bootstrap checks passed again after final corrections.
- Real PostgreSQL 16, owned disposable container
  `orch-definition-version-tests-20261103`: full adapter suite passed (722.497s),
  with `REQUIRE_POSTGRES=1` and container client tools. No retained database used.
- Additional populated version-5 → 6 tests passed (33.634s): original-table
  fingerprints, matching criteria/editor version and installation/history retained;
  repeat setup; rollback after grant-validation failure; immutable snapshot SQL
  guard; State isolation; organization/CAS/busy selection; owner-fenced activation
  and refusal after lease release. Captured configuration is not marked applied.
- Revision lifecycle tests cover Proposed/default independence, frozen Environment
  resolution, explicit selection, CAS, archived revisions and immutable snapshots.
  Legacy running JSON environments freeze before defaults can change; missing
  historical renderer evidence blocks substitution.
- Pending tests exercise exact Git rendering A → B → A through explicit selection,
  stale-token rejection, unavailable old installed renderer requiring a selected
  new revision, and no-op preservation. Definition-only database upgrades are
  blocked by Preview; direct Deploy cannot bypass the pending selection. Blocked
  operations leave resource execution and applied policy unchanged.
- Retained Terraform/renderer/provider tests cover original identities after a
  platform restart, preservation of old provider versions and drift rejection.
- Frontend: 95 files / 996 tests passed using four workers. An earlier unrestricted
  parallel run hit a timing failure in an unchanged observability test; the complete
  bounded-worker rerun passed. Final affected form/panel run: 108 tests passed.
  Typecheck/build passed. Lint has zero errors and the existing
  `PlatformSettingsPage.tsx` hook-cleanup warning. Vite retains its chunk-size warning.
- Documentation, PlantUML, migration immutability guard and whitespace checks passed.

## Authenticated browser and UI review

Exact preview origin checked from the host browser:
`http://desktop-bdjp2rq-1.tail342836.ts.net:3022`.
Rendering page: `/ui/platform/rendering-templates`.
Environment page: `/ui/applications/acceptance/settings?environment=staging`.

Real product UI/API, private isolated JSON fixtures and fake execution adapters:
edit existing Rendering Template → save Proposed r2 → compare r1/r2 → promote r2;
staging stays frozen; select r2 explicitly from Settings; API confirms pending
selection and unverified applied state. Requests were not intercepted or mocked.
This is not evidence of a real infrastructure or CD deployment. No other tailnet
client/ACL was tested.

Visual comparison used the existing Definition/Rendering form and Settings shell,
plus the pre-change screenshot. Panels reuse shared catalog rows, disclosure and
buttons. Before/after configuration stays in disclosure; unavailable legacy rows
have no useless action, selectors reuse the existing styled catalog field row,
and unchanged rows have no disabled full-width upgrade button. The form layout
and Catalog source controls are retained.

## Runtime, evidence and rollout

Private runtime/evidence directory:
`/home/thanhnt1/.local/state/orch-acceptance/definition-versions-20261103`.
It contains the source-isolated frontend build, binary, JSON fixtures, start script,
logs, screenshots and browser result. User-staged TemplatePicker/locale work is
excluded from the preview build and task commit. Restart with its `start.sh`;
Tailscale Serve maps port 3022 to the same local listener. Keep both alive for review.
The preview has fixture accounts and synthetic resources; it owns no cluster/cloud.

The original Console on port 3001 and its schema-5 PostgreSQL/Vault/cache volumes
are unchanged. Applying schema 6 requires separate environment authorization under
[ADR-027](../architecture/decisions/ADR-027-postgres-upgrades.md), then stopped
writers, a verified private backup/restore rehearsal, owner setup and runtime
restart per the [upgrade runbook](../operations/postgres-upgrades.md). No reset,
manual marker edit, automatic data rollback or existing workload upgrade is part
of that rollout. Original tool/cache volumes must survive recreation.
