---
id: VER-20261102-DEFINITION-CATALOG
artifact: verification-record
status: current
last_reviewed: 2026-11-02
---

# Definition forms with Catalog modules

User-approved [UC-03 CM-01–04](../usecase/UC-03/catalog-releases.md#definition-authoring-and-catalog-modules)
restores direct Resource Definition/render authoring and adds Catalog as a
Terraform module source. Existing Git, uploaded ZIP and registered module choices,
matching criteria, typed variables/mappings, accounts and version CAS remain.

Backend validates Catalog source/tag/component against the exact verified module
pin. Provenance persists in existing driver_inputs JSONB without schema migration.
Module-only releases are supported. Runtime/matching compose authored Definitions
with optional selected snapshots; identical authored keys replace snapshot keys.

Validation passed:

- Backend `go test ./...` and `go build ./...`; final focused catalog, release and
  HTTP suites passed after metadata/diagnostic changes. New coverage checks scoped
  module choices, module-only publication, pin/provenance mismatch, hash drift,
  immutable version behavior and authored same-key replacement.
- Frontend typecheck/lint/build, full suite 94 files / 992 tests. A first unconstrained
  parallel run hit a timing failure in an unchanged observability test; the complete
  suite passed with two workers. Final focused five Catalog tests also cover saving
  and reopening the form with the original Catalog version despite a newer release.
- Documentation checker, PlantUML regeneration, E2E syntax and `git diff --check`.

Retained `thieu-staging` backend/frontend were recreated with task-only images
`orchestrator-backend:definition-catalog-20261102` and
`orchestrator-frontend:definition-catalog-20261102`. Existing database, Vault,
volumes, cache and dependencies remain. The isolated frontend build excluded the
user's pre-existing TemplatePicker and related locale changes; those workspace
changes remain untouched. Restart/evidence directory:
`/home/thanhnt1/.local/state/orch-acceptance/definition-catalog-20261102`, `start.sh`.
Backend is a CGO-disabled build over the previously retained runtime/tool image.

Real authenticated browser verification from this host passed through
<http://desktop-bdjp2rq-1.tail342836.ts.net:3001/ui/platform/resource-definitions?resourceType=postgres>.
The [read-only scenario](../../e2e/scenarios/definition-catalog/README.md) selected
Catalog `catalog-only-20261102`, release `v0.20261102.2`, component
`module-postgres-k8s`, loaded its exact verified schema/pin, opened matching
criteria, retained selection across tabs and switched among source modes.
Rendering Templates route also loaded. No browser runtime errors occurred.

The first browser attempt used Playwright exact label lookup on a wrapped native
select; option text is included in that lookup. The runner now uses the select's
accessible combobox role/name and passes without changing product accessibility.

Visual review used the existing authenticated Catalog/AppShell reference and the
restored Definition form. Identity, sidebar, cards, typed fields and sticky actions
are reused. The new source UI adds only three selectors; pin details remain in a
collapsed disclosure by default. Required missing-field validation remains visible.

All 16 Application records and Environment metadata were identical before/after
container recreation. The browser run preserved all seven stored Definitions and
saved no draft, refreshed no source and deployed no workload. Service/component
tests exercise Save/CAS; no new live Definition was written. No Terraform/provider
execution, cloud/cluster mutation or database/storage upgrade was performed, and
access from other tailnet devices/ACLs was not asserted. Prior Catalog-only live
workload acceptance remains historical evidence for its original run.
