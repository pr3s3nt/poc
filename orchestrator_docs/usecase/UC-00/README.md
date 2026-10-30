---
id: UC-00-CONTEXT
artifact: use-case-context
status: current
last_reviewed: 2026-10-29
---

# UC-00 context — Sign In

## Delivery state

Implemented for local/test: fixed internal account persistence, opaque
HttpOnly-cookie sessions, session restore, sign-out and Web Console sign-in are
available. Production profile does not seed the fixed account.

User self-service password change (revokes every existing session), Admin user
management (list/create Organization accounts, backend-enforced), Admin
password reset (sets a new password for an Organization account and revokes its
sessions) and the `-bootstrap-admin` first-Admin CLI are implemented. The
bootstrap password comes only from an owner-supplied file; no default Admin
password exists. Every password input offers a show/hide plain-text toggle.

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
