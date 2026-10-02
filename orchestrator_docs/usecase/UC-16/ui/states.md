---
id: UC-16-UI-STATES
artifact: use-case-ui-states
status: current
last_reviewed: 2026-10-02
related: UC-16
---

# UC-16 UI states

| Screen | State | UI behavior |
|---|---|---|
| Application home | Loading/empty | Preserve selected Environment; show list skeleton or `No workloads` and Add action. |
| Application home | Pending | Show added/edited/deleted labels and Preview changes; no runtime-success claim. |
| Application home | API error | Show retry without replacing data with mock workload rows. |
| Form | Loading/new/edit | Show Environment, empty form for Add or existing configuration for Edit. |
| Form | Missing/invalid reference | Identify variable/secret row and source; disable Save until corrected. |
| Form | Missing UC-12 key | Explain the missing key in this Environment; link to Application Variables & Secrets. |
| Form | Invalid Service target | Explain missing target, missing Service/port or target pending deletion; retain other form fields. |
| Form/Preview | Invalid public path | Form requires a valid unique path and declared Service port; Preview rejects duplicate paths across workloads while allowing `/` and `/api`. |
| Preview | No effective change | A draft matching the current module has no affected workload and Deploy is disabled; changed referenced UC-12 key revision remains an update. |
| Preview | Route reconciliation failed | Show route-only pending state and enable `Retry public routes`; no workload restart on retry. |
| Form | Missing/invalid resource input | Identify dependency and required input; retain entered values and prevent invalid save. |
| Form | Secret source | Show name/output classification only; never echo secret value. |
| Import | Parsing/invalid | Show file progress or field-level error; direct literal binding is rejected. |
| Save | Submitting/error/success | Prevent double submit; preserve form on error; return to Application home with pending label on success. |
| Delete | Confirm/cancel/success | Require confirmation; cancel does nothing; success shows pending deletion and Undo. |

## Resource input errors

Save and Score import reject invalid resource params with safe field paths under
UC-16 BR-14. Preserve unsaved editor/import content; do not report a saved draft
or advance displayed draft version after rejection. Backend validation also
covers imported dependencies unused by container bindings.

## Application-key checklist states

| State | Behavior |
|---|---|
| New container | All keys unchecked; show Variables and Secrets separately for the selected Environment. |
| Selected | Same-name binding by default; optional alias input, no value input. |
| Edited/imported | Restore selected keys and all existing aliases, including multiple names for one key. |
| Unselected | Remove only that key's mappings from this container; leave other bindings intact. |
| No keys | Show Settings guidance per group; no automatic selection of future keys. |
| Missing selected key | Keep unavailable reference/name visible; block Save until removed/corrected. |
| Loading/catalog error | Loading or safe error + Retry; do not save using an empty replacement catalog. |
| Duplicate/empty name | Explain container and conflicting name; preserve edits and block Save without submitting. |
| Saving/stale | Freeze checklist, alias and Other sources controls during Save; retain selection and alias edits on failure/conflict. |
| Scope changed | Load only destination Application/Environment/workload state; ignore old import/catalog/save/reload responses. Same-scope retry/reload retains local selection and aliases. |
