---
id: UC-07-CONTEXT
artifact: use-case-context
status: current
last_reviewed: 2026-10-24
---

# UC-07 context — Update or Remove Workload

## Trạng thái thực hiện

Xem [CURRENT_STATE](../../CURRENT_STATE.md) và
[IMP-021](../../implementation/deviations.md#imp-021-terraform-v2-designimplementation-gap).
Specification/realization định nghĩa yêu cầu, không chứng minh implementation.

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

Luồng Xóa tài nguyên: [sequence](sequence-resource-delete.puml),
[screens](ui/screens.md), [states](ui/states.md), [API](ui/api-mapping.md).

## Git-owned manifest rendering

[ADR-021](../../architecture/decisions/ADR-021-git-render-bundles.md),
[engine/source contract](../../architecture/contracts/git-render-bundles.md),
[implementation handoff](../../implementation/git-render-bundles.md) and
[real acceptance](../../operations/git-render-bundles-acceptance.md).
Behavior is recorded in this use case's RB requirements;
[IMP-035](../../implementation/deviations.md#imp-035--git-owned-render-bundles)
records implementation and actual acceptance separately.

## Workload catalog — issue #24 (design review)

[Specification extension](../UC-16/workload-catalog-specification.md),
[shared contract](../../architecture/contracts/workload-catalog.md) và
[implementation handoff](../../implementation/workload-catalog.md) mô tả phần
mở rộng đang review; chưa thay thế current behavior hoặc chứng minh implementation.
