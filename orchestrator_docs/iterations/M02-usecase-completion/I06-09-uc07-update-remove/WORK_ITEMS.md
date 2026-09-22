---
id: I06-09-WORK
artifact: iteration-work-items
status: deferred
last_reviewed: 2026-09-22
related: I06-09
---

# I06-09 work items

## Implementation order

1. Add UC-07 application-service tests around canonical planner invariants.
2. Implement update/remove command orchestration and state transitions.
3. Wire workload apply/delete, unreferenced marking and final transaction.
4. Add HTTP contracts and Web Console update/remove states.
5. Run backend/frontend/kind regression and update implementation docs.

## Handoff checklist

- [ ] Before mismatch/shared conflict causes no external side effect.
- [ ] Remove does not implicitly destroy unreferenced resources.
- [ ] Other workload/shared contributions remain unchanged.
- [ ] Candidate becomes current only after successful runtime action.
- [ ] IMP-004 removed only after API/UI/system tests pass.
