---
id: UC-01-UI-SCREENS
artifact: use-case-ui-screens
status: current
last_reviewed: 2026-10-07
---

# UC-01 UI screens

Applications home lists name, hostname and separate staging/production summaries.
Create Application contains only Application name and Subdomain with URL preview.
Creation does not require Connection/default readiness.

Application home has Staging/Production tabs; target label belongs to the selected
Environment, never a header implying one target for both. Display `Connection not
configured` with Settings action for UNCONFIGURED. Workload draft/config editing
remains available; Preview/Deploy require configured target. Show selected key,
profile, AWS region and locked status for configured Environment. Recent deployments link to the existing UC-09 history/detail; persisted plans
keep the deployment target pinned rather than deriving it from defaults.

Environment Settings integrates a `Deployment connection` section in the existing
Environment-scoped Settings page alongside UC-12 variables/secrets. Scope is visible
in heading/tab. UNCONFIGURED: safe READY dropdown, explicit select, and `Set
connection` button with explanation `You can set this connection only once. It
cannot be changed after saving.` Choices may mark default but do not preselect or
auto-save it. Selection may change freely before Save. After successful set show
read-only target; no Change, Reset, Unset or Replace action. Other Environment stays
unconfigured until explicitly set. No extra confirmation dialog is required.
