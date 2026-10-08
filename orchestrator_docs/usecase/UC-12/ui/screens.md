---
id: UC-12-UI-SCREENS
artifact: use-case-ui-screens
status: current
last_reviewed: 2026-09-24
related: UC-12
---

# UC-12 UI screens

## Application Settings → Variables & Secrets

```text
Payment / Settings                                      Pending changes
Variables & Secrets

[ Staging  ] [ Production ]

Environment variables                                   [ + Add variable ]
Name                 Value                Used by           Actions
LOG_LEVEL            info                 backend, worker   Edit · Rename · Delete

Secrets                                                 [ + Add secret ]
Name                 Status               Used by           Actions
API_TOKEN            Configured           backend           Update · Rename · Delete
```

Only the selected Environment appears as a tab. Switching Environment changes
both section lists, not just displayed values. A key may be absent in one
Environment. The `Used by` column links to workloads in that Environment and
supports the impact warning.

## Environment variables section

Add/Edit asks for key name and a plain variable value. The value is visible
and editable. Rename asks for a new key name; Delete asks for confirmation.
Changes show a pending label and a path to Environment-scoped Preview changes.

## Secrets section

Add accepts key name and secret value. Existing rows show only `Configured` or
`Pending change`, never the value. Update asks for a new value in a concealed
input; leaving it empty does not reveal the previous value. Rename/Delete use
the same impact-warning flow without displaying secret material.

## Impact warning

For rename/delete of a referenced key, the confirmation names the selected
Environment and lists affected workloads. It states that references will not
be updated automatically and Preview will reject unresolved references.
`Continue` is available; the warning is not a hard block. Value update shows
affected workloads and explains that runtime changes only after Preview and
Deploy.

## Secret Store Connection

Selected Environment Settings displays the chosen store and editable READY choices.
Unset state allows ordinary Variable edits, but Secret Add asks to select a store.
Change-store action shows copying/verification, success/pending-deploy or safe error.
No raw Secret, store token or backup content appears in the UI.
