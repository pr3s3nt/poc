---
id: IMP-NAMESPACE-TEMPLATE-ROUTES
artifact: implementation-handoff
status: current
last_reviewed: 2026-11-17
---

# Namespace placement and template routes — issue #84

Branch `features/Httproute`, based on main `cd40a792` and explicitly fast-forwarded
to Score checkpoint `621fa61d` by the user's request. No merge to main is approved.
Required shared design: [ADR-043](../architecture/decisions/ADR-043-namespace-placement-template-routes.md).

## Current progress

- Approved issue created: [#84](https://github.com/pr3s3nt/poc/issues/84).
- Read-only live inspection confirmed the retained kind Gateway is Accepted and
  Programmed, its HTTP listener allows routes from all namespaces and supports
  HTTPRoute. This is infrastructure readiness, not task application acceptance.
- Read-only projection of retained Console Definitions confirmed endpoint policy
  has literal Gateway/domain fields without cluster binding; PostgreSQL binds
  separately to namespace output and cluster provider. No definition was edited.
- The separate Score review installation has no endpoint Definition; its catalog
  must not be mistaken for the retained Console catalog.
- The inspected release base `b9437b4bdbd4da696e946b964540ef5592a521c6` had
  Service templates without HTTPRoute. The feature branch now publishes the
  capability, route template and Gateway/endpoint/MongoDB Definition examples at
  `5407998b0e08dc0da51c19c28517dfa191e7a3ee`; no main merge or tag was made.
- Design checkpoint `2b549bdd` was refined following the user's explicit
  decision: Gateway is a separate resource/Definition. Implementation checkpoint
  `1a33bed7`, security/runtime checks `d5293723`, full-check collector `dc9d8b49`.
  Cluster Definitions do not contain Gateway policy maps.

## Environment

Host default Docker socket and kubectl PATH were unavailable to this checkout.
Existing rootless Docker and an existing scoped kind connection were located;
short-lived task-owned containers on the existing network performed only reads.
No shared service was stopped or reconfigured. Private paths, credentials and
network addresses remain outside this public handoff.

## Remaining stages

Keep the browser-verified trial installation for acceptance. Real execution,
cleanup and completed full kind comparison are recorded in
[verification](../verification/2026-11-16-namespace-template-routes.md).
Preserve the known Score baseline limits in its own verification record. User
acceptance, main integration and any release tag are still outstanding.

## Implementation checkpoint

- Namespace provider references inherit the selected namespace's original cluster,
  account/version and provenance. Registration/planning reject namespace overrides;
  runtime verifies the exact namespace/cluster chain before invocation.
- Gateway is a separate `k8s-gateway` Type / `existing-gateway` Definition; cluster
  variables remain empty. The adapter only reads Gateway/listener readiness and
  allowed namespaces/hostname; Gateway ownership is PREEXISTING.
- Gateway-bound endpoints supply authorized neutral input to the explicit
  `endpoint-httproute-v1` template capability. The Git template constructs the
  route; guard checks actual Service/port, parent, host/path and ownership.
  Existing literal endpoint rendering remains the explicit legacy path.
- Console adds a Gateway form and Gateway-reference endpoint form; legacy policy
  forms remain editable. Namespace provider bindings round-trip through typed
  controls and JSON. Shared shell/styles/components are preserved.
- Focused backend placement/Gateway/route/legacy endpoint/pin tests pass. Backend
  `go build ./...` passes. Frontend focused authoring/type tests pass (94 tests),
  followed by the changed namespace mapping/endpoint checks (12 tests).
  Typecheck, lint and build pass; existing PlatformSettingsPage lint warning and
  frontend bundle-size warning remain. Docs/whitespace checks pass; the new
  sequence PNG was rendered with PlantUML 1.2025.2.
- An earlier broad focused pattern also selected the existing failing
  `TestDeployWithUnknownTerraformOutcomeHoldsTheEnvironment`. It fails identically
  on isolated main `cd40a792` (renderer-unavailable before the assertion); this is
  not a green test and is not removed or relaxed.
- A separate task Docker stack is running with candidate binaries/assets and
  fresh isolated PostgreSQL/Vault volumes. No retained installation was upgraded.
  Gateway and endpoint Definitions were created through their new Console forms.
  Native and Score workloads were saved before one Environment Deploy: both
  succeeded through Fleet with one shared namespace-bound PostgreSQL. Browser
  checks reached both real workloads through the existing Gateway over Tailscale.
  Batch update and a UID-protected native Pod restart passed; authenticated SQL,
  init clients, accepted routes and all preserved resource UIDs were verified.
  Console product cleanup succeeded: PostgreSQL then namespace destroyed;
  namespace/PV absence and unchanged Gateway UID were verified. A fresh review
  application redeployed both released endpoint names successfully with new
  namespace/database/PVC identities. Full kind backend comparison has the exact
  same 103 failing top-level tests (143 including subtests), 20 failing packages
  and 8 skips on candidate/main; builds pass, no additional failures. Corrected
  full frontend environment passes 1067 tests/typecheck/lint/build. Both test
  namespaces were removed with ownership/UID guards. Backend remains baseline-red.
- UI reference: signed-in real Score review Console, Resource Definitions list,
  AppShell, styles/tokens and Button. No unrelated UI was redesigned. The candidate
  forms retain the same shell/styles and short field labels. Final handoff must
  record the reviewed screens; setup/bootstrap is not app acceptance.

## Approved Echo refinement — 2026-11-17

User approved the new Gateway metadata/verification boundary and updating #84.
ADR-043/GW-01 and affected use cases are amended first. New Gateway forms use
Echo with a gateway output envelope; legacy edits preserve driver/layout/version.
A generic identity executor is wired, public registration remains Gateway-scoped,
and standalone GatewayVerifier runs before render/CD. Planner checks supported
contract and same-cluster pins; provisioning refuses params, secret-derived
metadata and stale/foreign account/target/descriptor before Echo execution. Echo
receives no Connection/Target and retains no physical instance/target. Logical
ActiveResource identity/account provenance survives explicit legacy upgrades.

Current main `ab37e2d4` was merged into this feature at `df719115` and pushed before
continuing verification. Focused registration/planner/runtime/legacy-form checks
pass; cluster API/auth/readiness and provider-pin tests are reused. No SQL/schema
change. Merged native/Score deploy/update/restart and Console product cleanup pass.
A fresh review app is retained with native SQL/routes, both executed READY Echo
Gateway resources and browser-verified Console/workload access. Legacy form
versions/values remain unchanged. Full merged frontend passes 1076 tests and
typecheck/lint/build. Fresh backend/main have identical 101 top-level failure
events (141 including subtests), 20 failed packages and 8 skips; both builds pass
and no main pass is missing. Both PostgreSQL packages time out at 30 minutes,
leaving the suite baseline-red. The owned full-check namespace was cleaned.
[2026-11-17 execution evidence](../verification/2026-11-17-echo-gateway.md) owns
these results; the 2026-11-16 record remains historical. User acceptance, feature
merge into main, retained-stack rollout and any template tag remain outstanding.

## Gateway reference controls — approved 2026-11-17

The user requested reuse of existing Definition authoring patterns. Gateway's
cluster and endpoint's Gateway references now use class dropdowns backed by
Resource Type metadata and the shared class rules. The form generates the
Environment-scoped output reference, displayed under technical details. New
forms require a class choice; legacy driver/layout, unknown classes and advanced
stored references are retained without implicit pin changes. 28 focused tests,
live legacy/Echo form review and fresh full frontend checks (1078 tests plus
typecheck/lint/build) pass; owned namespace cleanup is verified. Backend source
is unchanged and completed evidence is explicitly reused. See the
[reference control record](../verification/2026-11-17-gateway-reference-controls.md). The user
also requested synchronized YAML/JSON editing for all Definitions; that broader
editor extension is a separate proposed issue, not implemented by this slice.

## Combined acceptance with issue #87 — approved 2026-11-18

The user approved integrating features/definition-code-editor into
features/Httproute for one Console trial before main acceptance. Both canonical
contracts remain: Endpoint references a separate Gateway; templates create
HTTPRoute; the Definition form and YAML/JSON share one draft. Gateway Echo and
legacy hydration/reset now use the code editor bridge without replacing layout,
class controls or pins. Merge `35cbc638` is deployed to the task Console; its
recognized v10→v11 owner upgrade preserves installation/catalog/Application
identities. Focused integration checks, real editor round-trips for all existing
driver layouts, workload update, guarded restart and real Tailscale browser
checks pass. Full frontend passes 1102 tests/typecheck/lint/build. Fresh backend
candidate/main checks complete with identical 103 top-level failures and 8 skips,
no new failure/skip or missing main passing test. Both builds pass; disposable
namespace cleanup is verified. Backend remains baseline-red.
See [combined verification](../verification/2026-11-18-integrated-routes-editor.md).
No main merge or shared Gateway change approved.
