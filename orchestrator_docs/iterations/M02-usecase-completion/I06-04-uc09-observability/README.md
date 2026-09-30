---
id: I06-04
artifact: iteration-plan
status: current
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

## First next action

Lập lại gap list bám từng UC-09 `MS-nn` trên checkout hiện tại trước khi tiếp
tục code.

## Outcome

Được kích hoạt lại sau khi M00-a hoàn thành ngày 2026-09-30; chưa hoàn thành.
