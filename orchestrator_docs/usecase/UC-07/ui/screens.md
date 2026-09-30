---
id: UC-07-UI-SCREENS
artifact: use-case-ui-screens
status: current
last_reviewed: 2026-09-30
related: UC-07, UC-16
---

# UC-07 screens

Reuse [UC-16 screens](../../UC-16/ui/screens.md), not a duplicate workload
editor. Application home visibly identifies the selected Environment. Edit
loads current/pending Score and Save creates pending change only. Delete
confirmation names workload and Environment, explains deferred execution and
allows cancel. Pending deletion offers Undo before deployment.

Preview panel shows update/remove target, resource changes and explicit Deploy
control. Completion shows per-workload result with a linkable Deployment ID
where available and refreshes list/history. Removed workloads disappear only
after the successful current Set commit. Other workloads remain visible and
unchanged. Failed/partial runs retain unfinished changes and explain that a
fresh Preview is required; no automatic rollback or secret output rendering.
