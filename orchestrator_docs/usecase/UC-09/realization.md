---
id: UC-09-REALIZATION
artifact: use-case-realization
status: current
last_reviewed: 2026-10-28
---

# UC-09 — Use Case Realization

## Trách nhiệm

Tạo read-only deployment view từ persisted plan/status/resource/workload snapshots và loại secret outputs.

## System operation

```go
DeploymentQueryService.GetDeployment(ctx context.Context, query GetDeploymentQuery) (*DeploymentView, error)
```

## Participants

- `DeploymentQueryController`, `DeploymentQueryService`.
- `ResourceQueryController/Service`: đọc resource detail đúng Environment và
  original generation, consumers/eligibility và tên xác nhận.
- `ResourceCleanupQueryService`: đọc scoped cleanup operation/tập đã xác nhận,
  progress/absence/remaining IDs từ persistence; không gọi provider.
- `DeploymentViewAssembler`, `OutputRedactor`.
- `DeploymentRepository`, `DeploymentSetRepository`,
  `DeploymentResourceRepository`, `DeploymentWorkloadRepository` and Resource
  Type read ports.

## Trace main flow

| Step | Collaboration |
|---|---|
| MS-01 | Authenticated controller tạo query với Organization, Application, Environment và Deployment IDs. |
| MS-02–MS-03 | Service đọc immutable Deployment/set snapshots và resource/workload status cùng safe stage/attempt/module/target từ persisted attempts. |
| MS-04–MS-05 | Assembler thêm status metadata và persisted graph/matches/batches nếu có; `FAILED` khi planning chưa tạo Snapshot chỉ trả status/failure reason, không dựng Delta/plan giả. |
| MS-06 | `OutputRedactor.Redact` theo Resource Type output metadata; resolved resource inputs không thuộc view. |
| MS-07 | Trả immutable `DeploymentView`; không gọi runtime/executor. |

## Transaction boundary

Read-only consistent query. Persistence read-snapshot boundary uses PostgreSQL
`REPEATABLE READ READ ONLY` or a private committed-state copy for memory/JSON,
so assembly cannot mix commits or persist a mutation. Resource output visibility
fails closed when contract metadata is missing or does not classify an output
as non-secret; nested secret references are redacted as well.

Startup reopens persisted per-Deployment history without reconstruction from
current Workload Instances. Older persistence formats require development reset
under [T28 baseline](../../architecture/database/schema.md#fresh-persistence-baseline-t28--leg-08);
no current or historical run receives fabricated status rows.

## Planned tests

- `TestGetDeployment_ReturnsPersistedPlanAndStatuses`.
- `TestGetDeployment_RedactsSecretOutputs`.
- `TestGetDeployment_DoesNotCallRuntimeAdapters`.
- Organization/Application/Environment scoping, status-filter and
  per-Deployment workload-snapshot tests.
- Web Console list/detail: Environment filter, navigation, status/failure,
  read-only resource/workload rows and secret-safe output rendering.


## Resource detail và cleanup query (RES-01..03)

Resource detail dùng instance/ActiveResource association trong Environment,
không yêu cầu resource còn trong current graph. Scope/role lấy từ session;
eligibility false khi metadata thiếu hoặc có operation/consumer. Tên xác nhận
là persisted descriptor hiển thị trên detail. UC-07 nhận mutation; UC-09 chỉ
project delete plan/status/remaining IDs, không tự execute hoặc refresh cloud.
Console polling giữ operation ID/scope, bỏ late responses; failure không
được phủ nhận tiến độ đã commit hoặc suy là tài nguyên đã xóa. Historic
Deployment view không lấy lifecycle hiện tại thay snapshot của lần triển khai.
Tests gồm read-only/scoped/secret-safe query, stale response và cleanup
partial/verified result; headed A01 ghi nhận hành vi trên resource detail.

## Visual observation collaboration (OBS-01..07)

`DeploymentQueryService` captures plan review, attempt/resource/workload facts and
manifest reviews in its existing read-only consistent snapshot. New pure
`NodeProgressProjector.Project(plan, observations)` maps status precedence,
readiness, dependency waiting/blocked and unknown cases under the shared review
contract. `DeploymentViewAssembler` joins stable node IDs and artifact provenance;
`OutputRedactor` remains the final secret boundary. No provider refresh and no
mutation of historical snapshots.

List accepts optional scoped `reviewRequestId` filter; admission supplies associations
through UC-06/07. Console observes only returned exact deployment IDs, displays
multi-run provenance for a pending batch and preserves unknown/unadmitted nodes.
Single-flight polling cancels stale requests and stops terminal runs. Elapsed is
computed from persisted timestamps; missing data is explicit. Existing task
operation/POST remains authoritative for batch completion and route sync.

Tests planned: READY versus APPLIED/READINESS, current failed attempt precedence,
retain proof, transitive blocked consumers, parallel observed branches, missing
rows/timestamps, deployment scoping/correlation, immutable history and redaction,
poll race/error/terminal handling. Browser acceptance uses actual HTTP DTO flow;
fixtures prove UI only, never AWS execution.

## Native telemetry collaboration (TEL-01..07)

A separate `ObservabilityService` discovers current Workload Instances, verifies
owning Deployments/opaque targets, leases a source and calls `TelemetryReader`
outside a transaction. The Kubernetes adapter resolves scoped credentials,
reads bounded pods/logs/Metrics API and emits facts plus safe coverage codes.
`TelemetryRepository` commits facts/checkpoints under a fence. Cleanup prunes
expired telemetry only, independently of resource cleanup and Environment claims.

Authenticated `ObservabilityController` queries the same repository through
`ObservabilityService`, producing interval summaries, exact log pages and bounded
metric series; it never calls the reader. Console home/detail share source facts
and keep missing/current/peak values distinct. Collaboration:
[sequence](telemetry-sequence.puml), [image](telemetry-sequence.png).

## ADR-030 collaboration extension

ObservationWorker reads persisted admitted revisions under per-unit leases and maps provider observations to sync/health/drift. QueryController serves stored summaries; historical Deployment state remains unchanged.

Canonical shared collaboration: [delivery sequence](../../architecture/components/delivery-reconciliation-sequence.puml),
[participants](../../architecture/components/delivery-reconciliation-vopc.puml),
[infrastructure sequence](../../architecture/components/infrastructure-drift-sequence.puml).
These supplement the existing direct-path diagrams for this new design slice;
[CDR contract](../../architecture/contracts/delivery-reconciliation.md) owns port/API semantics.
