---
id: DOMAIN-OBJECTS
artifact: domain-persistence-classification
status: current
last_reviewed: 2026-10-09
---

# Domain Objects and Persistence Classification

## Aggregates

| Aggregate | Root | Owned objects | Invariant chính |
|---|---|---|---|
| User Account | `UserAccount` | password hash, Organization identity, role, status | Username unique; chỉ `ACTIVE` account được authenticate; plaintext password không persist. |
| Session | `Session` | token hash, expiry, lifecycle status | Opaque token chỉ tồn tại ở browser/request memory; token hash unique và session revoked/expired không authenticate được. |
| Application | `Application` | system ID, name, subdomain; legacy provider metadata | ID do hệ thống sinh; execution binding belongs to Environment; legacy AWS scope retained only for migration. |
| Environment | `Environment` | namespace identity, versioned connection/store/generation/profile/region/runtime/scope, current-set pointer, public-route pending flag | Thuộc một Application; UC-01 tạo đúng `staging` và `production`; desired endpoint được suy ra từ Application Subdomain và Environment key; current Deployment Set chỉ đổi trong final deployment transaction. Route pending cho phép retry sau lỗi Fleet mà không restart workload. |
| Resource Type | `ResourceType` | input/output schema | Contract độc lập implementation. |
| Resource Definition | `ResourceDefinition` | optional Execution Profile guard, Matching Criteria, driver inputs, provision rules | Cùng Resource Type và profile hợp lệ; criteria match deterministic; output contract tương thích. |
| Connection | `Connection` | name, kind, authentication type, non-secret config, verification metadata | Chỉ secret reference được persist; chỉ `READY` được sử dụng. |
| Deployment | `Deployment` | `DeploymentDeltaSnapshot`, `DeploymentPlan`, deployment-resource progress, deployment-workload snapshots | Delta Snapshot immutable ngay khi persist; Plan immutable khi Deployment rời `PLANNING`; workload snapshot chỉ đổi khi Deployment đang chạy; `SUCCEEDED` chỉ sau workload readiness. |
| Active Resource | `ActiveResource` | executor state, outputs | Logical identity unique theo Organization + descriptor + scope. |
| Workload Instance | `WorkloadInstance` | manifest digest/status | Unique theo Environment + workload ID. |

`DeploymentSet.modules.<workload>.spec.service.publicRoutes` is an optional
orchestrator extension mapping public URL paths to declared Service ports.
Paths are unique across the Environment; legacy `publicPort` aliases `/`.
The resulting Environment-owned Ingress is runtime state of the internal-kind
`PublicRouteManager`, reconciled after workload readiness. It is not a
separate database aggregate.

## Entities and value objects

| Kind | Objects | Persisted form |
|---|---|---|
| Entity | `UserAccount`, `Session`, `Application`, `Environment`, `Connection`, `ResourceType`, `ResourceDefinition`, `DeploymentSet`, `DeploymentDeltaSnapshot`, `Deployment`, `DeploymentPlan`, `ActiveResource`, `WorkloadInstance` | Dedicated tables. |
| Child entity | `MatchingCriterion`, `DeploymentResource`, `DeploymentWorkload` | Dedicated child/join tables. |
| Value object | `ExecutionProfile`, `NamespaceIdentity`, `ResourceDescriptor`, `ResourceScope`, `DeploymentStatus`, `ResourceStatus`, `DriverType`, `OutputBinding`, `DeploymentTarget`, `ModuleDelta`, `JSONPatchOperation`, `ContainerResourceRequirements`, `ComputeResources`, `RenderBundle`, `RenderingSelection` | Scalar/JSONB columns; validated at construction. |
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
- `AuthenticationService`: verifies internal accounts and creates/revokes opaque sessions.

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
`ComputeResources` giữ chuỗi Score nguyên văn; field rỗng nghĩa là không khai
báo. Request suy ra cho field không khai báo (limit cùng field, rồi platform
default) chỉ được áp dụng khi render (UC-06 BR-11), không được ghi vào
Deployment Set.

## Persistence vs integration state

- Database owns logical identity, lifecycle, plan snapshot, output metadata and current-set pointer.
- Database owns internal account and session records; password hashes and session-token hashes are the only credential material persisted.
- Terraform backend owns Terraform physical state; database keeps opaque state reference/fingerprint, not the state file.
- Kubernetes owns live objects; database keeps target, workload identity, manifest digest and observed status.
- Secret Store owns secret values; database keeps opaque secret references and redacted output metadata.

## Cross-aggregate consistency

- CRUD operations use one local transaction per aggregate.
- Deployment creation persists Deployment + immutable Delta Snapshot + Candidate Set + Plan before external execution.
- Each resource result is persisted after its external call in a short transaction.
- Final deployment transaction performs optimistic Environment version check, updates current-set pointer, workload instances and Deployment `SUCCEEDED` atomically.

## New public catalog registration

Identifier validation is owned by UC-02 BR-05/BR-06 and UC-03 BR-10, at the
registration service boundary. Existing catalog objects and planner fixtures
are not migrated. Driver Inputs validation follows UC-03 BR-11–BR-14, using
embedded module contracts or static runtime driver contracts before insertion.
No new aggregate, table or persistence uniqueness rule is introduced.

## Connection execution credentials

[Shared credential design](../connection-credentials.md) owns credential store
and execution resolution. Authentication type is separate from destination kind;
new Kubernetes records use uploaded selected-context config. AWS identity is
for infrastructure provisioning; ADR-009 owns its storage decision. Legacy
host-context records remain compatible, and public DTOs exclude secret refs.

## Rendering values (ADR-010)

`RenderBundle` is immutable startup configuration (ID, version, binary/content
digests), not an Active Resource. `RenderingSelection` pins a workload
Definition/content hash and bundle in deployment plan JSON. Neither stores
credentials or changes resource/workload scope relationships. Native rendering
uses an omitted selection entry, including legacy plans without this metadata.

## Historical Environment execution binding (ADR-011; superseded below)

UC-01 BR-07..14 and [ADR-011](../decisions/ADR-011-environment-execution-binding.md)
own set-once Environment target. New Environments are UNCONFIGURED; targets may
have different kinds/regions/profiles in one Application. SetConnection is atomic,
scoped, versioned and immutable even before deploy. Runtime status may advance
without changing target. New AWS VPC/EKS have Environment scope; migrated legacy
AWS bindings keep application-scope identity/state. Canonical Environment resolver
feeds planner/matcher/executors/queries; Application legacy fields do not select
new targets. Preview/Deploy require configured binding.

## Editable Environment destinations (current ADR-012)

This supersedes the earlier set-once/provider prose. Environment owns independently
editable execution/store selections and version, target generation and active
operation identity. SecretStoreConnection is a separate Organization aggregate;
only credential ref/nonsecret provider configuration is persisted. EnvironmentOperation
is persisted admission/transition state; old targets/resources survive in immutable
retained-generation records. ConfigurationEntry contains ordinary Variable value
or owning store key + opaque Secret ref. SecretStoreResolver and PostgreSQLTransfer
are ports/adapters; application services coordinate copy, generation and cutover.

## Implicit internal cluster binding

[ADR-013](../decisions/ADR-013-implicit-existing-cluster.md) adds a trusted
system execution binding for the profile-enriched internal cluster node. It
uses a reserved ResourceDefinition identity for existing persistence FKs,
bypasses weighted matching, and resolves the pinned Environment Connection.
Existing logical descriptor/Application scope and historical targets stay intact;
no new domain table or cluster creation is introduced. Other Definition matching
and the common Kubernetes target contract remain unchanged.
