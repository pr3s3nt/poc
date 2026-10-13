---
id: UC-04-CONTEXT
artifact: use-case-context
status: current
last_reviewed: 2026-10-13
---

# UC-04 context — Configure execution connections

## Trạng thái thực hiện

Xem [CURRENT_STATE](../../CURRENT_STATE.md) và
[IMP-021](../../implementation/deviations.md#imp-021-terraform-v2-designimplementation-gap).
Specification/realization định nghĩa yêu cầu, không chứng minh implementation.

## Read in this order

1. [Specification](specification.md)
2. [Realization](realization.md)
3. [Sequence](sequence.puml)
4. [VOPC](vopc.puml)
5. [Connection state machine](../../architecture/state-machines/connection.puml)

UI: [screens](ui/screens.md), [states](ui/states.md), [HTTP mapping](ui/api-mapping.md).

## Implementation entry points

- [`shared credential design`](../../architecture/connection-credentials.md)
- [`backend/internal/application/connection`](../../../backend/internal/application/connection/)
- [`backend/internal/ports/credentials`](../../../backend/internal/ports/credentials/)
- [`backend/internal/adapters/vault/connection_credentials.go`](../../../backend/internal/adapters/vault/connection_credentials.go)
- [`backend/internal/domain/application`](../../../backend/internal/domain/application/)
- [`backend/internal/adapters/kubernetes`](../../../backend/internal/adapters/kubernetes/)
- [`backend/internal/adapters/terraform`](../../../backend/internal/adapters/terraform/)
- [`backend/internal/bootstrap`](../../../backend/internal/bootstrap/)
- [`backend/internal/seed`](../../../backend/internal/seed/)
