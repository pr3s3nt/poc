---
id: UC-04-CONTEXT
artifact: use-case-context
status: current
last_reviewed: 2026-10-28
---

# UC-04 context — Configure execution connections

Account diagnostics: [DIAG-01..14](specification.md#account-diagnostics--diag-0114),
[shared contract](../../architecture/contracts/account-diagnostics.md),
[delivery/acceptance](../../implementation/account-diagnostics.md),
[sequence](diagnostics-sequence.puml), [participants](diagnostics-vopc.puml).

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

## EC2 role authentication

UC-04 EC2-01..05 extend the account catalog with
[ADR-018](../../architecture/decisions/ADR-018-ec2-role-driver-accounts.md).
See [issue #6 verification](../../verification/2026-10-20-issue6-ec2-role.md) for
local checks, live EC2/Console/Terraform scope and native cleanup evidence.

## Optional CD and reconciliation — ADR-027

Read this use case's CD/INF/REC specification extension, then the
[design and implementation route](../../implementation/delivery-reconciliation.md).
Required design only; current runtime availability is tracked separately.

CD platform authoring: [Cấu hình CD](ui/cd-providers.md).
