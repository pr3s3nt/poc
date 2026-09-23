---
id: UC-06-CONTEXT
artifact: use-case-context
status: current
last_reviewed: 2026-09-23
---

# UC-06 context — Deploy Workload

## Delivery state

Implemented and end-to-end verified through HTTP on kind and AWS. Cloud rerun is
still a release gate after the last planning changes. Each Deployment persists
an immutable Humanitec-shaped `DeploymentDeltaSnapshot` (I06-06). Score
container requests/limits reach the Kubernetes Deployment with the BR-11
default policy (I06-07).

## Read in this order

1. [Specification](specification.md)
2. [Realization](realization.md)
3. [Shared sequence](sequence.puml)
4. [Cloud sequence](sequence-cloud.puml)
5. [Internal sequence](sequence-internal.puml)
6. [VOPC](vopc.puml)
7. [Planner reference](../../implementation/uc06-planner-reference.md)

## Implementation entry points

- [`backend/internal/application/deployment`](../../../backend/internal/application/deployment/)
- [`backend/internal/planning`](../../../backend/internal/planning/)
- [`backend/internal/delivery/http`](../../../backend/internal/delivery/http/)

The Web Console deploy feature is intentionally not present in M00-a. Its UI
will be introduced under `frontend/src/features/deployments/` when UC-06 is
scheduled for console delivery.
