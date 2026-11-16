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
- The release template repository at `b9437b4bdbd4da696e946b964540ef5592a521c6`
  has Service templates but no public-domain/HTTPRoute template.
- Design checkpoint committed at `2b549bdd`. User refinement requires a separate
  Gateway resource/Definition; the uncommitted cluster-policy-map draft is being
  replaced. Namespace provider planning checks pass; runtime and acceptance
  remain outstanding. No task readiness or full-suite pass is claimed.

## Environment

Host default Docker socket and kubectl PATH were unavailable to this checkout.
Existing rootless Docker and an existing scoped kind connection were located;
short-lived task-owned containers on the existing network performed only reads.
No shared service was stopped or reconfigured. Private paths, credentials and
network addresses remain outside this public handoff.

## Remaining stages

Implement namespace provider inheritance and focused tests; separate Gateway resource
policy resolution; neutral endpoint input and template capability/guards;
release-repository template changes; Definition samples and traceability;
real Score/native acceptance and full kind checks; verified Tailscale handoff.
Preserve the known Score baseline limits in its own verification record.

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
  HTTP shell/health checks pass; application acceptance and full kind checks are
  still pending, as are release-template branch publication and verified handoff.
- UI reference: signed-in real Score review Console, Resource Definitions list,
  AppShell, styles/tokens and Button. No unrelated UI was redesigned. The candidate
  still needs final browser comparison; setup/bootstrap is not app acceptance.
