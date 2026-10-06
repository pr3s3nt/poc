---
id: UC-04-UI-STATES
artifact: use-case-ui-states
status: current
last_reviewed: 2026-10-06
---

# UC-04 UI states

- List loading/empty/ready/error: READY list; error has Retry, not fake empty.
- Editing: name and upload/paste kubeconfig; context/summary cleared on change.
- Inspecting: disable repeated inspection; stale responses cannot restore an
  old document's context/summary. No secret or Connection is persisted.
- Inspected: one context auto-selected; multiple require selection; summary
  matches chosen context. Changed document requires fresh inspection.
- Unsupported/invalid: safe actionable parser/auth guidance, retain form.
- Verifying/saving: lock submitted form and duplicate submit; API/RBAC then store.
- Registration error: preserve form; safe connectivity/permission/store message.
- Registered: notify committed success, clear credentials/file/form, reload list.
- Registered/list error: keep success distinct from reload failure, offer Retry
  list without repeating registration. Unknown network outcome does not claim
  registration failure as proof that nothing was saved; do not auto-resubmit.
