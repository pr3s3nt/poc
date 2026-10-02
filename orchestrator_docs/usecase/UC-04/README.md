---
id: UC-04-CONTEXT
artifact: use-case-context
status: current
last_reviewed: 2026-10-02
---

# UC-04 context — Configure execution connections

## Delivery state

Local/kind Kubernetes registration, verification API/UI and seeded internal/AWS
connection data exist with normalized PostgreSQL persistence and insert-only
registration. Local tests use an explicit test verifier; the latest UI recording
does not demonstrate successful live cluster verification. AWS registration and
durable credential storage remain unimplemented; access key + Vault storage
has been accepted in [ADR-009](../../architecture/decisions/ADR-009-aws-access-key-storage.md).

## Read in this order

1. [Specification](specification.md)
2. [Realization](realization.md)
3. [Sequence](sequence.puml)
4. [VOPC](vopc.puml)
5. [Connection state machine](../../architecture/state-machines/connection.puml)

UI: [screens](ui/screens.md), [states](ui/states.md), [HTTP mapping](ui/api-mapping.md).

## Implementation entry points

- [`backend/internal/domain/application`](../../../backend/internal/domain/application/)
- [`backend/internal/adapters/kubernetes`](../../../backend/internal/adapters/kubernetes/)
- [`backend/internal/adapters/terraform`](../../../backend/internal/adapters/terraform/)
- [`backend/internal/bootstrap`](../../../backend/internal/bootstrap/)
- [`backend/internal/seed`](../../../backend/internal/seed/)
