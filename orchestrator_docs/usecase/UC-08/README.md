---
id: UC-08-CONTEXT
artifact: use-case-context
status: current
last_reviewed: 2026-10-28
---

# UC-08 context — Provision resources

## Trạng thái thực hiện

Xem [CURRENT_STATE](../../CURRENT_STATE.md) và
[IMP-021](../../implementation/deviations.md#imp-021-terraform-v2-designimplementation-gap).
Specification/realization định nghĩa yêu cầu, không chứng minh implementation.

## Read in this order

1. [Specification](specification.md)
2. [Realization](realization.md)
3. [Sequence](sequence.puml)
4. [VOPC](vopc.puml)
5. [Operation contracts](../../architecture/contracts/operation-contracts.md)

## Implementation entry points

- [`backend/internal/application/provisioning`](../../../backend/internal/application/provisioning/)
- [`backend/internal/adapters/kubernetes`](../../../backend/internal/adapters/kubernetes/)
- [`backend/internal/adapters/terraform`](../../../backend/internal/adapters/terraform/)
- [`backend/internal/ports/execution`](../../../backend/internal/ports/execution/)

## V2 cleanup collaboration

Separate cleanup [sequence](sequence-cleanup.puml) and
[state contract](../../architecture/contracts/terraform-state.md) define owned
reverse-dependency destroy/verification. Workload removal does not call cleanup.

## Workload catalog — issue #24 (design review)

[Specification extension](../UC-16/workload-catalog-specification.md),
[shared contract](../../architecture/contracts/workload-catalog.md) và
[implementation handoff](../../implementation/workload-catalog.md) mô tả phần
mở rộng đang review; chưa thay thế current behavior hoặc chứng minh implementation.

## Optional CD and reconciliation — ADR-030

Read this use case's CD/INF/REC specification extension, then the
[design and implementation route](../../implementation/delivery-reconciliation.md).
Required design only; current runtime availability is tracked separately.
