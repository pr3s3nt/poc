---
id: UC-12-UI-STATES
artifact: use-case-ui-states
status: current
last_reviewed: 2026-09-24
related: UC-12
---

# UC-12 UI states

| Screen | State | UI behavior |
|---|---|---|
| Settings | Loading | Keep Staging/Production tabs and both section headings visible; show list skeletons. |
| Settings | Empty | Show independent empty states and Add actions for Environment variables and Secrets in the selected Environment. |
| Settings | API error | Show retryable error without substituting mock keys. |
| Key form | Validation | Require a name and value on Add; reject duplicate name in selected scope; retain fields after error. |
| Secret form | Existing key | Show configured status only; never prefill or return existing secret value. |
| Rename/Delete | Referenced key | Warn with affected workload names and selected Environment; allow confirmation without changing workload references. |
| Rename/Delete | Unreferenced key | Confirm scope and action; do not imply immediate runtime change. |
| Save | Submitting/error | Prevent double submit and retain context after failure; do not expose secret values in errors. |
| Save | Pending | Show pending badge, affected workloads and Preview changes for selected Environment. |
| Preview handoff | Missing reference | Explain that user must repair workload bindings in UC-16 before Deploy. |

## Store transfer and concurrency

Unset store blocks Secret write, not Variable write. Store transfer shows progress
and disables duplicate actions. 409 stale version requires reload/review; active
Environment operation temporarily blocks configuration edits. Copy failure leaves
old selection active; successful copy shows pending Preview/Deploy for consumers.
