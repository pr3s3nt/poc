---
id: UC-04-UI-SCREENS
artifact: use-case-ui-screens
status: current
last_reviewed: 2026-10-06
---

# UC-04 Connections screen

`Platform / Connections` lists READY Connections owned by the Organization:
name/key, kind, cluster/endpoint and status. Existing unnamed records use key.

Kubernetes registration: Connection name, Upload kubeconfig file or Paste
kubeconfig, inspect action, context selector, selected cluster/endpoint summary,
then **Check and save**. Exactly one context auto-selects; multiple contexts
require a choice. No user-entered key, Cluster ID or endpoint; no host setup.
Switching file/document invalidates prior context/summary and pending results.

Credential textarea supports hiding its content; after success clear it and
file input. Errors retain the submitted form for correction. Never display
credential in summary/list/notification. AWS form and edit/delete are deferred.

## Layout and controls

Keep the registration content in a readable column (approximately 720px maximum),
with consistent field spacing. Present Upload file and Paste content as adjacent
compact choices with clear selected and keyboard-focus states. Radio/checkbox
controls sit beside their labels; text-field width and padding must not apply to
them. Keep the source group accessible and keyboard operable.

Use a file-picker button styled consistently with the console, with selected
filename next to it and an accessible native file input. Place the inspect action
beside or immediately below the file/content area, rather than at the far edge
of the page. Group context selection and the destination summary together below
the source area. Show context, cluster and endpoint as readable labelled values;
long endpoints wrap. The primary Check and save action belongs at the form footer.

Present registered Connections in aligned Name, Type, Destination and Status
columns. Distinguish the friendly name from the secondary key, and use a clear
READY status badge. Do not show empty Kubernetes fields for AWS records. On narrow
screens, stack content or use accessible scrolling without overflowing the page.
Preserve source-switch invalidation, credential masking, single-context automatic
selection, error retention and successful-save clearing behavior.

## Secret stores

Platform navigation adds Secret stores. List displays name/provider/backend and
workload addresses/mount/status. Registration form has name, Vault addresses,
KV/auth mounts, optional trusted CA and concealed token. Check and save verifies
before showing READY; clear token after success. Developers see only safe choices
in Environment Settings, not the privileged registration form.
