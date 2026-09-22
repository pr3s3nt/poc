---
id: UC-05-CONTEXT
artifact: use-case-context
status: current
last_reviewed: 2026-09-22
---

# UC-05 context — Validate and preview Score changes

## Delivery state

Planning core is implemented and shared with deployment; preview operation/API
and Web Console experience are not complete. The planner builds the
Humanitec-shaped Delta (I06-06) and preserves Score container requests/limits
in the Candidate Set (I06-07).

## Read in this order

1. [Specification](specification.md)
2. [Realization](realization.md)
3. [Sequence](sequence.puml)
4. [VOPC](vopc.puml)
5. [Planner reference](../../implementation/uc06-planner-reference.md)

## Implementation entry points

- [`backend/internal/planning`](../../../backend/internal/planning/)
- [`backend/test/conformance`](../../../backend/test/conformance/)
