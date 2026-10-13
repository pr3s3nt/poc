---
id: UC-09-CONTEXT
artifact: use-case-context
status: current
last_reviewed: 2026-10-13
---

# UC-09 context — Observe deployment

## Trạng thái thực hiện

Xem [CURRENT_STATE](../../CURRENT_STATE.md) và
[IMP-021](../../implementation/deviations.md#imp-021-terraform-v2-designimplementation-gap).
Specification/realization định nghĩa yêu cầu, không chứng minh implementation.

## Read in this order

1. [Specification](specification.md)
2. [Realization](realization.md)
3. [Sequence](sequence.puml)
4. [VOPC](vopc.puml)
5. [UI/API mapping](ui/api-mapping.md)
6. [Active I06-04 iteration](../../iterations/M02-usecase-completion/I06-04-uc09-observability/README.md)
7. [Deployment state machine](../../architecture/state-machines/deployment.puml)
8. [Web Console screens](ui/screens.md) and [states](ui/states.md)

## Implementation entry points

- [`backend/internal/application/deployment`](../../../backend/internal/application/deployment/)
- [`backend/internal/delivery/http`](../../../backend/internal/delivery/http/)
- [`frontend/src/features/deployments`](../../../frontend/src/features/deployments/)
