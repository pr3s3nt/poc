---
id: UC-03-CONTEXT
artifact: use-case-context
status: current
last_reviewed: 2026-10-13
---

# UC-03 context — Register Resource Definition

## Trạng thái thực hiện

Xem [CURRENT_STATE](../../CURRENT_STATE.md) và
[IMP-021](../../implementation/deviations.md#imp-021-terraform-v2-designimplementation-gap).
Specification/realization định nghĩa yêu cầu, không chứng minh implementation.

## Read in this order

1. [Specification](specification.md)
2. [Realization](realization.md)
3. [Sequence](sequence.puml)
4. [VOPC](vopc.puml)
5. [Planner reference](../../implementation/uc06-planner-reference.md)
6. [Humanitec/Score compatibility matrix](../../implementation/humanitec-compatibility.md)

UI: [screens](ui/screens.md), [states](ui/states.md), [HTTP mapping](ui/api-mapping.md).
Bản mẫu review (draft, issue #8): [Resource Definition v3](ui/v3-prototype.md);
mock module forms đã duyệt (issue #11): [module forms](ui/module-forms-prototype.md).

## Implementation entry points

- [`backend/internal/domain/resource`](../../../backend/internal/domain/resource/)
- [`backend/internal/planning`](../../../backend/internal/planning/)
- [`backend/internal/seed`](../../../backend/internal/seed/)
