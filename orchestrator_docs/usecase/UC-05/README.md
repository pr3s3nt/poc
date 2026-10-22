---
id: UC-05-CONTEXT
artifact: use-case-context
status: current
last_reviewed: 2026-10-23
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
