---
id: ADR-001
artifact: architecture-decision
status: current
last_reviewed: 2026-10-13
---

# ADR-001 — Execution-profile Resource Enrichment and Scopes

Status: Accepted — 2026-09-20.

## Quan hệ với các ADR sau

ADR-012 sở hữu Environment target selection/generation; ADR-013 sở hữu internal
cluster system binding. ADR này vẫn sở hữu enrichment, private/shared scopes
và profile guard trước weighted criteria. ADR-014 sở hữu module/executor/state
v2; ADR-001 không chọn renderer, account Aurora riêng hoặc tên chứa run ID.
Các logical descriptor và isolation rules dưới đây tiếp tục có hiệu lực.

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
