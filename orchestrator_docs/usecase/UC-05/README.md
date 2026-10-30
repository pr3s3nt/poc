---
id: UC-05-CONTEXT
artifact: use-case-context
status: current
last_reviewed: 2026-10-28
---

# UC-05 context — Validate and preview Score changes

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
6. [UI screens](ui/screens.md), [states](ui/states.md) and [API mapping](ui/api-mapping.md)

## Implementation entry points

- [`backend/internal/planning`](../../../backend/internal/planning/)
- [`backend/internal/application/preview`](../../../backend/internal/application/preview/)
- [`shared snapshot loader`](../../../backend/internal/application/deployment/snapshot.go)
- [`HTTP boundary`](../../../backend/internal/delivery/http/score_preview.go)
- [`Web Console`](../../../frontend/src/features/preview/)
- [`backend/test/conformance`](../../../backend/test/conformance/)

## Visual review extension — 2026-10-23

[ADR-019](../../architecture/decisions/ADR-019-visual-deployment-review.md),
[shared contract](../../architecture/contracts/deployment-review.md) and
[implementation handoff](../../implementation/visual-deployment-review.md)
define the accepted extension. Specification/realization and diagrams here own
behavior/collaboration; prototype acceptance is not product implementation proof.

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
