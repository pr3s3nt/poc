---
id: UC-00-UI-STATES
artifact: use-case-ui-states
status: current
last_reviewed: 2026-09-23
related: UC-00
---

# UC-00 UI states

| State | UI behavior |
|---|---|
| Initial | Empty username/password inputs; primary action enabled. |
| Client validation | Inline required-field message; no API request. |
| Submitting | Button reads `Signing in…`; both inputs and submit action are disabled. |
| Invalid credentials | Generic form-level message: `Username or password is incorrect.` Never reveal which field failed. |
| Network/API error | Form-level message with retryable `Sign in` action; do not clear username. |
| Session expired | Redirect to Sign in with one info callout: `Your session ended. Sign in again to continue.` |
| Success | Redirect directly to UC-01 Applications home. |
