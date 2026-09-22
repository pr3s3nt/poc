---
id: I06-08
artifact: iteration-plan
status: deferred
last_reviewed: 2026-09-22
related: UC-05, IMP-003
---

# I06-08 — Complete UC-05 preview

## Objective

Hoàn thiện read-only Preview operation/API/Web Console trên cùng planning
pipeline đã được M01 harden, không tạo runtime side effect.

## In scope

- `PreviewService.PreviewDeployment`, HTTP contract và Web Console Preview.
- Load/version planning snapshot; trả Delta Snapshot document, Candidate Set,
  graph, matches, batches và classification.
- Loading/empty/validation/success/error UI states và no-mutation tests.
- Traceability/status/code-map/evidence updates.

## Out of scope

- Terraform plan thật, cost/policy/approval hoặc lưu nhiều preview.
- Deploy, update/remove hoặc D05 standalone Delta API.

## Exit criteria

- Preview và deploy dùng cùng planning service/input semantics.
- Executor/deployer/store mutation không xảy ra trong Preview.
- Backend/frontend tests và validation pass; IMP-003 được xóa.

## First next action

Viết no-side-effect application-service tests trước khi wire HTTP/UI.

## Outcome

Chưa thực hiện.
