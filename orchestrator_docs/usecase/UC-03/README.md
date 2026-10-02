---
id: UC-03-CONTEXT
artifact: use-case-context
status: current
last_reviewed: 2026-10-02
---

# UC-03 context — Register Resource Definition

## Delivery state

Runtime-supported Definition registration API/UI and normalized PostgreSQL
persistence are implemented and locally verified, including atomic criteria,
duplicate guards and no-restart matching. Driver Inputs policy in specification BR-11–BR-14 is implemented and locally
verified on 2026-10-02; registration does not add new runtime drivers.

## Read in this order

1. [Specification](specification.md)
2. [Realization](realization.md)
3. [Sequence](sequence.puml)
4. [VOPC](vopc.puml)
5. [Planner reference](../../implementation/uc06-planner-reference.md)
6. [Humanitec/Score compatibility matrix](../../implementation/humanitec-compatibility.md)

UI: [screens](ui/screens.md), [states](ui/states.md), [HTTP mapping](ui/api-mapping.md).

## Implementation entry points

- [`backend/internal/domain/resource`](../../../backend/internal/domain/resource/)
- [`backend/internal/planning`](../../../backend/internal/planning/)
- [`backend/internal/seed`](../../../backend/internal/seed/)
