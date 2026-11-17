---
id: VER-SCORE-TAB-SYNC-20261117
artifact: verification-record
status: current
last_reviewed: 2026-11-17
---

# Synchronized workload form and Score/extension — issue #83

The user approved synchronization on tab changes. Implementation follows
[SC-06](../architecture/contracts/score-workload.md),
[UC-16](../usecase/UC-16/specification.md) and
[screen behavior](../usecase/UC-16/ui/screens.md). This is a frontend refinement
on `features/score`; no backend/planner, infrastructure or persistence contract
implementation changed.

## Behavior and focused checks

Entering the document tab fills both Score and extension from current authored
state through the existing export API. Returning to the form checks current text
through parse before replacing form state. Invalid documents stay visible and
block stale Save/export; a rejected export leaves form edits intact. An untouched
new form may start with blank document editors. Saved advanced documents load
their text but keep the existing lossless form restriction.

The form validation is reused at conversion. A specific Service-row guard keeps
unfinished/duplicate/invalid port rows from being silently dropped by dictionary
serialization. Pending conversion locks tab/Save/export actions. Scope, import
attempt and form identity guards discard outdated responses, including an export
whose form was edited while the request was in flight.

The Score document component suite passes 12 tests, including loaded/unsaved
round trips, explicit zero/false, secret references, template pins/defaults,
invalid text, failed export/retry, incomplete variable/Service rows, pending
locks, newer form edits, Environment changes and advanced-document preservation.
The prior focused authoring run also passed all 76 tests before the new cases.
A strict TypeScript error in a new test's indexed input lookup was corrected
before the complete final validation; it was not a product runtime failure.

## Real Console and browser

The [tab synchronization runner](../../e2e/scenarios/score-authoring/tab-sync.mjs)
uses actual APIs and the feature Console through its exact Tailscale origin.
It creates a separate Application, imports the existing fixture and checks:

- YAML-to-form on switch without an extra Check click;
- unsaved form image changes reflected in generated Score;
- YAML variable and extension replica changes reflected back in the form;
- resource references and exact template pins retained;
- invalid YAML retained, stale actions blocked and no implicit Save;
- unfinished Service row retained with an error;
- explicit Save, reopening equivalent documents, VI/EN and keyboard activation.

The final run passes and its owned Application deletion reaches SUCCEEDED.
An earlier runner attempt inserted a Humanitec replica field alongside an emitted
native replica field; the existing overlap guard correctly rejected it. The
runner was corrected to edit the emitted representation. Failed evidence and
successful owned cleanup are retained; no guard/assertion was relaxed.

A separate read-only check opens the user's existing Keycloak editor on the same
final frontend. Generated documents retain application/database Secret references
and the managed URL reference. Return-to-form retains the frozen template pin,
startup probe and explicit false security setting. An unsaved image edit is
reflected in YAML. Comparing the saved workload list before/after proves that
no user draft or deployed workload was changed. No new Preview/Deploy was run.

The original AppShell, controls, preview-form styles and prior editor screenshots
are the visual references. Final VI/EN screenshots retain the same shell, tabs,
field sizes and collapsed parsed-document details. The only new visible guidance
is short pending/validation status; no additional instructional panel is needed.
Local files are updated only by explicit download.

## Validation and retained services

Final isolated kind validation passes **113 files / 1072 tests**, typecheck,
lint and production build. Lint retains the existing PlatformSettingsPage ref
warning; build retains the existing chunk-size warning. A preceding complete
1071-test run passed before the final Service-row guard; the entire suite was
rerun after that change, rather than reusing the older result.
All 471 frontend files in that runner match the reviewed working tree hashes.
Backend product sources match the preceding full/runtime verification exactly
(`backend/internal`, `backend/cmd`, `go.mod`, `go.sum` against `137909bb`). Its
[baseline-red full comparison](2026-11-16-score-authoring.md) and runtime evidence
are reused; no new green-backend claim or backend rerun is made.

The feature frontend image/container was updated independently. Backend, user's
Keycloak/PostgreSQL, route, Vault and data volumes were kept running. Exact URL,
credentials location, restart procedure, screenshots, source hashes and receipts
stay outside Git. User acceptance is still required before merge/main rollout.

The final docs checker and whitespace/syntax checks pass. The updated UC-16
sequence passes PlantUML 1.2025.2 container check-only (the host checker reports
PlantUML unavailable). The receipt-owned full-check namespace was deleted using
matching ownership label and UID/resourceVersion preconditions, and absence was
verified. All disposable browser Applications were cleaned through product APIs;
no unrelated user changes were present or committed.
