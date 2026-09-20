# ADR-001 — Execution-profile Resource Enrichment and Scopes

Status: Accepted — 2026-09-20.

## Decision

Planning uses one shared core. `ImplicitResourceEnricher` adds infrastructure from Execution Profile:

- `aws-eks`: VPC and EKS with Application scope; namespace with Environment scope.
- `internal-k8s`: reference to registered existing cluster; namespace with Environment scope.
- Score resources retain private/shared identity; `postgres` matches profile-specific Definition.

Provider choice is expressed through context + Resource Definition + executor registry, not provider-specific branches scattered through DeploymentService.

## Consequences

- VPC/EKS are reused across Environments of one Application.
- Aurora and StatefulSet can expose one Resource Type output contract.
- Descriptor/scope uniqueness must be enforced in database and executor state.
