---
id: UC-01-CONTEXT
artifact: use-case-context
status: current
last_reviewed: 2026-09-23
---

# UC-01 context — Create Application

## Delivery state

Designed; executable baseline currently uses seeded Applications/Environments.
The self-service Developer flow, fixed `staging`/`production` Environments and
desired endpoints are planned in I06-10 after UC-09.

## Read in this order

1. [Specification](specification.md)
2. [Realization](realization.md)
3. [Sequence](sequence.puml)
4. [VOPC](vopc.puml)
5. [Current state](../../CURRENT_STATE.md)

## Implementation entry points

- [`backend/internal/domain/application`](../../../backend/internal/domain/application/)
- [`backend/internal/domain/environment`](../../../backend/internal/domain/environment/)
- [`backend/internal/seed`](../../../backend/internal/seed/)
- [`backend/internal/adapters/store`](../../../backend/internal/adapters/store/)
