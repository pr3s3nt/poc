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
configuration and resource identities. There is no global Definition fallback.
Unsupported destructive changes and incompatible inputs block before mutation.
Selection, bindings, account or draft changes invalidate the Preview.

CR-05. Deploy uses pinned cache without resolving remote refs. Only complete
success advances applied release. Partial success retains component outcomes,
acknowledged state and provenance; failure never claims a complete upgrade.
Existing operation/fencing, unknown-outcome and ownership guards govern recovery.
No automatic deploy, downgrade or blind rollback is introduced.

CR-06. Legacy Environments with no release retain existing behavior. Adoption
requires explicit compatibility Preview, never inference from a module name.
Referenced releases remain available for restart, retry and original-version
cleanup. Catalog versions do not imply database engine upgrades/data migrations.

## Realization

Console → authorized catalog application service → source/release repository.
Discovery worker → Organization Git resolver → hardened Git snapshot → manifest
parser → existing module/render inspectors → atomic verified-release publication.
Environment selection → CAS desired binding → Preview release resolution →
existing planner → fenced execution → per-component outcome → all-success applied
binding. Global Definition management remains independent.

See [ADR-031](../../architecture/decisions/ADR-031-versioned-catalog.md) and
[manifest contract](../../architecture/contracts/catalog-releases.md).

[Sequence](catalog-releases-sequence.puml) ([PNG](catalog-releases-sequence.png))
and [UI states](ui/catalog-releases.md).
