---
id: UC-04-CONTEXT
artifact: use-case-context
status: current
last_reviewed: 2026-09-21
---

# UC-04 context — Configure execution connections

## Delivery state

Local/kind Kubernetes registration, verification API/UI and seeded internal/AWS
connection data exist. AWS registration, durable credential store and PostgreSQL
persistence remain.

## Read in this order

1. [Specification](specification.md)
2. [Realization](realization.md)
3. [Sequence](sequence.puml)
4. [VOPC](vopc.puml)
5. [Connection state machine](../../architecture/state-machines/connection.puml)

UI: [screens](ui/screens.md), [states](ui/states.md).

## Implementation entry points

- [`backend/internal/domain/application`](../../../backend/internal/domain/application/)
- [`backend/internal/adapters/kubernetes`](../../../backend/internal/adapters/kubernetes/)
- [`backend/internal/adapters/terraform`](../../../backend/internal/adapters/terraform/)
- [`backend/internal/bootstrap`](../../../backend/internal/bootstrap/)
- [`backend/internal/seed`](../../../backend/internal/seed/)
