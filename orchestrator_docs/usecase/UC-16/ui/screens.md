---
id: UC-16-UI-SCREENS
artifact: use-case-ui-screens
status: current
last_reviewed: 2026-09-24
related: UC-16
---

# UC-16 UI screens

## Application home — selected Environment

The existing UC-01 Application home keeps visible Staging/Production tabs.
The selected Environment shows its workload list, pending-change labels,
`+ Add workload`, Edit/Delete actions and `Preview changes`. A link to the
Application-level Variables & Secrets settings opens UC-12. Preview is scoped
to the selected Environment, never to both at once.

## Add/Edit workload

One page shows the selected Environment and a two-way input choice: **Enter on
form** (default) or **Import Score**. The form groups basic information,
containers and compute resources, Service/health checks, resource dependencies,
environment variables, then secrets. Save/Cancel remain visible at the bottom.
Edit uses the same form and shows the current workload name.

| Group | Input |
|---|---|
| Environment variables | Each row has container variable name, source type and source picker. Types: Application variable (UC-12), non-secret resource output, or another workload's Service/cổng in this Environment. No direct-value field. |
| Secrets | Each row has container secret name, source type and source picker. Types: Application secret (UC-12) or secret resource output. Values are never shown. |
| Resource output picker | Select declared dependency, then eligible output from its contract; secret classification determines which group can use it. |
| Workload Service picker | Select a non-deleted workload in the same Environment that declares a Service, then select a declared port; show the resulting internal endpoint as read-only explanation. |

The variable and secret groups link to Application Variables & Secrets (UC-12).
Returning from settings preserves the unsaved workload form and refreshes the
available reference names.

## Import Score

Choose one Score file for one workload. After parsing, show the same form for
review, including resolved reference types and validation messages. Reject
direct literal values in container variable/secret bindings. Import does not
save or deploy by itself.

## Delete confirmation

The confirmation names workload and Environment and explains that deletion is
pending until deployment. After confirmation the row reads `Pending deletion`
and offers Undo. It remains visible in Preview changes.

## Pending changes

Saving or marking deletion returns to Application home. A visible pending
indicator leads to UC-05 Preview changes. No Deploy action is silently
performed by this screen.
