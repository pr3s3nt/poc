---
id: UC-09-CONTEXT
artifact: use-case-context
status: current
last_reviewed: 2026-09-23
---

# UC-09 context — Observe deployment

## Delivery state

Partially implemented; backend list/detail, graph, batches, resources,
workloads and redacted outputs are available. React Web Console now offers
recent deployments, Environment history with a status filter, and a persisted
detail view. Cross-deployment state comparison, live status and PostgreSQL read
model remain deferred.

## Read in this order

1. [Specification](specification.md)
2. [Realization](realization.md)
3. [Sequence](sequence.puml)
4. [VOPC](vopc.puml)
5. [Deferred I06-04 iteration](../../iterations/M02-usecase-completion/I06-04-uc09-observability/README.md)
6. [Deployment state machine](../../architecture/state-machines/deployment.puml)
7. [Web Console screens](ui/screens.md) and [states](ui/states.md)

## Implementation entry points

- [`backend/internal/application/deployment`](../../../backend/internal/application/deployment/)
- [`backend/internal/delivery/http`](../../../backend/internal/delivery/http/)
- [`frontend/src/features/deployments`](../../../frontend/src/features/deployments/)
