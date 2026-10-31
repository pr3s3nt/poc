---
id: IMP-CATALOG-RELEASES
artifact: implementation-map
status: current
last_reviewed: 2026-10-30
---

# Issue #28 — catalog releases implementation

[Specification](../usecase/UC-03/catalog-releases.md),
[ADR-031](../architecture/decisions/ADR-031-versioned-catalog.md) and
[manifest contract](../architecture/contracts/catalog-releases.md) define the
implementation target. They are not acceptance evidence.

## Implemented

- Strict manifest/Definition parsers and content/hash-based component differences.
- Hardened authenticated branch/tag discovery, annotated tag peeling, ancestry,
  conflict retention, SemVer ordering and periodic/manual refresh.
- Existing module/render inspectors, normal Definition validation, immutable
  cache preparation, unchanged-component pin reuse and atomic release publication.
- Memory/JSON and PostgreSQL source/release/Environment persistence; immutable
  migration 004 with complete version-4 reference and least-privilege grants.
- Release-only Definition and workload-template resolution; desired/applied
  binding snapshots, Environment CAS/busy/fencing and Preview token pins.
- Resource acknowledgement supports a release Definition absent from the global
  registry while preserving logical/state identity and original cleanup artifacts.
- Pending and single-workload paths block unverified database/storage updates;
  complete batch success activates applied release, partial results retain
  acknowledged deployment plans/outcomes and old applied selection.
- Platform Catalog and Environment Catalog UI with VI/EN labels, safe errors,
  usage, component differences/blockers and existing Preview/Deploy controls.

## Evidence and limits

[Verification](../verification/2026-10-30-catalog-releases.md) records local checks
and browser fixtures. [Live scenario](../../e2e/scenarios/catalog-upgrade/README.md)
requires separately authorized, prepared Environments and has not been run on a
retained cluster by this task. No staging database or cloud/cluster was upgraded.

The initial data migration boundary is deliberately conservative: existing
PostgreSQL/MySQL/Redis and PVC-capability resources must have provable RETAIN
intent during a catalog upgrade. There is no published database/storage upgrade
capability, automatic deploy/downgrade, webhook, cross-source switch, release
purge, blind rollback or expanded Terraform recovery/orphan adoption.

Source diagnostics currently summarize a refresh with a safe code and timestamp;
ready release tags/commits and usage are listed. Detailed per-component Git
validation diagnostics are not returned. Source identity is frozen after tag
observation. The registry retains older referenced releases and caches; missing
cache/engine artifacts fail closed rather than fetching at execution time.
