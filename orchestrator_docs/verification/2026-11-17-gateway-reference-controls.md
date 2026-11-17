---
id: VERIFY-20261117-GATEWAY-REFERENCE-CONTROLS
artifact: verification-record
status: current
last_reviewed: 2026-11-17
---

# Gateway reference controls — issue #84

The user approved replacing raw reference inputs with the selection patterns used
by other Definitions. Implementation `12a4fb5c` is pushed to features/Httproute.
Required behavior is in the [UC-03 specification](../usecase/UC-03/specification.md#gateway-reference-controls--approved-2026-11-17)
and [screens](../usecase/UC-03/ui/screens.md#gateway-reference-controls--issue-84-approved-2026-11-17).
No backend, schema, template source, shared Gateway/controller or cloud change.

## Behavior and focused checks

Gateway selects a cluster class; Gateway-bound endpoint selects a Gateway class.
Both use Resource Type metadata and shared class rules to serialize the whole
output reference for the current Application/Environment. The expression is in
collapsed technical details. New forms require an explicit choice. Stored
unavailable classes and advanced references remain visible and unchanged until
explicit replacement. Legacy endpoint policy fields and Gateway driver/layout
remain intact. Reset clears the previous Gateway cluster choice.

28 focused tests pass across EndpointDefinition, ExistingClusterDefinition,
moduleMapping and valueSource. They include generated JSON matching an explicit
class selection, preserving an advanced reference and preserving Echo/legacy
driver/layout/version. Typecheck/lint/build pass; existing PlatformSettingsPage
lint and bundle-size warnings remain. Docs checker, whitespace and Node syntax
pass; local docs check reports PlantUML unavailable, with no diagram changes.

## Real browser review

The isolated task frontend was rebuilt and replaced only after build success;
backend/database/Vault and retained trial data remain running. The review phase
opens the old legacy and current Echo Gateway/endpoint forms over the actual
Tailscale Console URL. For all four Definitions it confirms the stored class,
clears/reselects it, then checks the generated JSON reference, driver and version.
It never submits Save, and exact before/after Definition API comparison passes.
No additional test Definitions or revisions were created.

UI reference: live existing namespace/provider class controls, Resource Definition
editor, AppShell and shared styles/components. Visual review confirms the same
layout and labels, with the raw expression replaced by one dropdown and collapsed
technical detail. No extra explanatory banner or shell redesign was introduced.
Private screenshots and receipts stay outside Git/public issues. The separate
browser phase also verifies the Application API and two actual workload pages.

## Full checks and source attribution

Fresh full frontend checks passed on the existing authorized kind cluster:
113 files / 1078 tests, install/typecheck/lint/build all exit 0 on Node 22.23.4.
The owner-labelled disposable namespace was deleted with owner/UID/API
preconditions and its absence confirmed. Source is the exact `12a4fb5c` archive,
frontend tree `d9689027a4c0cb193b9a48285a2be0190e9ed59f`.

Backend tree `a6c9c7a8f26a99b564d8886d2794ccdcb44b69c6` is identical to tested
`df719115`. Completed candidate/main evidence is explicitly reused from the
owned and cleaned [Echo run](2026-11-17-echo-gateway.md), with the same main
`ab37e2d4`. It is not a fresh backend execution: both builds pass, while both
suites retain identical 101 emitted top-level failure events, 20 failed packages,
8 skips and the 30-minute PostgreSQL package timeout. No full green backend gate
or completed PostgreSQL suite is claimed. Backend and frontend trees are
unchanged by the later documentation-only execution checkpoint.

User acceptance, feature merge into main and retained-stack rollout remain
outstanding. YAML/JSON editing for all Definitions with bidirectional UI sync is
a separately requested extension with an issue proposal pending; it is not
implemented by this class-control checkpoint.
