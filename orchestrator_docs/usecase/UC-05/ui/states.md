---
id: UC-05-UI-STATES
artifact: use-case-ui-states
status: current
last_reviewed: 2026-09-30
related: UC-05
---

# UC-05 states

| State | Visible behavior |
|---|---|
| Ready / empty input | Instructions and labeled editors; no fabricated preview. |
| Invalid input | Actionable parse/required-field/Score error; input retained. |
| Previewing | Loading label; duplicate submit disabled. |
| Success | All planning artifacts and version context; no execution controls. |
| No change | Explicit no-change notice with valid planning details. |
| API failure | Error with retry available; no stale success shown. |
| Session expired | Existing UC-00 sign-in flow, no cross-scope artifacts. |
| Input/scope changed | Clear old preview; ignore late obsolete responses. |

Keyboard navigation and accessible labels apply to all inputs/actions.
