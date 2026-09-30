---
id: UC-05-CONTEXT
artifact: use-case-context
status: current
last_reviewed: 2026-09-30
---

# UC-05 context — Validate and preview Score changes

## Delivery state

Standalone Score Preview service, authenticated scoped API and Web Console are
implemented, alongside the separate pending-change Preview. Both Preview and
direct Deploy use the shared planner; standalone Preview and Deploy load one
consistent planning snapshot. Preview remains read-only and its public view
excludes sensitive planning details.

## Read in this order

1. [Specification](specification.md)
2. [Realization](realization.md)
3. [Sequence](sequence.puml)
4. [VOPC](vopc.puml)
5. [Planner reference](../../implementation/uc06-planner-reference.md)
6. [UI screens](ui/screens.md), [states](ui/states.md) and [API mapping](ui/api-mapping.md)

## Implementation entry points

- [`backend/internal/planning`](../../../backend/internal/planning/)
- [`backend/internal/application/preview`](../../../backend/internal/application/preview/)
- [`shared snapshot loader`](../../../backend/internal/application/deployment/snapshot.go)
- [`HTTP boundary`](../../../backend/internal/delivery/http/score_preview.go)
- [`Web Console`](../../../frontend/src/features/preview/)
- [`backend/test/conformance`](../../../backend/test/conformance/)
