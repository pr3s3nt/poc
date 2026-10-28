---
id: VERIFY-2026-10-28-WORKLOAD-SHARED-RESOURCES
artifact: verification-record
status: current
last_reviewed: 2026-10-28
---

# Default shared workload resources

Scope: [UC-16 BR-28–30](../usecase/UC-16/specification.md), new/existing
resource selection, generated IDs, advanced private scope and lossless existing
identities. HTTP/schema/planner scopes are unchanged.

## Browser evidence

[Scenario](../../e2e/scenarios/workload-shared-resources/README.md) ran against
`backend/test/fixtures/deployment-review`: product UI/HTTP handlers with
in-memory storage and **fake execution adapters**. No Preview/Deploy, retained
database mutation, Kubernetes or AWS operation was performed.

Assertions passed: new shared ID with no manual input; worker uses API database
under a different local alias and locks configuration; saved edits preserve ID;
other Application and production cannot select staging resources; explicit
advanced private omits ID. No browser JavaScript errors.

VI/EN at 1512×980 and 1280×900, expanded/collapsed sidebar: no page overflow.
Sources inspected: AppShell, tokens/app styles, Button, Advanced, shared Dialog,
WorkloadEditorPage and existing dependency controls. Browser reference before
edits and final screenshots were inspected. Shell, cards and footer retain the
product layout. The saved-resource selector says Current resource rather than
Create new; redundant provisioning help was removed. The scope summary remains
visible when advanced is closed. These are product screens with synthetic data,
not evidence of real database provisioning.

Private artifacts outside Git: `/tmp/orch-shared-resource-review/before.png`,
`/tmp/orch-shared-resource-review/final/` (eight locale/viewport/sidebar screenshots
and `assertions.json`), and browser logs. Fixture password/session is not included
in the record or Git. Task-owned fixture process is stopped after checks; its
in-memory applications are discarded. No cluster cleanup is needed.

## Automated checks

- Frontend typecheck, lint and production build passed. Build retains the
  existing large-chunk advisory.
- Full frontend suite: `npm test -- --maxWorkers=2`, 80 files / 936 tests passed.
  The initial unconstrained run hit worker setup timeouts under host load;
  no assertion was weakened. Limiting concurrency resolved the timeouts.
- Focused workload suite: 7 files / 106 tests passed, covering new/inline reuse,
  locked params, private and imported identities, unavailable/conflicting
  projections, stale selection, generated IDs on internal HTTP and Save retry.
- After the final selector review, the three affected test files passed again:
  61 tests; the rebuilt Console browser scenario also passed all six assertions.
- Documentation checker, diff whitespace checks, changed runner syntax checks
  and PlantUML `-checkonly` passed.

The existing kind form runner now selects the backend's resources for its worker;
its real provisioning run was not repeated because this task changes the UI
boundary and does not authorize external deployment. Go product/schema code is
unchanged, so no backend full suite or PostgreSQL integration was run.
