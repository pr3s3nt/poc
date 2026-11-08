---
id: VERIFY-2026-11-08-WORKLOAD-AUTHORING-UI
artifact: verification
status: current
last_reviewed: 2026-11-08
---

# Workload authoring controls — issue #47

User approved fixing Add Workload Cancel, opening Service/dependencies by default
and allowing explicit endpoint creation from Service. Branch remains
`features/organization_seed`; no main merge.

Picker dismissal could clear React state while leaving the physical overlay
blocking the page in the production Console. A dedicated parent DOM host now
removes the complete overlay. Cancel/Escape/reopen/custom selection were checked
again in the actual production build. No shared Dialog behavior was changed.

Service and dependency sections now appear open outside workload advanced fields.
An explicit action on a valid named TCP port adds the existing endpoint resource
with editable name, port and path. Existing alias/class/unknown params survive
editing. UDP, duplicate/invalid port names and explicitly disabled Service cannot
create an endpoint. Missing/removed port selection blocks Save. This uses the
existing endpoint contract/API; no automatic publishing or backend schema change.
Editor navigation retains the selected Environment.

## Evidence and limits

- 55 focused tests pass across ApplicationHomePage, WorkloadEditorPage,
  TemplatePicker and ServiceEndpointFields. Save payload includes both Service
  port and endpoint resource; this Save assertion is a component test.
- Typecheck, lint and production build pass; lint retains the existing
  PlatformSettings ref warning. Full frontend: 987 pass, 2 fail out of 989.
  The same legacy static checks (`terraform-v2-labels`, `v04-internal-full`)
  were reproduced on the pre-change baseline in the
  [preceding verification](2026-11-08-retained-progress-catalog.md).
- Real browser through the feature Tailscale URL passed picker Cancel/Escape,
  reopening/custom selection, default-visible groups, explicit endpoint creation,
  name/port/path fields and editor Cancel returning to staging. Before/after
  technical GETs confirmed identical workloads/draftVersion, with zero draft writes.
  Scenario: `e2e/scenarios/workload-authoring-ui/`; receipt/screenshot outside Git.
- Visual reference was the existing Console shell/editor and shared UI design.
  Browser screenshot confirms reused sections, buttons and field styles. Content
  review kept only endpoint labels/actions; no extra explanatory product prose.
- No application Save/Preview/Deploy was performed in this browser scenario, so
  this is authoring verification, not a new endpoint routing/deployment acceptance.
  No Go/backend change; the full kind backend comparison from the preceding
  verification was not repeated for these Console-only edits. The user-stopped
  Backstage business trial and deferred rollback scenario remain unchanged.

The dedicated feature frontend image and private resume script are updated;
Console/data/forwards remain running. Public evidence omits private URLs and
credentials. Main merge awaits user acceptance.
