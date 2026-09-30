---
id: I06-09
artifact: iteration-plan
status: historical
last_reviewed: 2026-09-30
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

## Outcome

Completed 2026-09-30 through the existing UC-16 pending flow: final-transaction
UNREFERENCED marking, final Service-reference validation, scoped strict HTTP,
safe failures and stale/busy/reload UI states. Memory/PostgreSQL and frontend
checks passed; UI-only human-paced recording reviewed and published. IMP-004
removed. See [evidence](../../../verification/2026-09-30-uc07-update-remove-local.md).
No live cluster/cloud rerun or Application-wide cleanup was performed.
Next: [I06-10](../I06-10-uc01-04-management/README.md).
