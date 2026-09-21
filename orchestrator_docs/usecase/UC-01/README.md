---
id: UC-01-CONTEXT
artifact: use-case-context
status: current
last_reviewed: 2026-09-21
---

# UC-01 context — Manage Application and Environment

## Delivery state

Designed; executable baseline currently uses seeded Applications/Environments.
Full management API, persistence and Web Console flow are planned after UC-09.

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
