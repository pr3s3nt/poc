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
- Design checkpoint only: implementation, focused checks and live acceptance
  remain outstanding. No task readiness or full-suite pass is claimed.

## Environment

Host default Docker socket and kubectl PATH were unavailable to this checkout.
Existing rootless Docker and an existing scoped kind connection were located;
short-lived task-owned containers on the existing network performed only reads.
No shared service was stopped or reconfigured. Private paths, credentials and
network addresses remain outside this public handoff.

## Remaining stages

Implement namespace provider inheritance and focused tests; cluster Gateway
policy resolution; neutral endpoint input and template capability/guards;
release-repository template changes; Definition samples and traceability;
real Score/native acceptance and full kind checks; verified Tailscale handoff.
Preserve the known Score baseline limits in its own verification record.
