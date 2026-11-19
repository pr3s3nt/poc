---
id: VERIFICATION-SCORE-CLEANUP-2026-11-18
artifact: verification-record
status: current
last_reviewed: 2026-11-18
---

# Score trial cleanup — issue #83

The user requested removal of resources created during this session, superseding
retention of the live trial in the [main handoff](2026-11-17-score-main.md).
This was operational cleanup; no product code, schema or authentication settings
were changed.

## Removed and verified

- Product Application Deletion succeeded for all three remaining trial
  Applications: the Keycloak trial, the manual Score import trial and the saved
  draft trial. The feature installation had zero visible/non-deleted Applications
  before shutdown. Product deletion also purged the Keycloak trial's two Secrets.
- Removed trial namespaces, workloads, database storage, routes and delivery
  controllers. Final inventory found no owned namespace, persistent volume or
  GitRepo controller remaining, including the earlier acceptance deployment.
- Keycloak deletion correctly stopped on setup-owned Role/RoleBinding objects.
  Their exact rules, subjects, labels and namespace identity were verified against
  bootstrap configuration; UID/resourceVersion preconditions fenced their removal.
  Retrying product deletion then succeeded. No cleanup guard was bypassed.
- Removed one legacy GitRepo controller left by the manual import trial only
  after verifying the deleted Application, absent namespaces, repository identity,
  ownership marker, labels and paths. Its deletion used UID/resourceVersion
  preconditions and normal foreground deletion; no finalizer was stripped.
- Removed nine dedicated containers (six Compose services and three helpers),
  fourteen project volumes, two dedicated networks, two feature image tags and
  three dedicated relay/forward processes. Removed obsolete trial password and
  workload kubeconfig files plus eight temporary source/dependency archives.

## Preserved and checked

The current main Console belongs to a newer SSO rollout from another task.
Backend/frontend remained ready on `main-57ecc971`; PostgreSQL and Vault remained
ready. Main had zero Secret Store references to the removed feature Vault.
Shared Gateway/Fleet resources, other Applications, main storage and the main
Console forward were preserved.

A real browser opened the current Tailscale Console route and rendered its SSO
login control. The same-origin authentication configuration API returned HTTP 200.
This checks public login availability; an authenticated Application inventory
check was not repeated after the newer authentication rollout. The exact private
URL, timestamp and screenshot are retained locally, outside Git.

Private audit evidence and backups were retained, including a validated
post-deletion feature database archive and pre-upgrade main database/storage
backups. The three generated delivery repositories remain private and retained
under the product's default history-retention policy. Repository IDs and ownership
markers were verified; no remote repository was deleted.

Historical trial URLs no longer provide a running Keycloak or feature Console.
The main Console and its access path remain running. A retirement marker records
that the removed feature stack should not be restarted as an active trial.

## Validation and evidence

Private receipts include product deletion results, empty Application inventory,
final Kubernetes absence checks, fenced RBAC/controller cleanup, Docker teardown,
relay termination, repository ownership checks and main browser evidence. They
are held in the local issue #83 state directory; no credentials, private URLs or
infrastructure addresses are published here.

Documentation validation passed (656 Markdown files, 527 artifact IDs), and
`git diff --check` passed. Local PlantUML syntax checking was skipped because the
tool is unavailable; this record changes no diagrams. Product unit/full suites
were not rerun: this cleanup changed no product code.
