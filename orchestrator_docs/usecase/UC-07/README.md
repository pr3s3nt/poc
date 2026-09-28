---
id: UC-07-CONTEXT
artifact: use-case-context
status: current
last_reviewed: 2026-09-28
---

# UC-07 context — Update or Remove Workload

## Delivery state

Partially implemented. Planner covers before/shared validation, conflicts and
last-reference preservation. UC-16 Preview → Deploy update/remove is wired;
the optional Fleet GitRepo remove path has kind verification. Broader
deployment lifecycle UI/recovery remains deferred. The planner builds the
Humanitec-shaped `modules.add/remove/update` and `shared` Delta.

## Read in this order

1. [Specification](specification.md)
2. [Realization](realization.md)
3. [Sequence](sequence.puml)
4. [VOPC](vopc.puml)
5. [Known deviations](../../implementation/deviations.md)

## Implementation entry points

- [`backend/internal/application/deployment`](../../../backend/internal/application/deployment/)
- [`backend/internal/planning`](../../../backend/internal/planning/)
- [`backend/test/conformance`](../../../backend/test/conformance/)
