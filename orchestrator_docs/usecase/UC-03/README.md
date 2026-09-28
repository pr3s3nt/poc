---
id: UC-03-CONTEXT
artifact: use-case-context
status: current
last_reviewed: 2026-09-22
---

# UC-03 context — Register Resource Definition

## Delivery state

Designed; seeded Definitions, profile eligibility, five-field matching and driver contract validation
run in the planner. Management API/UI and durable catalog persistence remain.

## Read in this order

1. [Specification](specification.md)
2. [Realization](realization.md)
3. [Sequence](sequence.puml)
4. [VOPC](vopc.puml)
5. [Planner reference](../../implementation/uc06-planner-reference.md)
6. [Humanitec/Score compatibility matrix](../../implementation/humanitec-compatibility.md)

## Implementation entry points

- [`backend/internal/domain/resource`](../../../backend/internal/domain/resource/)
- [`backend/internal/planning`](../../../backend/internal/planning/)
- [`backend/internal/seed`](../../../backend/internal/seed/)
