---
id: UC-08-CONTEXT
artifact: use-case-context
status: current
last_reviewed: 2026-09-21
---

# UC-08 context — Provision resources

## Delivery state

Implemented and end-to-end verified for internal PostgreSQL/Kubernetes and AWS
VPC/EKS/Aurora resource nodes.

## Read in this order

1. [Specification](specification.md)
2. [Realization](realization.md)
3. [Sequence](sequence.puml)
4. [VOPC](vopc.puml)
5. [Operation contracts](../../architecture/contracts/operation-contracts.md)

## Implementation entry points

- [`backend/internal/application/provisioning`](../../../backend/internal/application/provisioning/)
- [`backend/internal/adapters/kubernetes`](../../../backend/internal/adapters/kubernetes/)
- [`backend/internal/adapters/terraform`](../../../backend/internal/adapters/terraform/)
- [`backend/internal/ports/execution`](../../../backend/internal/ports/execution/)
