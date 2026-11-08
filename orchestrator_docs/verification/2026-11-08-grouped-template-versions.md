---
id: VERIFY-2026-11-08-GROUPED-TEMPLATE-VERSIONS
artifact: verification
status: current
last_reviewed: 2026-11-08
---

# Grouped workload template versions — issue #47

The user approved one card per workload type with the latest version selected by
default and an optional older-version choice, including immediate main merge and
application to the retained Console. Issue #47 is reused for this authoring
follow-up; branch `features/workload_template_versions` starts from origin/main.

The picker groups only the existing Environment-scoped API result by template ID.
It compares numeric major/minor/patch values (including 1.10 versus 1.9), shows the
selected version's kind/label/description, and puts the version select in a
collapsed disclosure when more than one version is available. Selecting a version
alone does not navigate. Card activation passes its exact ID/version/digest through
the existing callback. Existing stored pins, edit behavior, template records,
backend query, rendering and deployment are unchanged.

## Validation

- Focused TemplatePicker/ApplicationHome tests pass 31/31, covering grouping,
  numeric ordering, exact latest/older object selection, distinct IDs, single
  versions, scope reset and existing search/cancellation/error behavior.
- Full frontend passes 988/988 across 104 files. A later parallel run had an
  unrelated observability follow/pause timing failure; that file passed 5/5 on
  focused recheck. The final full run uses four workers to reduce contention;
  and passes 988/988 across 104 files. No observability test/product code was altered.
  Typecheck/build/lint pass with
  the existing PlatformSettings ref warning and bundle-size advisory.
- Real-browser scenario `e2e/scenarios/workload-template-versions/` uses actual
  metadata and auth: 16 versions become six cards, one Backend API defaults to
  1.2.0, and both newest and selected 1.0.0 navigate with the exact API digest.
  The version control starts collapsed and changing it does not navigate. Editor
  Cancel preserves staging and before/after workload data is identical. No
  Save/Preview/Deploy or API business mutation occurs.
- The isolated feature frontend container uses the retained main backend via its
  real proxy; no mock or separate seeded dataset. Screenshots were compared to
  the existing picker/shared shell. Existing card styles and editor select styles
  are reused; product text is limited to version labels.
- No Go/schema/backend change; no repeat full backend or stopped application
  acceptance. The existing full-kind baseline and deferred issue #49 limits
  remain disclosed in the preceding integration record. Documentation and
  whitespace checks pass; local PlantUML is unavailable and no diagram changed.

Private screenshots, browser receipts and logs stay outside Git. Main frontend
rollout and final browser verification are recorded after completion below.
