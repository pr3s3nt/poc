---
id: UC-05-REALIZATION
artifact: use-case-realization
status: current
last_reviewed: 2026-10-28
---

# UC-05 — Use Case Realization

## Trách nhiệm

Tạo preview deterministic từ Score và snapshot hiện tại bằng cùng planning pipeline của UC-06 nhưng không gọi executor, Kubernetes hoặc thay đổi persisted runtime state.

## System operation

```go
PreviewService.PreviewDeployment(ctx context.Context, query PreviewDeploymentQuery) (*DeploymentPreview, error)
PlanningService.Plan(ctx context.Context, req PlanRequest) (*DeploymentPlan, error)
```

## Participants

- `PreviewController`, `PreviewService` — boundary/control read-only.
- `PlanningService` — orchestrator của pure planning pipeline.
- `ScoreConverter`, `WorkloadSpecValidator`, `BeforeStateValidator`, `DeltaBuilder`.
- `ImplicitResourceEnricher`, `ResourceGraphBuilder`, `DefinitionMatcher`, `GraphExpander`.
- `DriverContractInspector`, `BatchScheduler`, `ActiveResourceClassifier`.
- Application/Environment/Resource/ActiveResource repositories — read ports.
- VerifiedModuleRegistry và ResourceInstanceMetadata — immutable read snapshot
  cho module/provider pins, stable state identity và resource versions.

## Trace main flow

| Step | Collaboration |
|---|---|
| MS-01 | Load Application, Environment và current Deployment Set snapshot/version. |
| MS-02–MS-03 | `WorkloadSpecValidator` validate container resources; `ScoreConverter` bảo toàn requests/limits trong typed module spec. |
| MS-04 | Validate before-state; `DeltaBuilder.BuildHumanitecDelta` tạo transient Delta document và Candidate Set rồi chứng minh invariant. Preview không persist Snapshot entity. |
| MS-05 | Enrich implicit resources, build/expand graph và match Definitions. |
| MS-06–MS-07 | Đọc verified contracts, kiểm known inputs, pin module/provider/renderer/ target/state identity, classify resources và schedule batches. |
| MS-08 | Trả immutable `DeploymentPreview`; không gọi UC-08. |

## Transaction boundary

Read-only operation. `DeploymentPreview` chứa phiên bản snapshot; pending Preview còn phát token
pin current Set/config/draft/bindings để Deploy kiểm trước side effects.

Standalone operation load Application, Environment, current Set, connection,
catalog và Active Resources qua `persistence.Store.ReadSnapshot` trước khi
gọi shared planner. Snapshot không được ghi và service không nhận executor,
provisioner hoặc Kubernetes deployer. API dùng explicit preview DTO, không
serialize toàn bộ Connection/ActiveResource hoặc persisted plan bất kỳ.
Authorization Organization lấy từ UC-00 session, không từ request body.
Standalone result không phải pending-change token và không được UC-16 Deploy
chấp nhận. Xem [HTTP mapping](ui/api-mapping.md).

## Planned tests

- `TestPreview_NoRuntimeMutation`.
- `TestPreview_DeltaInvariantAndContainerResources` plus shared planner
  Delta shape/array-diff conformance tests.
- `TestPreview_ReadsOneConsistentSnapshot`,
  `TestPostgresPreview_RepeatableReadAndNoWrites`.
- `TestPreview_MatchesDeployPlan` for add/update/remove.
- `TestPreview_AWSImplicitGraph`, `TestPreview_InternalImplicitGraph`.
- `TestUC05ScorePreviewHTTP`, safe public projection/error tests and
  `ScorePreviewPage.test.tsx` including obsolete-response rejection.
- Reuse 33 planner fixtures như Go conformance tests và bổ sung contract tests riêng cho Delta/container resources vì fixture hiện tại không assert hai vùng này.

## Definition-selected workload rendering collaboration

See [ADR-010](../../architecture/decisions/ADR-010-score-k8s-workload-rendering.md)
and [rendering contract](../../architecture/contracts/workload-rendering.md).
Catalog kiểm installed bundle cho cả profiles; planner yêu cầu một
score-k8s Definition và pin bundle. No match/tie/bundle thiếu thì dừng.
UC-06 validates availability before provisioning, then dispatches the existing
WorkloadRenderer port after output binding. UC-08 still executes resource-only
batches. Kubernetes/Fleet apply/readiness and Environment route ownership stay
in their existing adapters. No database transaction spans a CLI/external call.

## Selected Environment connection

Shared snapshot loader resolves persisted Environment target in session
Organization; UNCONFIGURED fails before planning/execution. Definition guards and
UC-08 defense-in-depth compare against this Environment connection. Context,
version, scope mode and nonsecret target metadata are pinned into hashes/snapshots.
AWS enrichment produces Environment/generation-scoped VPC/EKS only (T27).
Retired scopes fail closed; current generation-0 identity is preserved under [ADR-011](../../architecture/decisions/ADR-011-environment-execution-binding.md).
Target changes use ADR-012 versioned admission/transition; runtime READY update is Environment-only.
UI/query use selected Environment or deployment-pinned target, no Application target.

## Implicit internal cluster collaboration

[ADR-013](../../architecture/decisions/ADR-013-implicit-existing-cluster.md)
changes only internal cluster binding: the enricher supplies the provider node,
trusted system binding bypasses criteria matching, and UC-08 resolves the selected
Environment Connection through existing-cluster. Preview is pure; execution
admits the reserved system Definition before persistence writes. Other resource
Definitions, executor scheduling and output propagation remain unchanged.

## Matching preview boundary (T19)

Catalog `PreviewMatching` của UC-03 dùng cùng domain weighted criteria và
winner account/target guard theo UC-03 V02-05, không profile filter và không gọi full PlanningService.Plan. Kết quả
không là input admission cho UC-05/06; shared planner tiếp tục kiểm snapshot mới.


## V03 collaboration replacement

[ADR-016](../../architecture/decisions/ADR-016-definition-selected-workload-targets.md) replaces internal profile enrichment/system binding in the collaboration above. Catalog validates public existing-cluster/Internal account and workload typed namespace/cluster refs plus provision rules. Planning matches workload first and expands all nodes to fixed point; same normal matcher selects cluster Definition. Target resolver builds/pins per-consumer placement, rejects mismatch/ambiguous/cycle/multi-cluster V03. Shared snapshot/admission compare pins/versions; UC-08 resolves cluster then owned namespace/resources. UC-06 renders/applies/waits on selected workload target; no global target or builtin persistence. Applied target restored for Update/Remove/restart; separate cleanup preserves preexisting cluster. AWS legacy collaboration remains until V05–V06.


## V04–V06 collaboration

[ADR-017](../../architecture/decisions/ADR-017-definition-dependency-execution.md)
replaces the remaining profile-driven collaboration above. Select workload
Definition, expand all authored references/provision/provider bindings to fixed
point, validate full DAG and pin each node's Definition/account/region/placement.
Planning keeps unknown typed outputs and safe provenance without provider calls.
Provisioning dispatches matched driver with private invocation credentials,
validates completed outputs/readiness and persists original state/target pins.
Namespace/workload resolve their explicit cluster; EKS target auth is reconstructed
per invocation after API/network/node readiness. OutputBindingResolver renders
only after dependencies pass. Failure blocks consumers and preserves progress.
Cleanup walks reverse actual graph using original state/module/identity and
ownership/UID guards, protecting shared/preexisting objects. See cloud sequence
and module/state contracts for target and secret boundaries.

## Visual review collaboration (VIS-01..08)

`PreviewService` / pending `PreviewDesired` capture one PlanningSnapshot plus
baseline artifact associations, run shared PlanningService, then close its read
transaction. `DeploymentReviewAssembler.Assemble(snapshot, plans)` coordinates
`VerifiedModuleSourceReader.ReadPinned(pin)`, `WorkloadPreviewRenderer.Preview`
and `ReviewRedactor.Project` outside DB transactions. Source reader cannot fetch;
preview renderer uses selected installed bundle/shared execution normalization,
symbolic unknowns and output-only sandbox. No executor/provider/secret-store port
is available. `ReviewBaselineResolver` uses only captured successful associations.

Assembler returns additive versioned `review` to explicit safe HTTP DTOs and
pending final graph; resource IDs deduplicate shared nodes without dropping
provenance. Contract owns bounds/availability; unsupported preview returns a safe
reason rather than sample code. Frontend `DeploymentReview` reuses Console shell,
node inspector and bounded diff view in pending and standalone screens. Pending
starts scoped UC-09 observation using fresh request correlation bound at admission.

Tests planned: read-only/no-fetch/no-provider spies, selected-bundle parity,
unknown-output structural guard, exact source-pin verification, baseline missing/
failed/generation mismatch, shared node aggregation, source/manifest nested-secret
redaction, bounded artifacts/diff, stale request discard, code copy, desktop UI.

Node facts follow VIS-09 and the shared review contract: the assembler projects
bounded semantic fields from already validated Terraform input contracts and
verified pinned root-module literal outputs. Sensitive/executor-owned nil values
are omitted; unresolved references stay unknown. The frontend derives workload
facts only from sanitized Deployment/Service artifacts and uses public placement
metadata for legacy/missing summaries. No executor, state or secret resolver is
introduced. Layout metrics are shared by node rendering and connector routing;
unique approach/outer lanes preserve the graph and full reason provenance.

## Git render-bundle Preview — ADR-021

Scoped planning snapshot includes immutable Git bundle metadata and effective
input contract. Selection/pending comparison pins content/engine/binary/inputs.
After snapshot close, DeploymentReviewAssembler calls the same Git render engine
with neutral symbolic input, verified local cache and no secret bytes. Safe
projection extends only to approved customization fields, retaining unknown-path
provenance. No fetch/provider call enters planning or Preview. Sequence and
shared classes include RenderBundleRegistry as the immutable source boundary.

## ADR-027 collaboration extension

Snapshot loader includes desired/applied binding, capability digest, policy and inventory pins. Planner returns selection-only/transition actions without external calls; admission checks the same pins.

Canonical shared collaboration: [delivery sequence](../../architecture/components/delivery-reconciliation-sequence.puml),
[participants](../../architecture/components/delivery-reconciliation-vopc.puml),
[infrastructure sequence](../../architecture/components/infrastructure-drift-sequence.puml).
These supplement the existing direct-path diagrams for this new design slice;
[CDR contract](../../architecture/contracts/delivery-reconciliation.md) owns port/API semantics.
