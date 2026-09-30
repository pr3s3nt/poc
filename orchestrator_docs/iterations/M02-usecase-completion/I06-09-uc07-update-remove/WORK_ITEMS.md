---
id: I06-09-WORK
artifact: iteration-work-items
status: historical
last_reviewed: 2026-09-30
related: I06-09
---

# I06-09 work items

## Implementation order

1. Add UC-07 application-service tests around canonical planner invariants.
2. Implement update/remove command orchestration and state transitions.
3. Wire workload apply/delete, unreferenced marking and final transaction.
4. Add HTTP contracts and Web Console update/remove states.
5. Run backend/frontend/local PostgreSQL regression, record and review UI-only
   update/remove video, then update docs and commit/push milestone. Kind/cloud
   runs require explicit external verification scope; not part of this local run.

## Handoff checklist

- [x] Before mismatch/shared conflict causes no external side effect.
- [x] Remove does not implicitly destroy unreferenced resources.
- [x] Other workload/shared contributions remain unchanged.
- [x] Candidate becomes current only after successful runtime action.
- [x] IMP-004 removed only after API/UI/system tests pass.
