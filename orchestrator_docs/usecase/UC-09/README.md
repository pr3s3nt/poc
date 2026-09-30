---
id: UC-09-CONTEXT
artifact: use-case-context
status: current
last_reviewed: 2026-09-30
---

# UC-09 context — Observe deployment

## Delivery state

Implemented and locally browser-verified. Authenticated list/detail APIs enforce
Organization/Application/Environment scope; history supports server-side status
filtering and detail reads consistent per-Deployment snapshots. Resource inputs
are omitted and output visibility fails closed. React loading/error/not-found
states and JSON restart persistence are covered by executable tests.
Cross-deployment comparison and live runtime refresh remain out of scope.

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
