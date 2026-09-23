---
id: UC-09-CONTEXT
artifact: use-case-context
status: current
last_reviewed: 2026-09-23
---

# UC-09 context — Observe deployment

## Delivery state

Partially implemented; M01 contract hardening is complete. Completion remains
in M02/I06-04, deferred until M00-a developer onboarding is complete.
The backend exposes list/detail, graph, batches, resources, workloads and
redacted outputs; history/filter/state comparison remain. The prior Web Console
views were retired for the M00-a rebuild and will return under
`frontend/src/features/deployments/` when UC-09 is scheduled.

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
