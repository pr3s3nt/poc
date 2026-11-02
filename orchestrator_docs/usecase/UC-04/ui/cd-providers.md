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

## Platform-managed applications — ADR-033

The platform form configures a reusable Fleet connection and GitHub owner,
private credentials and generated-repository prefix. No concrete application
repository/branch/root fields are authored. Cluster discovery fills exact Fleet
identities and pairs the workload account with the selected management cluster.
Technical Fleet namespaces, reader Secret, TLS and host URLs live in Advanced.
After Verify reaches READY, the PE/Admin explicitly chooses Use by default.
Display the default badge and preserve CAS/reload/unknown-outcome handling.
Default changes apply to new Applications only. GitHub is the first host adapter.

## Shared platform Git connection

CD setup reuses Organization-scoped Platform Settings Git access by default. Backend resolves the current credential on each new host/publisher operation without exposing or copying tokens into browser forms. Scope must cover the configured repository owner. Credential rotation applies to subsequent operations; operators re-verify CD after rotation. Separate credentials remain an explicit override. Account selection automatically performs read-only Fleet discovery with a retry action and visible cluster summary. Verification reports bounded actionable reasons for missing reader Secret, namespace or ServiceAccount; setup does not create cluster resources. READY bindings retain the explicit Use by default action.

Discovery displays an in-progress status and actionable failure beside the retry
button inside the form. A backend discovery failure must not appear solely above
the registered-bindings list, outside the current form viewport.

Shared CD Git derives repository ownership from the explicit account/group URL scope in Platform Settings (for example `https://github.com/my-team`). Host-only and repository-specific scopes do not define an eligible owner. Settings displays the derived destination with guidance; CD hides owner authoring for shared Git and shows the inherited destination. Registration resolves this value server-side before validation, overriding any submitted owner. Authentication username is never assumed to be the owner. Existing binding versions retain their pinned owner; changed destinations apply when registering a new version. No separate owner column or schema migration is needed.
