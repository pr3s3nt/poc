---
id: UC-08-CONTEXT
artifact: use-case-context
status: current
last_reviewed: 2026-10-09
---

# UC-08 context — Provision resources

## Delivery state

Implemented and end-to-end verified for internal PostgreSQL/Kubernetes and AWS
VPC/EKS/Aurora resource nodes. Container CPU/memory requests belong to UC-06
workload rendering and are explicitly outside UC-08 resource-node execution.

## Definition-selected workload rendering

Workload rendering stays outside resource matching and provision batches.
Database/namespace identity, provisioning and credentials remain owned by UC-08;
score-k8s receives only resolved values and references in UC-06.

See [ADR-010](../../architecture/decisions/ADR-010-score-k8s-workload-rendering.md)
and the [rendering contract](../../architecture/contracts/workload-rendering.md).
Local verification does not claim live kind/AWS execution.

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

## Implicit internal cluster delivery (2026-10-09)

[ADR-013](../../architecture/decisions/ADR-013-implicit-existing-cluster.md)
removes per-cluster user Definition registration/matching for internal-k8s.
Default planning, system binding, persistence admission and Console projection
are implemented and [locally verified](../../verification/2026-10-09-implicit-existing-cluster-local.md).
Other resource/cloud matching remains unchanged; no new live cluster/cloud proof.
