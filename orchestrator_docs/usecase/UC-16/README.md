---
id: UC-16-CONTEXT
artifact: use-case-context
status: current
last_reviewed: 2026-10-13
---

# UC-16 context — Manage workload configuration

## Trạng thái thực hiện

Xem [CURRENT_STATE](../../CURRENT_STATE.md) và
[IMP-021](../../implementation/deviations.md#imp-021-terraform-v2-designimplementation-gap).
Specification/realization định nghĩa yêu cầu, không chứng minh implementation.

## Read in this order

1. [Specification](specification.md)
2. [UI design](ui/README.md)
3. [UC-01 Application home](../UC-01/ui/README.md)
4. [UC-05 preview](../UC-05/README.md)
5. [Realization](realization.md), [sequence](sequence.puml) and [VOPC](vopc.puml)
6. [UC-12 Application variables and secrets](../UC-12/README.md)
7. [UC-06 deployment](../UC-06/README.md) and [UC-07 update/removal](../UC-07/README.md)

UC-12 owns Application variables/secrets; saving a draft never changes the
running workload until an explicit Preview → Deploy.
