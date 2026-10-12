---
id: UC-07-CONTEXT
artifact: use-case-context
status: current
last_reviewed: 2026-10-13
---

# UC-07 context — Update or Remove Workload

## V2 design scope

[ADR-014](../../architecture/decisions/ADR-014-terraform-v2-lifecycle.md) and the
Terraform v2 sections in this use-case specification/realization define the
required next implementation. They supersede conflicting baseline driver/account/
renderer/state constraints. Current delivered behavior remains separately tracked
in CURRENT_STATE/deviations; old evidence is not v2 acceptance proof.

## Delivery state

Implemented through UC-16 pending Preview → Deploy with local API/UI and
PostgreSQL tests. Planner covers before/shared conflicts, preservation of other
workloads and final Service-reference validation. Environment-owned READY
resources become UNREFERENCED in the successful current-set transaction, never
through implicit destroy. Console covers confirmation/Undo, busy/stale states
and deployment-history navigation. Existing Fleet remove kind evidence remains
historical; this milestone does not reverify a live cluster. Automatic rollback,
destroy, concurrent deployment recovery and Application-wide resource cleanup
remain outside scope.

## Read in this order

1. [Specification](specification.md)
2. [Realization](realization.md)
3. [Sequence](sequence.puml)
4. [VOPC](vopc.puml)
5. [Known deviations](../../implementation/deviations.md)
6. [UI screens](ui/screens.md), [states](ui/states.md), [API mapping](ui/api-mapping.md)

## Implementation entry points

- [`backend/internal/application/deployment`](../../../backend/internal/application/deployment/)
- [`backend/internal/planning`](../../../backend/internal/planning/)
- [`backend/test/conformance`](../../../backend/test/conformance/)
