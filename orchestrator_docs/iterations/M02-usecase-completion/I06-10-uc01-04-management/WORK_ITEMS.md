---
id: I06-10-WORK
artifact: iteration-work-items
status: deferred
last_reviewed: 2026-09-30
related: I06-10
---

# I06-10 work items

## Implementation order

1. Reconcile remaining UC-02..04 traceability against current code and tests.
2. Complete application services and repository behavior for remaining
   Resource Type, Resource Definition and Connection gaps.
3. Add HTTP contracts with validation and stable identifiers.
4. Add Web Console management flows and complete UI states.
5. Add planner integration proving newly registered catalog/connections work.
6. Run backend/frontend validations and update implementation docs/evidence.

## Handoff checklist

- [ ] No requirement is inferred only from current seed shape.
- [ ] Resource Definition registration enforces at least one criterion.
- [ ] Connection persists only opaque secret references.
- [ ] Newly created data is usable without process restart.
- [ ] I00-00 UC-01 behavior remains covered by regression tests.
- [ ] IMP-002 removed only when all remaining UC-02..04 gaps are delivered.
