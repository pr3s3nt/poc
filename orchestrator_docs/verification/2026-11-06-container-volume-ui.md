---
id: VER-CONTAINER-VOLUME-UI-20261106
artifact: verification-record
status: current
last_reviewed: 2026-11-06
---

# Container volume and file authoring — WC-22

The user approved moving volume/file authoring into each container, with a single
Add volume or file picker and reuse of shared sources. Implemented directly in
`features/chinhform`, following [WC-22](../usecase/UC-16/workload-catalog-specification.md),
[UI design](../usecase/UC-16/ui/workload-catalog.md) and shared UI rules.

## Behavior

Each app/init/sidecar card owns Volumes & files. A shared Dialog offers PVC,
emptyDir, ConfigMap, hostPath and Secret. Local source creation and first mount
are one draft update; existing workload sources remain selectable by name.
The target is implicit. Mount cards expose path, mode and source; source settings
are collapsed and labelled shared when multiple containers use them. New mount
paths avoid existing target paths. G removing one mount retains other users,
unused sources, PVC resources and Secrets. Source names stay stable while editing.

PVC/Secret creation reuses the existing scoped dialogs, retaining the invoking
container and mount path/permissions. Cancel/Escape does not change desired state
and restores focus. Unknown/missing sources remain repairable; orphan container
references have explicit reassign/remove controls. Existing Save/CAS/error/BUSY,
metadata loading/failure and scope guards remain. ExecutionFields now owns only
runtime/schedule/preset inputs. The four global volume/file panels are removed.
API/JSON shape, storage/secret identity, immutable source pins and backend runtime
behavior are unchanged; no schema or backend deployment was needed.

## Validation

103 focused editor/runtime/storage/class/env/locale tests passed across the
relevant files. New/updated coverage includes atomic source creation, shared
source edit/reuse, removing one mount, unused-source preservation, custom paths
through PVC/Secret creation, sidecar Secret targeting, Cancel/focus restoration,
renamed-target repair and lossless reopened intent. Frontend typecheck/lint/build,
documentation/whitespace checks and PlantUML sequence generation pass. Existing
PlatformSettingsPage lint warning, async class act warnings and Vite chunk-size
warning remain; no claim is made that the full repository suite was run/green.

## Real Console/API: PASS

[Runner](../../e2e/scenarios/workload-runtime-form/README.md) created a dedicated
owned app and app-scoped Definition. It used the real dev PostgreSQL/Vault/backend,
created all five source kinds from container sections, reused emptyDir/hostPath
across app/init/sidecar, and uploaded a generated private UTF-8 Secret file through
UC-12. Save/reopen/resave equality and parsed Preview verified exact lifecycle,
mount sources/paths/modes, references and redaction. The runner waits for form
hydration and uses explicit accessible select names. No Kubernetes/cloud
provision/apply/delete was invoked.

- Application: `ae7b9a8d-a523-450d-9557-975c84dec1b7`.
- Definition: `chinhform-1793901161714`.
- Template source: `33e5ff09-46b8-4e80-83d3-e5f65fd9a8e0`.
- Latest check: `2026-11-05T17:57:44.969Z`.
- [Verified Tailscale editor](http://desktop-bdjp2rq-1.tail342836.ts.net:5191/ui/applications/ae7b9a8d-a523-450d-9557-975c84dec1b7/environments/staging/workloads/app).

The app/draft/Secret/Definition are retained for manual testing. Private receipt,
screenshots, session and generated secret input stay outside Git in
`~/.local/state/chinhform-preview/container-volumes-e2e/`. Cleanup is separate,
through product APIs for these receipt-owned identities only; do not remove shared
source/account/Secret Store/namespace/runtime/Tailscale resources. Existing user
forms and tmux sessions remain open. Frontend continues in chinhform-console on
port 5191 against endpoint-dev-backend-1 on 8098; restart instructions remain in
[prior runtime record](2026-11-06-chinhform-runtime.md).

UI reference is the existing WorkloadEditorPage/AppShell, EnvironmentVariablesTable,
shared Dialog/Button and field-grid/content-panel styles. A real in-app browser
inspection through the exact Tailscale URL confirmed the container sections and
picker. Native fieldset borders were removed locally to match shared controls;
no shell/theme/navigation redesign was introduced. Labels use VI/EN catalogs,
source settings are progressive disclosure, and duplicate global panels are gone.
Kubernetes runtime acceptance remains outside this authoring/layout change.
