---
id: UC-01-CONTEXT
artifact: use-case-context
status: current
last_reviewed: 2026-09-23
---

# UC-01 context — Create Application

## Delivery state

Implemented for local/test: authenticated Developer can list, create and read
Applications. Creation resolves the platform-owned `internal-cluster` default
for `acme`, then atomically creates `staging`/`production` and empty sets;
desired endpoints remain unprovisioned until UC-06.

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
