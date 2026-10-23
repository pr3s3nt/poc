---
id: DOMAIN-OBJECTS
artifact: domain-persistence-classification
status: current
last_reviewed: 2026-10-23
---

# Domain Objects and Persistence Classification

## Aggregates

| Aggregate | Root | Owned objects | Invariant chính |
|---|---|---|---|
| User Account | `UserAccount` | password hash, Organization identity, role, status | Username unique; chỉ `ACTIVE` account được authenticate; plaintext password không persist. |
| Session | `Session` | token hash, expiry, lifecycle status | Opaque token chỉ tồn tại ở browser/request memory; token hash unique và session revoked/expired không authenticate được. |
| Application | `Application` | system ID/key, Organization, name, subdomain, version | ID do hệ thống sinh; không có target/provider fields; Secret Store binding thuộc Environment; deployment targets/account/region theo từng resource/workload, không backfill từ Application (T26/ADR-017). |
| Environment | `Environment` | namespace identity, versioned Secret Store/configuration, identity generation/scope, operation owner, current-set pointer, public-route pending flag | Thuộc một Application; UC-01 tạo đúng `staging` và `production`; desired endpoint được suy ra từ Application Subdomain và Environment key; current Deployment Set chỉ đổi trong final deployment transaction. Route pending cho phép retry sau lỗi Fleet mà không restart workload. |
| Resource Type | `ResourceType` | input/output schema | Contract độc lập implementation. |
| Resource Definition | `ResourceDefinition` | Name, version, Driver Account, Matching Criteria, driver inputs, provision rules | UC-03 V02: immutable ID/type, CAS edits; five-field matching without profile filter; compatible account/region per Definition. |
| Connection | `Connection` | name, kind, authentication type, non-secret config, verification metadata | Chỉ secret reference được persist; chỉ `READY` được sử dụng. |
| Deployment | `Deployment` | `DeploymentDeltaSnapshot`, `DeploymentPlan`, deployment-resource progress, deployment-workload snapshots | Delta Snapshot immutable ngay khi persist; Plan immutable khi Deployment rời `PLANNING`; workload snapshot chỉ đổi khi Deployment đang chạy; `SUCCEEDED` chỉ sau workload readiness. |
| Active Resource | `ActiveResource` | executor state, outputs | Logical identity unique theo Organization + descriptor + scope. |
| Workload Instance | `WorkloadInstance` | manifest digest/status | Unique theo Environment + workload ID. |

`DeploymentSet.modules.<workload>.spec.service.publicRoutes` is an optional
orchestrator extension mapping public URL paths to declared Service ports.
Paths are unique across the Environment; `publicPort` is rejected, with no alias
conversion or route fallback.
The resulting Environment-owned Ingress is runtime state of the internal-kind
`PublicRouteManager`, reconciled after workload readiness. It is not a
separate database aggregate.

## Entities and value objects

| Kind | Objects | Persisted form |
|---|---|---|
| Entity | `UserAccount`, `Session`, `Application`, `Environment`, `Connection`, `ResourceType`, `ResourceDefinition`, `DeploymentSet`, `DeploymentDeltaSnapshot`, `Deployment`, `DeploymentPlan`, `ActiveResource`, `WorkloadInstance` | Dedicated tables. |
| Child entity | `MatchingCriterion`, `DeploymentResource`, `DeploymentWorkload` | Dedicated child/join tables. |
| Value object | `NamespaceIdentity`, `ResourceDescriptor`, `ResourceScope`, `DeploymentStatus`, `ResourceStatus`, `DriverType`, `OutputBinding`, `DeploymentTarget`, `ModuleDelta`, `JSONPatchOperation`, `ContainerResourceRequirements`, `ComputeResources`, `RenderBundle`, `RenderingSelection` | Scalar/JSONB columns; validated at construction. |
| Planning document | `WorkloadFragment`, `CandidateDeploymentSet`, `ResourceGraph`, `ProvisionBatch` | Immutable JSONB snapshot in `deployment_plans`; typed Go model in memory. |
| Result | `ProvisionResult`, `DeploymentResult`, `DeploymentPreview`, `DeploymentView` | DTO/read model, not aggregate roots. |
| UC-17 | `EffectivenessBaseline` (entity, append-only versions); `Run`, `WorkItem`, `MetricResult` (read model derived from Deployment/cleanup history, not persisted) | `effectiveness_baselines`; read model only. |

## Domain services

- `PlanningService`: orchestrates pure/deterministic planning components.
- `DefinitionMatcher`: weighted criteria selection.
- `ImplicitResourceEnricher`: preserves required workload/namespace structure; normal matching/references select infrastructure (ADR-017), with read-only reference-fixture behavior isolated.
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
- Database owns internal account and session records; logical metadata chỉ giữ password/session-token hashes và opaque credential
  refs. Terraform state có thể chứa provider secrets nhưng chỉ lưu ciphertext
  trong restricted tables, không public JSON state.
- Terraform backend owns Terraform physical state. In v2 the Orchestrator HTTP backend stores encrypted state blobs in platform PostgreSQL, separate from logical metadata; logical resource records keep opaque state references, never plaintext state.
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
are not migrated. Driver Inputs validation follows UC-03 BR-11–BR-14, dùng verified module/schema/provider mappings trước insertion. Registry
bundles/state/attempts có aggregates và constraints riêng trong schema.

## Connection execution credentials

[Shared credential design](../connection-credentials.md) owns credential store
and execution resolution. Authentication type is separate from destination kind;
new Kubernetes records use uploaded selected-context config. AWS identity is
for infrastructure provisioning; ADR-009 owns its storage decision. Kubernetes requires explicit scoped KUBECONFIG; HOST_CONTEXT and missing
authentication fail closed. Public DTOs exclude secret refs.

## Rendering values (ADR-010)

`RenderBundle` is immutable startup configuration (ID, version, binary/content
digests), not an Active Resource. `RenderingSelection` pins a workload
Definition/content hash and bundle in deployment plan JSON. Neither stores
credentials or changes resource/workload scope relationships. RenderingSelection là bắt buộc trên mọi Workload cho cả hai profiles;
không dùng omitted entry để chọn renderer khác.

## Editable Environment destinations (current ADR-012)

Environment owns independently
editable execution/store selections and version, target generation and active
operation identity. SecretStoreConnection is a separate Organization aggregate;
only credential ref/nonsecret provider configuration is persisted. T23 removes
SecretStore legacy flags and backfill/conversion; bootstrap manages ordinary
stores through marker, endpoint identity and expected credential CAS. EnvironmentOperation
is persisted admission/transition state; old targets/resources survive in immutable
retained-generation records. ConfigurationEntry contains ordinary Variable value
or owning store key + opaque Secret ref, never both. T24 rejects store-backed
Variable formats and removes materialization; Variable CRUD has no store calls
and store switches preserve Variables while copying only Secrets. SecretStoreResolver and PostgreSQLTransfer
are ports/adapters; application services coordinate copy, generation and cutover.

## Implicit internal cluster binding

[ADR-013](../decisions/ADR-013-implicit-existing-cluster.md) adds a trusted
system execution binding for the profile-enriched internal cluster node. It
uses a reserved ResourceDefinition identity for existing persistence FKs,
bypasses weighted matching, and resolves the pinned Environment Connection.
Existing logical descriptor/Application scope and historical targets stay intact;
no new domain table or cluster creation is introduced. Other Definition matching
and the common Kubernetes target contract remain unchanged.

## T27 — Infrastructure binding scope

InfrastructureScope has only ENVIRONMENT. Không có Application-scope infrastructure migration. Current generation-0 identities, later
generation isolation and the ADR-013 internal cluster Application resource scope
remain unchanged. Retired/unknown binding scopes are rejected without conversion.

## Terraform resource và cleanup domain

- `TerraformModuleBundle`: immutable verified module/release contract, a platform
  registry entity. Definition references it; Plan pins its snapshot/digests.
  Direct Git inspection (issue #7) retains optional credential-free `sourceUrl`
  in its immutable contract; tag resolution is preparation-only.
- `TerraformResourceInstance`: aggregate for stable Organization/Environment/
  generation/descriptor/scope identity, ownership, original target, state key
  and write-ahead physical intent. One per private/shared/infrastructure instance.
- `TerraformStateSnapshot`: restricted encrypted integration state associated
  one-to-one with instance; no public DTO contains raw state or secret checksum.
- `TerraformStateLock` / invocation capability: persisted instance lock under
  Environment operation owner/fence. Expiry is a signal, not permission to unlock.
- `TerraformResourceAttempt`: action/stage/ownership/physical-ID provenance for
  CREATE/UPDATE/DESTROY, written before external side effects and retained for cleanup.
- `CleanupPlanner`: liệt kê candidates và kiểm selected set, retained reasons và token
  pin scope/versions/actor cho hộp xác nhận của PE/Admin.
- `ResourceCleanupService`: nhận user-confirmed scope hoặc narrow test-run
  capability; kiểm ownership/consumer, reverse dependency và provider absence.
- `ResourceCleanupOperation`: persisted actor, authorization mode, confirmed
  set/token hash/versions, original targets, stages và remaining IDs.
- `ResourceQueryService` / `ResourceCleanupQueryService`: scoped read models
  cho detail/progress; không provider refresh, không state/credential bytes.

Private Redis/PostgreSQL belongs to Workload within Environment/generation;
shared resource belongs to Environment/generation with multiple consumers.
Platform Connection/cluster is reference-only. Logical descriptor scope preserves
existing planner semantics; ownership and run provenance are explicit additional
metadata, not guessed from descriptor or namespace prefix. Required structures/
constraints are canonical in [schema](../database/schema.md#terraform-v2-schema-design-adr-014-not-yet-implemented).


## V01 infrastructure account

[ADR-015](../decisions/ADR-015-infrastructure-driver-accounts.md) uses Connection
as the backing account aggregate, adding optimistic Version and an AWS/INTERNAL
public type distinct from authentication. Identity/key/Organization remain stable;
metadata-only edit, private credential replacement and read-only verify are
separate operations. Account config is not an applicability filter. Physical
Connection names/legacy consumers remain until V02–V03 replacement; no copied
credential aggregate or Environment assignment is added. SQL constraints belong
to [schema](../database/schema.md); behavior belongs to UC-04 AC-01..12.


## V03 placement model

[ADR-016](../decisions/ADR-016-definition-selected-workload-targets.md) extends workload rendering selection into lifecycle selection with per-workload namespace/cluster pins; ResourceDefinition references/provision participate before graph fixed point. Typed cluster outputs carry opaque scoped target, not auth bytes. Resource instances and applied workload records preserve independent target identity and namespace through restart/update/remove/cleanup. V03 one-cluster admission is a service capability check, not a single-cluster domain cardinality. Internal cluster Definition owns account choice; consumers inherit it.


## V04–V06 Definition dependency model

[ADR-017](../decisions/ADR-017-definition-dependency-execution.md) replaces the
remaining profile-enricher/Environment destination model above. Environment
retains application scope, namespace base identity, current Set/config/store
versions and operation owner/fence; it does not supply an infrastructure account
or region. ExecutionProfile and old deployment-binding compatibility retire
after V05.10/V06.08 replacement gates.

Resource Definition and references own account/region/dependency selection;
WorkloadPlacement and original resource/state/applied-target pins retain cluster
identity per consumer. Typed ClusterTargetIdentity can represent an existing
Internal cluster or EKS produced in the same graph; Preview keeps unresolved
typed provenance. EKS resolved target adds provider/region/original AWS identity,
with credentials reconstructed per invocation. No new lifecycle aggregate or
managed-only public entry is introduced. Reuse and cleanup preserve original
identity, ownership, scoped descriptor and state through restart/catalog edits.

## Deployment review values — ADR-019

`DeploymentReview`, `ReviewArtifactSide`, `ArtifactProvenance` and
`NodeProgress` are safe read models/value objects, not executable plans or
resource lifecycle entities. Plan owns immutable desired review; per-deployment
workload snapshot owns sanitized rendered review. Deployment optionally records
admission-bound reviewRequestId for correlation. The planner graph/identity and existing
successful workload/resource associations remain authoritative. See
[review contract](../contracts/deployment-review.md) and
[schema](../database/schema.md#visual-review-persistence--adr-019-required-design).
