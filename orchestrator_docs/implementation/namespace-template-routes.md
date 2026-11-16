---
id: IMP-NAMESPACE-TEMPLATE-ROUTES
artifact: implementation-handoff
status: current
last_reviewed: 2026-11-16
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
