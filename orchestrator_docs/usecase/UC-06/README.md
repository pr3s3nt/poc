---
id: UC-06-CONTEXT
artifact: use-case-context
status: current
last_reviewed: 2026-10-24
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
Required design is recorded in this use case's RB requirements; IMP-034 tracks
unimplemented work separately.
