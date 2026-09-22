---
id: UC-05-REALIZATION
artifact: use-case-realization
status: current
last_reviewed: 2026-09-22
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

## Trace main flow

| Step | Collaboration |
|---|---|
| MS-01 | Load Application, Environment và current Deployment Set snapshot/version. |
| MS-02–MS-03 | `WorkloadSpecValidator` validate container resources; `ScoreConverter` bảo toàn requests/limits trong typed module spec. |
| MS-04 | Validate before-state; `DeltaBuilder.BuildHumanitecDelta` tạo transient Delta document và Candidate Set rồi chứng minh invariant. Preview không persist Snapshot entity. |
| MS-05 | Enrich implicit resources, build/expand graph và match Definitions. |
| MS-06–MS-07 | Inspect contracts, classify Active Resources, schedule batches. |
| MS-08 | Trả immutable `DeploymentPreview`; không gọi UC-08. |

## Transaction boundary

Read-only operation. `DeploymentPreview` chứa Environment version để UC-06 có thể phát hiện preview/snapshot cũ nếu preview được tái sử dụng sau này.

## Planned tests

- `TestPreview_NoRuntimeMutation`.
- `TestPreview_DeltaInvariant`.
- `TestPreview_HumanitecDeltaShapeAndArrayDiff`.
- `TestPreview_PreservesContainerResourceRequirements`.
- `TestPreview_AWSImplicitGraph`, `TestPreview_InternalImplicitGraph`.
- Reuse 33 planner fixtures như Go conformance tests và bổ sung contract tests riêng cho Delta/container resources vì fixture hiện tại không assert hai vùng này.
