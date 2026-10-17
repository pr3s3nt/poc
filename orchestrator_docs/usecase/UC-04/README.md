---
id: UC-04-CONTEXT
artifact: use-case-context
status: current
last_reviewed: 2026-10-17
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

## V01 account catalog

[AC-01..12](specification.md#v01--danh-mục-tài-khoản-hạ-tầng-ac-0112),
[ADR-015](../../architecture/decisions/ADR-015-infrastructure-driver-accounts.md)
and [V01 handoff](../../verification/2026-10-17-v01-handoff.md) define the new
account catalog and its staged replacement boundary. Account catalog delivery
does not imply V02–V06 driver/planner changes are implemented.
