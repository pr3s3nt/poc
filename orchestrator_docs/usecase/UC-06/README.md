---
id: UC-06-CONTEXT
artifact: use-case-context
status: current
last_reviewed: 2026-10-28
---

# UC-06 context — Deploy Workload

## Trạng thái thực hiện

Xem [CURRENT_STATE](../../CURRENT_STATE.md) và
[IMP-021](../../implementation/deviations.md#imp-021-terraform-v2-designimplementation-gap).
Specification/realization định nghĩa yêu cầu, không chứng minh implementation.

## Read in this order

1. [Specification](specification.md)
2. [Realization](realization.md)
3. [Shared sequence](sequence.puml)
4. [Cloud sequence](sequence-cloud.puml)
5. [Internal sequence](sequence-internal.puml)
6. [VOPC](vopc.puml)
7. [Planner reference](../../implementation/uc06-planner-reference.md)

## Implementation entry points

- [`backend/internal/application/deployment`](../../../backend/internal/application/deployment/)
- [`backend/internal/planning`](../../../backend/internal/planning/)
- [`backend/internal/delivery/http`](../../../backend/internal/delivery/http/)

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

## Optional CD and reconciliation — ADR-030

Read this use case's CD/INF/REC specification extension, then the
[design and implementation route](../../implementation/delivery-reconciliation.md).
Required design only; current runtime availability is tracked separately.

## Versioned catalog release selection — issue #28

[CR-01..06](../UC-03/catalog-releases.md) extends catalog resolution and
Environment upgrades. Read its [sequence](../UC-03/catalog-releases-sequence.puml)
and [shared contract](../../architecture/contracts/catalog-releases.md).
Existing ownership, locking, fencing, state identity and cleanup rules continue.
