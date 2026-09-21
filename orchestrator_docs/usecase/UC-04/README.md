---
id: UC-04-CONTEXT
artifact: use-case-context
status: current
last_reviewed: 2026-09-21
---

# UC-04 context — Configure execution connections

## Delivery state

Designed with partial execution support. Seeded internal/AWS connection data and
real adapters exist; registration, verification UI and durable persistence do
not.

## Read in this order

1. [Specification](specification.md)
2. [Realization](realization.md)
3. [Sequence](sequence.puml)
4. [VOPC](vopc.puml)
5. [Connection state machine](../../architecture/state-machines/connection.puml)

## Implementation entry points

- [`backend/internal/domain/application`](../../../backend/internal/domain/application/)
- [`backend/internal/adapters/kubernetes`](../../../backend/internal/adapters/kubernetes/)
- [`backend/internal/adapters/terraform`](../../../backend/internal/adapters/terraform/)
- [`backend/internal/bootstrap`](../../../backend/internal/bootstrap/)
- [`backend/internal/seed`](../../../backend/internal/seed/)
