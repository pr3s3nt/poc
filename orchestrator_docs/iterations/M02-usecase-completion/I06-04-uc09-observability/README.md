---
id: I06-04
artifact: iteration-plan
status: historical
last_reviewed: 2026-09-30
related: UC-09, IMP-005
---

# I06-04 — Complete UC-09 observability

## Objective

Hoàn thiện UC-09 trên read model hiện có: deployment history, filter và quan sát
trạng thái/graph/resources/workloads một cách nhất quán qua API và Web Console.
Iteration này đã bắt đầu trước M01 và được reprioritize, không được coi là hoàn
thành hoặc bỏ scope.

## Canonical inputs

- [UC-09 context](../../../usecase/UC-09/README.md), specification,
  realization, sequence và VOPC.
- [Traceability matrix](../../../traceability/matrix.md), UC-09.
- [Known deviations](../../../implementation/deviations.md), IMP-005.

## In scope

- Hoàn thiện read operations/repository queries cho history/filter.
- Hoàn thiện Web Console loading, empty, validation, success và API error states.
- Giữ secret-output redaction và hiển thị graph/batches/resources/workloads.
- Bổ sung Go/frontend tests và cập nhật implementation-owned documentation.

## Out of scope

- UC-01..05 admin/preview flows và UC-07 update/remove execution.
- Rollback, retry, RBAC, audit và failure recovery.
- Durable Terraform state và stable cloud physical names.

## Exit criteria

- Mọi UC-09 main-flow step map tới operation, code và executable test.
- History/filter hoạt động qua `/api/v1/` và Web Console.
- Details giữ graph, batches, Active Resources, workloads và redacted outputs.
- Go test/build/vet, frontend typecheck/lint/test/build và docs checker pass.
- IMP-005 được xóa và dated verification record được link tại outcome.

## Completed work order

Đóng audit gaps theo thứ tự: authenticated scoped read contract →
per-Deployment workload snapshot và secret-safe view → frontend states/filter
→ local browser verification.

## Audit 2026-09-30

- List/detail artifacts, newest-first ordering, graph/matches/batches,
  deployment resources and output redaction đã có.
- GET deployment endpoints hiện không yêu cầu session và detail chỉ nhận ID,
  nên chưa bảo đảm Organization/Application/Environment scope ở backend.
- Detail lấy `workload_instances` hiện tại của Environment; deployment cũ có
  thể hiển thị state của run mới hơn. Canonical schema cần
  `deployment_workloads` snapshot.
- `ResourceView` đang serialize `resolvedInputs`; trường này không thuộc UC-09
  read contract và phải bị loại khỏi response.
- Status filter hiện chỉ lọc client-side; API mapping mới sở hữu validation và
  server-side filter. UI tests còn thiếu not-found/API-error/retry và browser
  flow thật.

## Outcome

Completed 2026-09-30. Authenticated scoped history/filter and detail, consistent
read snapshots, per-Deployment workload history, fail-closed output redaction,
React states and local browser redeploy/restart verification are implemented.
IMP-005 is closed. Memory/JSON and disposable local PostgreSQL tests passed;
kind/AWS/cloud verification was not run in this iteration.

Evidence: [UC-09 local verification](../../../verification/2026-09-30-uc09-local-observability.md).
