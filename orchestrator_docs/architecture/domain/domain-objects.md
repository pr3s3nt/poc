---
id: DOMAIN-OBJECTS
artifact: domain-persistence-classification
status: current
last_reviewed: 2026-09-22
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
| Deployment | `Deployment` | `DeploymentDeltaSnapshot`, `DeploymentPlan`, deployment-resource progress | Delta Snapshot immutable ngay khi persist; Plan immutable khi Deployment rời `PLANNING`; `SUCCEEDED` chỉ sau workload readiness. |
| Active Resource | `ActiveResource` | executor state, outputs | Logical identity unique theo Organization + descriptor + scope. |
| Workload Instance | `WorkloadInstance` | manifest digest/status | Unique theo Environment + workload ID. |

## Entities and value objects

| Kind | Objects | Persisted form |
|---|---|---|
| Entity | `Application`, `Environment`, `Connection`, `ResourceType`, `ResourceDefinition`, `DeploymentSet`, `DeploymentDeltaSnapshot`, `Deployment`, `DeploymentPlan`, `ActiveResource`, `WorkloadInstance` | Dedicated tables. |
| Child entity | `MatchingCriterion`, `DeploymentResource` | Dedicated child/join tables. |
| Value object | `ExecutionProfile`, `NamespaceIdentity`, `ResourceDescriptor`, `ResourceScope`, `DeploymentStatus`, `ResourceStatus`, `DriverType`, `OutputBinding`, `DeploymentTarget`, `ModuleDelta`, `JSONPatchOperation`, `ContainerResourceRequirements`, `ComputeResources` | Scalar/JSONB columns; validated at construction. |
| Planning document | `WorkloadFragment`, `CandidateDeploymentSet`, `ResourceGraph`, `ProvisionBatch` | Immutable JSONB snapshot in `deployment_plans`; typed Go model in memory. |
| Result | `ProvisionResult`, `DeploymentResult`, `DeploymentPreview`, `DeploymentView` | DTO/read model, not aggregate roots. |

## Domain services

- `PlanningService`: orchestrates pure/deterministic planning components.
- `DefinitionMatcher`: weighted criteria selection.
- `ImplicitResourceEnricher`: adds profile-scoped infrastructure.
- `ResourceGraphBuilder`/`GraphExpander`: build fixed-point DAG.
- `BatchScheduler`: provider-first Kahn batches.
- `ResourceProvisioningService`: application service executing resource batches.
- `OutputBindingResolver`: resolves context and completed provider outputs.

## Deployment Delta Snapshot contract

`DeploymentDeltaSnapshot` là artifact bất biến được persist cho từng
deploy/update/remove và có metadata/identity độc lập với `DeploymentPlan`.
Preview chỉ trả transient Delta document cùng shape, không persist Snapshot.
Nội dung canonical:

```text
modules.add                 map workload ID -> full module
modules.remove              sorted workload IDs
modules.update.<workload>   RFC 6902 patch relative to that module
shared                      RFC 6902 patch relative to the shared object
```

Object keys và output operations deterministic. Array diff xử lý index chung,
remove đuôi theo thứ tự giảm dần và append bằng `/-`. Applying the Delta to the
base Deployment Set must reproduce the Candidate Deployment Set exactly.

Snapshot này không phải mutable `DeploymentDelta` của Humanitec. Standalone
Delta create/update/archive lifecycle nằm ở D05; khác biệt contract được tổng
hợp trong compatibility matrix.

`ContainerResourceRequirements` giữ optional requests/limits cho `cpu` và
`memory`. Nó thuộc workload module, không phải Resource Graph node, và được
Workload Renderer ánh xạ nguyên vẹn sang Kubernetes container resources.

## Persistence vs integration state

- Database owns logical identity, lifecycle, plan snapshot, output metadata and current-set pointer.
- Terraform backend owns Terraform physical state; database keeps opaque state reference/fingerprint, not the state file.
- Kubernetes owns live objects; database keeps target, workload identity, manifest digest and observed status.
- Secret Store owns secret values; database keeps opaque secret references and redacted output metadata.

## Cross-aggregate consistency

- CRUD operations use one local transaction per aggregate.
- Deployment creation persists Deployment + immutable Delta Snapshot + Candidate Set + Plan before external execution.
- Each resource result is persisted after its external call in a short transaction.
- Final deployment transaction performs optimistic Environment version check, updates current-set pointer, workload instances and Deployment `SUCCEEDED` atomically.
