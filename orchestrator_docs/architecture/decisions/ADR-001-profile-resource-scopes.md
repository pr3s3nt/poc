---
id: ADR-001
artifact: architecture-decision
status: current
last_reviewed: 2026-10-13
---

# ADR-001 — Execution-profile Resource Enrichment and Scopes

Status: Accepted — 2026-09-20.

## V2 supersession — ADR-014

[ADR-014](ADR-014-terraform-v2-lifecycle.md) replaces, for v2, embedded-only/
Kubernetes application provisioning, run-derived temporary state/names, explicit
Aurora account override and optional/internal-only/native-default rendering.
V2 uses registered Terraform modules, Environment account/region, encrypted
PostgreSQL state and required score-k8s on both profiles. Historical naming/
verification below remains provenance of the implemented baseline; stable v2
resource identity has no run ID. Other descriptor/matching/generation/credential
constraints remain unless explicitly superseded. V1 import is not required.

## Decision

Planning uses one shared core. `ImplicitResourceEnricher` adds infrastructure from Execution Profile:

- `aws-eks`: VPC/EKS with Environment scope only per [ADR-011](ADR-011-environment-execution-binding.md) (T27); namespace Environment-scoped.
- `internal-k8s`: implicit existing-cluster node bound directly to the Environment Connection, without user Definition matching ([ADR-013](ADR-013-implicit-existing-cluster.md)); namespace with Environment scope.
- Score resources retain private/shared identity; `postgres` matches profile-specific Definition.

Provider choice is expressed through context + Resource Definition + executor registry, not provider-specific branches scattered through DeploymentService.

For the seeded catalog, each profile-specific Definition declares an optional
`execution_profile` guard. The planner filters by that guard before applying
the existing five-field criteria and specificity weights. A missing guard
remains profile-neutral for shared Definitions and conformance fixtures;
`env_type` continues to mean Environment type, not cluster/cloud target.

## Consequences

- VPC/EKS are isolated per Environment and target generation; retired Application-scope bindings are rejected (T27).
- Aurora and StatefulSet can expose one Resource Type output contract.
- Descriptor/scope uniqueness must be enforced in database and executor state.
