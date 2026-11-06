---
id: VER-REUSE-FIRST-VOLUMES-20261106
artifact: verification-record
status: current
last_reviewed: 2026-11-06
---

# Existing volume sources before creation — WC-23

The user approved an existing-first picker and requested commit/push on
`features/chinhform`. [WC-23](../usecase/UC-16/workload-catalog-specification.md)
and [UI design](../usecase/UC-16/ui/workload-catalog.md) own the behavior.

## Implemented behavior

Add volume or file initially lists reusable sources, searchable by name/source
workload/container and filterable by type. Each entry shows type and known mount
users; shared PVC entries identify the source workload. No source is selected by
default. Search/type changes clear selection so a hidden prior choice cannot be
attached. Selecting an existing source shows the target path/mode; Create a new
source explicitly opens creation controls, with a return to existing sources.

Sources are current-workload named volumes, local storage dependencies, compatible
shared PVC resources from the current app/environment and scoped configured
Secrets. Other workloads' emptyDir/ConfigMap/hostPath sources are never offered.
Loading/error metadata is distinct from empty/search-no-results; local named
sources remain usable when the Secret catalog fails. Secrets remain references.
Shared PVC reuse uses the existing safe sharedResources projection and existing
selection guard, preserves shared ID/typed parameters, and atomically adds a
collision-free dependency alias and container mount. Save/Preview/Deploy, CAS,
source pins, storage retention and secret lifecycle remain unchanged.
No backend/API/schema or runtime image change is part of this UI refinement.

## Validation

Frontend typecheck/lint/build and 106 focused tests (runtime/source/storage/editor,
class, env and locale) pass. Coverage includes existing-first display, search/type
filtering, no default/hidden selection, scope exclusion, metadata failure, source
creation/cancel, cross-container source sharing, shared-PVC identity and alias
collision handling. Existing PlatformSettingsPage lint warning, async class act
warnings and Vite chunk-size warning remain. Documentation checker, PlantUML
sequence generation and whitespace checks pass. No full repository suite or
backend tests were rerun for this frontend-only change.

## Real Console/API result: PASS

The [runtime-form runner](../../e2e/scenarios/workload-runtime-form/README.md)
created a dedicated owned app/Definition through the real dev backend and scoped
PE/Developer sessions. It created all five source kinds, reused named volumes
from the initial list in app/init/sidecar, read a generated UTF-8 file through
UC-12, then saved/reopened/resaved without changing authored intent.

A second workload `reuse` selected the first workload's shared PVC directly from
the existing app-source list. Persisted resource ID/typed inputs remained equal;
parsed Preview confirmed both workloads reference the same PVC claim and the
second container mounts it at `/reused-data`. Preview also preserves the original
five sources/lifecycle/modes and keeps Secret bytes absent. No Kubernetes/cloud
provision/apply/delete was requested.

- App: `0c7470c8-121d-4507-a2e4-08eb0d487495`.
- Scoped Definition: `chinhform-1793903377848`.
- Prepared source: `33e5ff09-46b8-4e80-83d3-e5f65fd9a8e0`.
- Reused shared storage ID: `resource-75e8103d42ce1601234d5536647785e1`.
- Checked at: `2026-11-05T18:33:05.069Z`.
- [Verified Tailscale editor](http://desktop-bdjp2rq-1.tail342836.ts.net:5191/ui/applications/0c7470c8-121d-4507-a2e4-08eb0d487495/environments/staging/workloads/app).

The app, two drafts, scoped Definition and Secret are retained for manual testing.
Private receipt/screenshots/generated Secret input stay outside Git under
`~/.local/state/chinhform-preview/reuse-first-e2e/`. Cleanup is separate through
product APIs for exact receipt-owned identities only; shared source/account/
Secret Store/namespace/runtime/Tailscale resources must remain. Existing user
forms/tmux sessions were preserved. Frontend remains on 5191 against backend 8098;
restart instructions remain in the earlier runtime verification record.

UI reference: existing WorkloadEditorPage, container-volume Dialog, shared
Button/field-grid/content-panel controls and UI tokens. The exact Tailscale editor
was opened independently in the in-app browser. Visual review confirmed a compact
search/filter/list, mount-user labels and secondary creation action; technical
source settings remain hidden until creation/editing. The same registered dev
runtime stays running; cluster execution is not claimed by this authoring run.
