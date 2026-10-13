---
id: UC-02-CONTEXT
artifact: use-case-context
status: current
last_reviewed: 2026-10-13
---

# UC-02 context — Register Resource Type

## Trạng thái thực hiện

Xem [CURRENT_STATE](../../CURRENT_STATE.md) và
[IMP-021](../../implementation/deviations.md#imp-021-terraform-v2-designimplementation-gap).
Specification/realization định nghĩa yêu cầu, không chứng minh implementation.

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
