---
id: UC-06-CONTEXT
artifact: use-case-context
status: current
last_reviewed: 2026-09-28
---

# UC-06 context — Deploy Workload

## Delivery state

Implemented and end-to-end verified through HTTP on kind and AWS. Cloud rerun is
still a release gate after the last planning changes. Each Deployment persists
an immutable Humanitec-shaped `DeploymentDeltaSnapshot` (I06-06). Score
container requests/limits reach the Kubernetes Deployment with the BR-11
default policy (I06-07).
Optional `fleet-gitrepo` workload delivery for internal kind was verified with
a Harbor image; UC-08 resource provisioning remains direct.
Internal kind now supports an optional Environment public entry via Traefik
Ingress after workload readiness; DNS/TLS and controller exposure are separate.

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

The Web Console Application home now exposes the UC-12/16 pending-change
Preview → Deploy flow. Broader UC-06 deployment-details UI is still deferred.
