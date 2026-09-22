---
id: I06-10-WORK
artifact: iteration-work-items
status: deferred
last_reviewed: 2026-09-22
related: I06-10
---

# I06-10 work items

## Implementation order

1. Reconcile UC-01..04 traceability against current seed/domain/store code.
2. Implement application services and repository behavior in dependency order.
3. Add HTTP contracts with validation and stable identifiers.
4. Add Web Console management flows and complete UI states.
5. Add planner integration proving newly registered catalog/connections work.
6. Run backend/frontend validations and update implementation docs/evidence.

## Handoff checklist

- [ ] No requirement is inferred only from current seed shape.
- [ ] Resource Definition registration enforces at least one criterion.
- [ ] Connection persists only opaque secret references.
- [ ] Newly created data is usable without process restart.
- [ ] IMP-002 removed only when all four happy paths are delivered.
