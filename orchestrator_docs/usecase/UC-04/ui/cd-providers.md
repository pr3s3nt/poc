---
id: UC-04-UI-CD-PROVIDERS
artifact: use-case-ui-design
status: current
last_reviewed: 2026-10-29
related: [ADR-030, UC-04]
---

# Cấu hình CD

PE/Admin platform page reuses AppShell, Secret Stores/Driver Accounts list and
form styling, shared Button/Dialog, safe errors and VI/EN catalogs. No redesign.

List: name, provider, version, verification status/time; actions View, Edit as new
version, Verify. Empty state offers Add. GET/reload never retries a mutation.

Create/edit form: name, installed provider, management account. Fleet fields:
controller namespace/name, agent namespace, workspace, registered target cluster
(select discovered name/UID), workload target account; generated repo mode or
existing GitRepo. Managed mode asks HTTPS repository, branch, generated root,
reader Secret reference, release namespace and deployment ServiceAccount. Existing
mode selects a discovered GitRepo and assigned subtree, shows its read-only
publication identity; no implicit takeover or edit of the preexisting controller.
Optional TLS CA and write-only Git username/token are under connection details.
No JSON/YAML input or credential readback. Provider schema supplies typed fields
and options so another adapter does not add planner/deployment branches.

Save persists UNVERIFIED configuration. Verify performs read-only network checks;
READY means eligible, not deployed. Editing creates a new immutable version with
an explicit credential-retention/replacement choice, leaving applied consumers
pinned. Details show safe config and desired/applied Environment usage links.

Loading, unavailable provider/account/discovery, empty, validation, stale,
verification failure, busy and unknown-save outcomes have distinct states. Clear
credential inputs after submission; unknown creation disables another POST until
reloaded by request identity. No raw provider errors, hidden secret refs or tokens.

Browser acceptance uses ordinary forms and real adapters. Fixture screenshots
only prove UI behavior, never Fleet health or application deployment.
