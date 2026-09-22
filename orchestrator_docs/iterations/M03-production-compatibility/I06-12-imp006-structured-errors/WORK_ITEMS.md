---
id: I06-12-WORK
artifact: iteration-work-items
status: deferred
last_reviewed: 2026-09-22
related: I06-12
---

# I06-12 work items

## Implementation order

1. Resolve D02 and publish the selected error ownership/versioning contract.
2. Add typed planner error model and construction tests by planning phase.
3. Add compatibility/API mapping without leaking sensitive cause strings.
4. Compare all rejected fixtures when that boundary is selected.
5. Update API docs, traceability, current state and verification evidence.

## Handoff checklist

- [ ] No fixture-only field became public API accidentally.
- [ ] Error code/path are deterministic and tested.
- [ ] Secret/provider internals are redacted.
- [ ] IMP-006 status matches the explicitly selected boundary.
