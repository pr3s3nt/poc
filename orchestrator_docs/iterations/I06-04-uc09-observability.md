---
id: I06-04
artifact: iteration-plan
status: current
last_reviewed: 2026-09-21
related: UC-09
---

# I06-04 — Complete UC-09 observability

## Objective

Hoàn thiện UC-09 trên read model hiện có: deployment history, filter và quan sát
trạng thái/graph/resources/workloads một cách nhất quán qua API và Web Console.

## In scope

- Đối chiếu toàn bộ [UC-09 specification](../usecase/UC-09/specification.md),
  realization, sequence, VOPC và traceability trước khi code.
- Hoàn thiện read operations và repository queries cho history/filter.
- Hoàn thiện Web Console states: loading, empty, validation, success và API
  error cho danh sách/details liên quan.
- Giữ redaction của secret outputs.
- Bổ sung Go tests, frontend tests và cập nhật code map/current state.

## Out of scope

- UC-01..UC-05 admin/preview flows.
- UC-07 update/remove execution.
- Rollback, retry, RBAC, audit và failure recovery.
- Durable Terraform state và stable cloud physical names.

## Exit criteria

- Mọi UC-09 main-flow step được map tới operation, code và test thực thi.
- History và filter hoạt động qua `/api/v1/` và Web Console.
- Deployment details vẫn hiển thị graph, batches, Active Resources, workloads và
  redacted outputs.
- `go test ./...`, Go build/vet, frontend typecheck/lint/test/build và
  documentation checker pass.
- `CURRENT_STATE.md`, code map, deviations và traceability được cập nhật.

## First next action

Đọc UC-09 context/specification/realization cùng query handler, deployment
repository và frontend deployment features; lập gap list bám từng `MS-nn` trước
khi chỉnh code.
