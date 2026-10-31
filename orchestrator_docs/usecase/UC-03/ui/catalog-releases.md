---
id: UC-03-CATALOG-UI
artifact: ui-design
status: current
last_reviewed: 2026-10-30
---

# Catalog releases UI

Reuse AppShell navigation/gates and PlatformSettingsPage create-form, fields,
Button, status/error and advanced details patterns. Reference inspected in a
local browser with fixture API data before implementation. Product shell/styles
are reused without redesign. All text uses VI/EN runtime message catalogs.

PE/Admin opens **Nền tảng → Catalog**. Source list offers edit and manual refresh;
form includes ID, repository and branch. Tag filter, manifest path, prereleases
and platform account slots are in Advanced. List shows observation time and safe
status; release details show tag/commit. Refresh is discovery, never application
deployment. Errors keep nonsecret draft; stale/uncertain saves require reload.
Loading, empty, API error and retry have explicit states.

Environment Settings adds a Catalog panel beside the existing delivery settings.
It displays applied/desired/status, available releases and a Save action. Save
retains desired configuration only; a short notice links to Preview. Selection
uses current Environment and catalog revision CAS. Busy operations disable Save;
failed loads show Retry. Scope changes discard old requests and do not carry
selection across staging/production. Backend blockers stay authoritative.
