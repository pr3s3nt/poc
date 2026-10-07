---
id: UC-16-UI-SCREENS
artifact: use-case-ui-screens
status: current
last_reviewed: 2026-10-07
related: UC-16
---

# UC-16 UI screens

## Application home — selected Environment

The existing UC-01 Application home keeps visible Staging/Production tabs.
The selected Environment shows its workload list, pending-change labels,
`+ Add workload`, Edit/Delete actions and `Preview changes`. A link to the
Application-level Variables & Secrets settings opens UC-12. Preview is scoped
to the selected Environment, never to both at once. The pending Preview/Deploy
panel displays the persisted selected Environment connection key/profile (UC-01 BR-08);
both Environments use this same target.

## Add/Edit workload

One page shows the selected Environment and a two-way input choice: **Enter on
form** (default) or **Import Score**. The form groups basic information,
containers and compute resources, Service/health checks, resource dependencies,
environment variables, then secrets. Save/Cancel remain visible at the bottom.
Edit uses the same form and shows the current workload name.

| Group | Input |
|---|---|
| Application variables | Per-container checklist of UC-12 Variable key names in this Environment. Tick to use the same name; expand "Use a different container name" only for aliases. No value field or repeated Application-source selection. |
| Application secrets | Separate checklist of Secret key names with the same optional alias control. Values are never shown or requested. |
| Other sources | Explicit container name and resource-output or workload-Service picker. No Application variable/secret options in this row editor; Application keys are selected above. |
| Resource output picker | Select declared dependency, then eligible output from its contract; secret classification determines which group can use it. |
| Resource dependency inputs | After selecting Resource Type, show its declared input fields with required indicators and type-appropriate controls. PostgreSQL exposes required `database` and `username`; editing preserves saved params. |
| Workload Service picker | Select a non-deleted workload in the same Environment that declares a Service, then select a declared port; show the resulting internal endpoint as read-only explanation. |
| Public access | Repeatable Public path + Service port rows; for example `/` to frontend and `/api` to backend. Paths must be unique within the Environment, ports declared on their workload. Show the intended shared host without claiming DNS/TLS is ready. |

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

## Existing-key selection behavior

- Render keyboard-accessible labelled checkboxes under Variables and Secrets
  for each container. Newly added containers start unchecked. Display the key
  name and selected container name; do not show catalog values in this picker.
- For `DATABASE_PASSWORD`, a checked key defaults to container name
  `DATABASE_PASSWORD`. Expanding "Use a different container name" reveals an
  input where the user can enter `PGPASSWORD`. Removing the override resets
  the name to `DATABASE_PASSWORD`.
- Edit/import preselect existing keys and reveal differing aliases. Multiple
  names already mapped to the same key remain visible and editable. Unchecking
  removes those mappings from that container only.
- Unavailable selected keys appear explicitly with their original names and
  removal affordance. Empty Variables/Secrets sections explain that keys are
  created in Application Settings; API failure has Retry, not an empty state.
- All selected/added names must be complete and unique within their container.
  Conflicts with Application/resource/Service mappings keep the form and block
  Save; identical names in another container are allowed.
- Freeze key-selection and alias controls during a pending Save. A stale Save
  preserves selection/aliases and uses the existing reload/review flow.
