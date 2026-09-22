---
id: I06-05
artifact: iteration-plan
status: current
last_reviewed: 2026-09-22
related: IMP-010, UC-03
---

# I06-05 — Fix conformance catalog semantics

## Objective

Đóng IMP-010 bằng cách làm conformance adapter giữ đúng khác biệt giữa
Definition thiếu/`criteria: []` và criterion `{}`, không thay đổi invariant của
product registration/planner.

## Canonical inputs

- [UC-03 specification](../../../usecase/UC-03/specification.md), BR-07.
- [Known deviations](../../../implementation/deviations.md), IMP-010.
- [Compatibility matrix](../../../implementation/humanitec-compatibility.md),
  Empty matching criteria.
- [Planner challenge](../../../../orchestrator_reference/humanitec-planner-challenge-v4/PROBLEM.md),
  section Matching Criteria.

## In scope

- Conformance loader bỏ external Definition thiếu criteria hoặc có `[]` khỏi
  challenge catalog.
- Criterion `{}` vẫn được tạo thành wildcard score `0`.
- Tests phân biệt missing, `[]` và `[{}]`; chạy lại đủ 33 fixture.
- Cập nhật implementation status/traceability/evidence và xóa IMP-010 khi pass.

## Out of scope

- Thay `Definition.Validate` hoặc behavior fail-fast của product catalog; D07 sở
  hữu quyết định đó.
- IMP-008/009 hoặc planner public error contract IMP-006.

## Exit criteria

- Adapter không còn chế tạo wildcard từ missing/`[]`.
- Product planner vẫn từ chối catalog vi phạm invariant có ít nhất một
  criterion.
- Loader unit tests và `go test ./test/conformance/ ./internal/planning/...`
  pass 33/33.
- IMP-010 được xóa và một dated verification record được link tại outcome.

## First next action

Viết characterization tests cho ba input shape trước khi sửa loader.

## Outcome

Chưa thực hiện.
