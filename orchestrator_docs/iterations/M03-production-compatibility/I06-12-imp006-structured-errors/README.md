---
id: I06-12
artifact: iteration-plan
status: deferred
last_reviewed: 2026-09-22
related: IMP-006, D02
---

# I06-12 — Decide and implement structured planner errors

## Activation condition

Chỉ activate khi structured `phase/code/path` được duyệt là product/public API
contract hoặc challenge compatibility boundary cần duy trì.

## Objective

Đóng IMP-006 bằng typed planner errors và deterministic boundary mapping theo
contract đã chọn, không để fixture shape ngầm trở thành public requirement.

## In scope

- Quyết định ownership của error taxonomy và versioning/API exposure.
- Typed phase/code/path errors at planner boundaries.
- Map six rejected fixtures và product/API error responses.
- Unit/conformance/HTTP contract tests và observability-safe messages.

## Out of scope

- Retry policy, localization hoặc arbitrary provider error normalization.
- Thay business behavior của accepted planner cases.

## Exit criteria

- D02 decision được chốt trước code.
- Rejected fixtures so phase/code/optional path nếu compatibility được chọn.
- Public API mapping không leak secret/internal implementation details.
- IMP-006/D02 được đóng hoặc iteration kết thúc bằng explicit deferral record.

## First next action

Chốt D02: product contract, compatibility adapter hay không hỗ trợ; chỉ sau đó
mới thiết kế typed error model.

## Outcome

Chưa activate.
