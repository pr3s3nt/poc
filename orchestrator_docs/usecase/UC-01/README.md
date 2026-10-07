---
id: UC-01-CONTEXT
artifact: use-case-context
status: current
last_reviewed: 2026-10-07
---

# UC-01 context — Create Application

## Delivery state

Environment Settings set-once design accepted 2026-10-07, including independent
AWS targets/VPC/EKS and locked legacy migration ([ADR-011](../../architecture/decisions/ADR-011-environment-execution-binding.md)).
Code/API/UI binding and legacy migration are implemented; real kind staging
deployment and separate stored Environment bindings are [verified](../../verification/2026-10-07-environment-connection-kind.md).
Final review and validation passed. Previous Application selection
verification below is historical evidence, not current target requirement.

## Read in this order

1. [Specification](specification.md)
2. [Realization](realization.md)
3. [Sequence](sequence.puml)
4. [VOPC](vopc.puml)
5. [UI design](ui/README.md)
6. [Current state](../../CURRENT_STATE.md)

## Implementation entry points

- [`backend/internal/domain/application`](../../../backend/internal/domain/application/)
- [`backend/internal/domain/environment`](../../../backend/internal/domain/environment/)
- [`backend/internal/seed`](../../../backend/internal/seed/)
- [`backend/internal/adapters/store`](../../../backend/internal/adapters/store/)
- [`backend/internal/application/application`](../../../backend/internal/application/application/)
- [`frontend/src/features/applications`](../../../frontend/src/features/applications/)

## Historical Application connection selection delivery (2026-10-07)

API/UI selection, safe Developer choices, shared Environment binding, internal
and AWS target Definition guards, and local fake-adapter Playwright create/deploy/
restart verification are implemented. See [verification](../../verification/2026-10-07-application-connection-selection-local.md).

[Live kind recording](../../verification/2026-10-07-application-connection-selection-kind.md)
also verifies creation with an uploaded nondefault Connection and real staging
workload/database/Vault execution using UI interactions (2026-10-07).
