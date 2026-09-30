---
id: UC-07-CONTEXT
artifact: use-case-context
status: current
last_reviewed: 2026-09-30
---

# UC-07 context — Update or Remove Workload

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
