---
id: UI-ARCHITECTURE-INDEX
artifact: shared-ui-design
status: current
last_reviewed: 2026-09-23
---

# Shared Web Console UI

Shared UI design owns the application shell and reusable interaction patterns.
It does not own a use-case screen: each use case owns its screens under
`usecase/UC-xx/ui/`.

## Shared shell

- A signed-in page has a persistent left sidebar with `Applications` and
  `Deployments` navigation.
- The footer of the sidebar shows authenticated User name and role, plus a
  sign-out action.
- Main content owns the page title, primary action and use-case-specific state.
- The sign-in page is intentionally outside the authenticated shell.

## Shared interaction rules

- Primary actions are blue; destructive actions require a clear confirmation in
  the owning use case.
- `staging` and `production` are visible Environment tabs, not a generic select.
- Status is expressed through a readable text label and color, never color alone.
- Every screen declares loading, empty, validation, API-error and success states
  in its use-case UI design.

## Use-case UI designs

- [UC-00 sign in](../../usecase/UC-00/ui/README.md)
- [UC-01 application onboarding and home](../../usecase/UC-01/ui/README.md)
- [UC-12 Application Variables & Secrets](../../usecase/UC-12/ui/README.md)
- [UC-16 workload configuration](../../usecase/UC-16/ui/README.md)
