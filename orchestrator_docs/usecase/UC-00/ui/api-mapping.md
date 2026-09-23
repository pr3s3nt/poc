---
id: UC-00-UI-API
artifact: use-case-ui-api-mapping
status: current
last_reviewed: 2026-09-23
related: UC-00, OC-00
---

# UC-00 UI API mapping

These are proposed same-origin HTTP contracts for the UI; the canonical service
operation remains [OC-00](../../../architecture/contracts/operation-contracts.md).

| UI action | HTTP contract | Success handling | Failure handling |
|---|---|---|---|
| Submit sign-in | `POST /api/v1/auth/sign-in` with username/password | Backend sets opaque `HttpOnly` cookie; UI redirects to `/ui/applications`. | `401` gives generic invalid-credential state; other failures give API-error state. |
| Restore shell session | `GET /api/v1/auth/session` | Render authenticated shell and User context. | `401` redirects to Sign in. |
| Sign out | `POST /api/v1/auth/sign-out` | Clear local UI context and redirect to Sign in. | Clear local UI context and redirect to Sign in. |
