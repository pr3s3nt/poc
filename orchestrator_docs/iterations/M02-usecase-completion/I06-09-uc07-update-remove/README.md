---
id: I06-09
artifact: iteration-plan
status: deferred
last_reviewed: 2026-09-22
related: UC-07, IMP-004
---

# I06-09 — Complete UC-07 update and remove

## Objective

Hoàn thiện application/API/UI flow update/remove trên planner đã harden, giữ
before-state, shared ownership và current-set transaction invariants.

## In scope

- Update/remove commands, handlers và Web Console flows.
- Before Score deep-equality, scoped Delta Snapshot và Candidate Set.
- Resource provisioning cho desired graph; apply/update hoặc delete workload.
- Mark unreferenced không destroy; final optimistic current-set commit.
- Backend/frontend/integration tests và implementation-doc updates.

## Out of scope

- Automatic resource destruction, rollback, concurrent deployment recovery.
- Incremental deployment mode hoặc D05 external lifecycle.

## Exit criteria

- Update và remove main/variant flows của UC-07 chạy end-to-end.
- Shared resource còn consumer được giữ; unreferenced không bị destroy.
- Current set chỉ commit sau runtime success; conflicts fail trước side effect.
- Validation suites pass và IMP-004 được xóa.

## First next action

Viết application-service tests cho before mismatch, shared conflict,
last-reference và preserve-other-workload trước khi wire API/UI.

## Outcome

Chưa thực hiện.
