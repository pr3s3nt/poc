---
id: UC-06-CONTEXT
artifact: use-case-context
status: current
last_reviewed: 2026-10-09
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

## Definition-selected workload rendering

Definition-selected score-k8s rendering runs after UC-08 resolves outputs.
The adapter normalizes the product Score subset, bridges Secret references and
validates protected fields before existing delivery/readiness/current-set commit.
The built-in renderer remains the default when no renderer Definition matches.

See [ADR-010](../../architecture/decisions/ADR-010-score-k8s-workload-rendering.md)
and the [rendering contract](../../architecture/contracts/workload-rendering.md).
Local verification does not claim live kind/AWS execution.

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

## Implicit internal cluster delivery (2026-10-09)

[ADR-013](../../architecture/decisions/ADR-013-implicit-existing-cluster.md)
removes per-cluster user Definition registration/matching for internal-k8s.
Default planning, system binding, persistence admission and Console projection
are implemented and [locally verified](../../verification/2026-10-09-implicit-existing-cluster-local.md).
Other resource/cloud matching remains unchanged; no new live cluster/cloud proof.
