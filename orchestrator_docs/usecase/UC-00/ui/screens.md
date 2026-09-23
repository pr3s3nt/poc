---
id: UC-00-UI-SCREENS
artifact: use-case-ui-screens
status: current
last_reviewed: 2026-09-23
related: UC-00
---

# UC-00 UI screens

## Sign in

The unauthenticated page is split into a dark brand panel and a calm form panel.
The visual separation makes sign-in feel like product entry, not a technical
admin form.

| Area | Content and behavior |
|---|---|
| Brand panel | `Orchestrator`, `Internal developer platform`, concise product message and low-contrast infrastructure line art. |
| Form heading | `Welcome back` and `Sign in to create and deploy applications.` |
| Inputs | Required `Username` and `Password`; password is never retained in browser state after submit. |
| Primary action | `Sign in`; disabled only while the request is in flight. |
| Helper text | `Use your local test account.` No self-registration, password-reset or social-login affordance. |

On success, redirect to `/ui/applications`. Do not show a separate success page.
