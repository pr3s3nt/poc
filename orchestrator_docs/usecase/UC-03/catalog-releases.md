---
id: UC-03-CATALOG-RELEASES
artifact: specification-extension
status: current
last_reviewed: 2026-10-30
---

# Versioned catalog releases — issue #28

CR-01. PE/Admin manages Organization catalog sources and refreshes discovery.
Source errors expose safe codes and observation times, never transport secrets.
Periodic discovery and manual refresh have the same validation/publication path.

CR-02. Discovery verifies tag ancestry against the observed tracked branch tip,
inspects every manifest component and atomically publishes a complete immutable
release. A moved observed tag conflicts; no existing pins are overwritten.
Missing tags, repository failure or new publication never alter Environment intent.

CR-03. Application Environment settings display applied and desired releases,
available releases, component differences and blockers. Existing application edit
permissions govern desired selection; deploy permissions remain unchanged.
Saving only updates desired state with CAS and operation guards.

CR-04. Preview resolves every component from the selected release and platform
bindings. It preserves matching, Organization isolation, dependencies, user
configuration and resource identities. Authored Definitions participate in matching as specified below.
Unsupported destructive changes and incompatible inputs block before mutation.
Selection, bindings, account or draft changes invalidate the Preview.

CR-05. Deploy uses pinned cache without resolving remote refs. Only complete
success advances applied release. Partial success retains component outcomes,
acknowledged state and provenance; failure never claims a complete upgrade.
Existing operation/fencing, unknown-outcome and ownership guards govern recovery.
No automatic deploy, downgrade or blind rollback is introduced.

CR-06. Environment release selection remains available for existing release-based
configuration. Environments using directly authored Definitions do not require
an Environment Catalog selection. Module pins are stored in each Definition;
refreshing a Catalog or selecting another Environment release never upgrades an
authored Definition implicitly. Referenced artifacts remain available for cleanup.

CR-07. PE/Admin can inspect immutable release components in Catalog and create
or edit Resource Definitions and Rendering Templates through the existing forms.
Git, uploaded ZIP and registered module sources remain available.

## Definition authoring and Catalog modules

CM-01. Restore Definition list/Create/Edit, typed variables, account/placement,
input/output mapping, companion resources, criteria and server matching preview.
Save uses the existing strict validation and version CAS. It does not deploy.

CM-02. Terraform module source adds Catalog. PE/Admin selects an Organization
Catalog source registered in Console, an explicit published release, then a
verified terraform-module component serving the selected Resource Type. Load,
empty, incompatible, error/retry and selection changes must be explicit. Never
choose the newest release automatically. Changing source/type/release invalidates
the previous module choice; late responses cannot restore it.

CM-03. Resolve the selected component through its immutable verified bundle ID,
validate content hash/Resource Type and reuse existing capability schemas. Save
stores the exact execution pin plus source/release/component provenance. Edit
restores that selection; a newer release does not alter it. Module defaults,
provider bindings, account validation and secret boundaries remain unchanged.
Module-only Catalog releases are valid; a Definition document is optional.

CM-04. Preview, Deploy, matching preview and metadata use the same effective
Definitions: directly authored Organization Definitions plus snapshots of an
optional selected Environment release. An authored Definition replaces a snapshot
with the same key; other keys follow normal specificity/tie rules. This preserves
existing release-based applications while making form edits effective. Without
an Environment selection, use authored Definitions. Invalid selected releases
still fail closed. Original execution/cleanup pins remain authoritative.

## Realization

Console → authorized catalog application service → source/release repository.
Discovery worker → Organization Git resolver → hardened Git snapshot → manifest
parser → existing module/render inspectors → atomic verified-release publication.
Environment selection → CAS desired binding → Preview release resolution →
existing planner → fenced execution → per-component outcome → all-success applied
binding. Catalog supplies verified reusable artifacts; authored Definitions configure matching.

See [ADR-031](../../architecture/decisions/ADR-031-versioned-catalog.md) and
[manifest contract](../../architecture/contracts/catalog-releases.md).

[Sequence](catalog-releases-sequence.puml) ([PNG](catalog-releases-sequence.png))
and [UI states](ui/catalog-releases.md).
