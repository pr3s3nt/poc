---
id: I00-00-WORK
artifact: iteration-work-items
status: historical
last_reviewed: 2026-09-30
related: I00-00
---

# I00-00 work items

## Implementation order

1. Review UC-00 targeted design-gate items and create the paired screen-flow:
   sign-in → Applications empty/list → create Application → success context.
2. Define API request/response, loading, validation, unauthorized, error and
   success states for every UC-00/UC-01 screen.
3. Implement and test backend authentication: fixed local/test accounts,
   password hash verification, opaque sessions, middleware and sign-out.
4. Implement and test frontend sign-in, authenticated application shell and
   session-expired/sign-out behavior.
5. Implement and test backend UC-01 atomic Application/default-Environments
   creation and authenticated Organization scoping.
6. Implement and test frontend Application list, empty state, create form and
   `staging`/`production` success context.
7. Run end-to-end onboarding verification, then update traceability, current
   state, code map, deviations and dated evidence.

## Handoff checklist

- [x] No password, raw session token, credential or secret value appears in
  logs, API response, UI state or test snapshot.
- [x] Fixed test accounts are unavailable outside local/test profile.
- [x] UI never submits trusted Organization ID or role; backend derives both
  from authenticated session.
- [x] Create Application has no runtime infrastructure or deploy side effect.
- [x] Every screen has loading, validation, API error and success/empty states.
- [x] Existing deploy/detail UI remains reachable until its replacement flows
  are designed in later use cases.
