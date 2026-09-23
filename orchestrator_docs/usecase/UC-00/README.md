---
id: UC-00-CONTEXT
artifact: use-case-context
status: current
last_reviewed: 2026-09-23
---

# UC-00 context — Sign In

## Delivery state

Implemented for local/test: fixed internal account persistence, opaque
HttpOnly-cookie sessions, session restore, sign-out and Web Console sign-in are
available. Production profile does not seed the fixed account.

## Read in this order

1. [Specification](specification.md)
2. [Realization](realization.md)
3. [Sequence](sequence.puml)
4. [VOPC](vopc.puml)
5. [UI design](ui/README.md)
6. [Current state](../../CURRENT_STATE.md)

## Implementation entry points

- `backend/internal/application/authentication`
- `backend/internal/domain/identity`
- `backend/internal/delivery/http`
- `backend/internal/ports/persistence`
- `frontend/src/features/auth`
