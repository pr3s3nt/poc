---
id: OPS-CATALOG-RELEASES
artifact: runbook
status: current
last_reviewed: 2026-10-30
---

# Versioned catalog sources and Environment upgrades

Run versioned owner setup first using the
[PostgreSQL procedure](postgres-upgrades.md); keep the module/render cache volume
and existing state/credential keys. No runtime owner credentials or DDL.
This text does not authorize mutation of a retained installation or cluster.

PE/Admin configures Organization Git access through platform settings, then opens
**Catalog**, enters source ID/HTTPS repository/tracked branch and saves. Defaults:
`main`, `v*`, `catalog.yaml`, stable SemVer only. Advanced holds the manifest path,
prerelease opt-in and symbolic account slots (`slot=account-ID`, one per line).
Accounts must already be registered and READY for Definition publication.
Credentials remain in platform Git/account settings, never URLs or repository.

Prepare a [v1 manifest and Definition files](../architecture/contracts/catalog-releases.md).
Terraform/render components live at one repository commit. Tag the commit and
ensure it is in the tracked branch history. **Kiểm tra cập nhật** performs
bounded discovery, full component inspection and complete-release publication;
the worker also checks every 15 minutes. It never deploys an Application.
Moved observed tags conflict permanently for that source; publish a new tag.
To change repository/branch/manifest identity after observation, register a new
source. Remote deletion or outage never removes existing release/cache metadata.

Open Application Settings, select staging or production, then the Catalog panel.
Review applied/desired status, component differences and binding blockers. Save
records desired only under Environment CAS; it does not touch infrastructure.
Open Preview, review resource intents/targets and Deploy using that token.
Selection/config/account changes invalidate stale review. Runtime uses verified
cache; it never resolves a fresh Git ref. Snapshot Definitions are exclusive;
there is no fallback to the global Definition registry for a pinned Environment.
Legacy Environments remain unpinned until explicitly reviewed/adopted.

Complete success advances applied. PARTIAL retains prior applied selection and
per-workload/resource deployment acknowledgements, actual state and artifacts.
Read the existing deployment/operation report before retrying. Unknown outcomes
retain existing recovery holds; never reset state, replay apply blindly or invent
provenance. Catalogue selection is separate from database engine/data migration.
Destructive replacement remains subject to executor guards; catalog release
publication is not evidence that a database engine major upgrade is supported.
Downgrade and cross-source switching of an applied Environment are unsupported;
use a separately reviewed recovery/migration workflow.

No release/source deletion API is provided. Preserve cache, release rows and
historical artifacts needed for cleanup. Use the existing ownership/UID-protected
Application/resource deletion path; storage retention remains authoritative.
Do not remove a release from SQL to work around references or clean test data.

Validation: focused Go tests cover manifest/HTTPS ancestry, strict Definition
inspection, cache-pin reuse, atomic publication, tag conflict, selection CAS,
fencing, restart and least-privilege PostgreSQL storage. The
[E2E scenario](../../e2e/scenarios/catalog-upgrade/README.md) requires separately
authorized prepared Environments. Browser fixture checks prove UI behavior only;
application data/physical IDs, Jobs and partial provider outcomes still need
live acceptance for the chosen workload and module changes.

Initial safety boundary: database/storage module or effective-input changes on
existing PostgreSQL/MySQL/Redis/PVC resources are blocked during catalog Preview
unless the resource has provable RETAIN intent. No database/storage upgrade
capability is published in v1. Upgrade other supported components while retaining
those verified data pins, or use a separately reviewed data migration workflow.
