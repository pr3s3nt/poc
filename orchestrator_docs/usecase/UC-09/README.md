---
id: UC-09-CONTEXT
artifact: use-case-context
status: current
last_reviewed: 2026-09-22
---

# UC-09 context — Observe deployment

## Delivery state

Partially implemented; M01 contract hardening is complete and completion is the
active M02/I06-04 iteration.
List/detail, graph, batches, resources, workloads and redacted outputs exist;
history/filter/state comparison remain.

## Read in this order

1. [Specification](specification.md)
2. [Realization](realization.md)
3. [Sequence](sequence.puml)
4. [VOPC](vopc.puml)
5. [Deferred I06-04 iteration](../../iterations/M02-usecase-completion/I06-04-uc09-observability/README.md)
6. [Deployment state machine](../../architecture/state-machines/deployment.puml)

## Implementation entry points

- [`backend/internal/application/deployment`](../../../backend/internal/application/deployment/)
- [`backend/internal/delivery/http`](../../../backend/internal/delivery/http/)
- [`frontend/src/features/deployment-details`](../../../frontend/src/features/deployment-details/)
- [`frontend/src/features/deploy`](../../../frontend/src/features/deploy/)
