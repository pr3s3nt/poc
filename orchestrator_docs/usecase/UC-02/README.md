---
id: UC-02-CONTEXT
artifact: use-case-context
status: current
last_reviewed: 2026-09-21
---

# UC-02 context — Register Resource Type

## Delivery state

Registration API/UI and normalized PostgreSQL persistence are implemented;
insert-only duplicate guards, scoped contracts and no-restart planner consumption
are locally verified. New identifier/reserved-key policy remains unresolved.

## Read in this order

1. [Specification](specification.md)
2. [Realization](realization.md)
3. [Sequence](sequence.puml)
4. [VOPC](vopc.puml)
5. [Resource domain](../../architecture/domain/domain-objects.md)

UI: [screens](ui/screens.md), [states](ui/states.md), [HTTP mapping](ui/api-mapping.md).

## Implementation entry points

- [`backend/internal/domain/resource`](../../../backend/internal/domain/resource/)
- [`backend/internal/adapters/store`](../../../backend/internal/adapters/store/)
- [`backend/internal/seed`](../../../backend/internal/seed/)
