---
id: UC-00-CONTEXT
artifact: use-case-context
status: current
last_reviewed: 2026-09-23
---

# UC-00 context — Sign In

## Delivery state

Designed; no authentication code, internal-account persistence, session store or
Web Console sign-in flow exists yet. The executable baseline remains unauthenticated.

## Read in this order

1. [Specification](specification.md)
2. [Realization](realization.md)
3. [Sequence](sequence.puml)
4. [VOPC](vopc.puml)
5. [Current state](../../CURRENT_STATE.md)

## Planned implementation entry points

- `backend/internal/application/authentication`
- `backend/internal/domain/identity`
- `backend/internal/delivery/http`
- `backend/internal/ports/persistence`
- `frontend/src`
