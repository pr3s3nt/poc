---
id: DOMAIN-OBJECTS
artifact: domain-persistence-classification
status: current
last_reviewed: 2026-09-21
---

# Domain Objects and Persistence Classification

## Aggregates

| Aggregate | Root | Owned objects | Invariant chính |
|---|---|---|---|
| Application | `Application` | Execution Profile binding | Profile cố định khi đã có Active Resource; AWS scope sở hữu tối đa một VPC/EKS descriptor. |
| Environment | `Environment` | namespace identity, current-set pointer | Thuộc một Application; current Deployment Set chỉ đổi trong final deployment transaction. |
| Resource Type | `ResourceType` | input/output schema | Contract độc lập implementation. |
| Resource Definition | `ResourceDefinition` | Matching Criteria, driver inputs, provision rules | Cùng Resource Type; criteria match deterministic; output contract tương thích. |
| Connection | `Connection` | verification metadata | Chỉ secret reference được persist; chỉ `READY` được sử dụng. |
| Deployment | `Deployment` | `DeploymentPlan`, deployment-resource progress | Plan immutable sau khi provisioning bắt đầu; `SUCCEEDED` chỉ sau workload readiness. |
| Active Resource | `ActiveResource` | executor state, outputs | Logical identity unique theo Organization + descriptor + scope. |
| Workload Instance | `WorkloadInstance` | manifest digest/status | Unique theo Environment + workload ID. |

## Entities and value objects

| Kind | Objects | Persisted form |
|---|---|---|
| Entity | `Application`, `Environment`, `Connection`, `ResourceType`, `ResourceDefinition`, `DeploymentSet`, `Deployment`, `DeploymentPlan`, `ActiveResource`, `WorkloadInstance` | Dedicated tables. |
| Child entity | `MatchingCriterion`, `DeploymentResource` | Dedicated child/join tables. |
| Value object | `ExecutionProfile`, `NamespaceIdentity`, `ResourceDescriptor`, `ResourceScope`, `DeploymentStatus`, `ResourceStatus`, `DriverType`, `OutputBinding`, `DeploymentTarget` | Scalar/JSONB columns; validated at construction. |
| Planning document | `WorkloadFragment`, `DeploymentDelta`, `CandidateDeploymentSet`, `ResourceGraph`, `ProvisionBatch` | Immutable JSONB snapshot in `deployment_plans`; typed Go model in memory. |
| Result | `ProvisionResult`, `DeploymentResult`, `DeploymentPreview`, `DeploymentView` | DTO/read model, not aggregate roots. |

## Domain services

- `PlanningService`: orchestrates pure/deterministic planning components.
- `DefinitionMatcher`: weighted criteria selection.
- `ImplicitResourceEnricher`: adds profile-scoped infrastructure.
- `ResourceGraphBuilder`/`GraphExpander`: build fixed-point DAG.
- `BatchScheduler`: provider-first Kahn batches.
- `ResourceProvisioningService`: application service executing resource batches.
- `OutputBindingResolver`: resolves context and completed provider outputs.

## Persistence vs integration state

- Database owns logical identity, lifecycle, plan snapshot, output metadata and current-set pointer.
- Terraform backend owns Terraform physical state; database keeps opaque state reference/fingerprint, not the state file.
- Kubernetes owns live objects; database keeps target, workload identity, manifest digest and observed status.
- Secret Store owns secret values; database keeps opaque secret references and redacted output metadata.

## Cross-aggregate consistency

- CRUD operations use one local transaction per aggregate.
- Deployment creation persists Deployment + Candidate Set + Plan before external execution.
- Each resource result is persisted after its external call in a short transaction.
- Final deployment transaction performs optimistic Environment version check, updates current-set pointer, workload instances and Deployment `SUCCEEDED` atomically.
