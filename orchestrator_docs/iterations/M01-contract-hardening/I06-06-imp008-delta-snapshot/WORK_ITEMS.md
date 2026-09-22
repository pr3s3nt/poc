---
id: I06-06-WORK
artifact: iteration-work-items
status: historical
last_reviewed: 2026-09-22
related: I06-06
---

# I06-06 work items

## Implementation order

1. Add contract tests for canonical shape and `base + delta = candidate`.
2. Add domain value/entity types and `DeltaBuilder.BuildHumanitecDelta`.
3. Implement module add/remove/update and shared relative patch application.
4. Implement deterministic object/array diff including tail remove order and
   append `/-`.
5. Add Snapshot repository/store serialization and Deployment association.
6. Wire planning/deployment/query paths; remove flat whole-document Delta path.
7. Run backend regression and update implementation-owned documentation.

## Required tests

- Module add, update and remove.
- Shared add/update/remove and conflict preservation.
- Nested object and array diff; deterministic operation order.
- No-op document `{}`.
- Snapshot save/load and one-Deployment association.
- Candidate invariant and plan-hash determinism.

## Handoff checklist

- [x] No D05 API/lifecycle implemented.
- [x] Canonical design files were not changed to fit code.
- [x] All old flat-Delta call sites were searched and handled.
- [x] IMP-008 removed only after full backend regression passes.
- [x] Dated verification record links exact commands/results.
