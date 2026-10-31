---
id: CATALOG-RELEASES-CONTRACT
artifact: operation-contract
status: current
last_reviewed: 2026-10-30
---

# Catalog release contract

## Manifest v1

The manifest is UTF-8 YAML, at most 1 MiB, one document, no unknown fields,
anchors, aliases or duplicate keys. At most 256 components; IDs are lowercase
DNS-style identifiers (1..63 bytes). Paths are clean relative slash paths,
without traversal, `.git`, backslashes or absolute paths. Components are sorted
by ID for deterministic comparison, without changing the original byte hash.

```yaml
apiVersion: orchestrator.dev/catalog/v1
components:
  - id: postgres
    kind: terraform-module
    path: modules/postgres
    resourceType: postgres
  - id: workloads
    kind: render-bundle
    path: rendering
  - id: postgres-default
    kind: definition
    path: definitions/postgres.yaml
```

Kinds are `terraform-module`, `render-bundle`, `definition`. Resource type is
required only for Terraform modules. Paths/IDs are unique. Every release must
include at least one Definition; definition documents refer to module/render
components by stable ID, never mutable external refs. Component hashes are
SHA-256 of inspected content; manifest digest is SHA-256 of original bytes.

## Source and release

Source configuration is Organization-scoped: ID, repository URL, tracked branch,
tag pattern, manifest path, includePrereleases, version. HTTPS URLs cannot contain
userinfo, query or fragment. Branches are validated as literal Git branch names;
tag pattern is a bounded single-name glob. Default branch `main`, tag pattern
`v*`, manifest `catalog.yaml`, prereleases off, refresh interval 15 minutes.

Observed tag commit is immutable, even if validation failed. Release publication
requires commit/branch-tip SHA, manifest digest, complete component hashes,
verified module/render pins and resolved Definition snapshots. IDs and hashes
are stable. Per-tag failures are distinct from source fetch failures. API errors
are fixed codes without raw Git stderr or submitted content.

## Upgrade compatibility and differences

Compare stable component IDs/hashes; results are added, removed, update or
no-change. Absence is distinct from empty/default values. Existing supplied
parameters are never overwritten by new defaults. Removing a supplied input,
changing its type or requiring a missing value is a blocker. Existing planner
contract validation remains authoritative for references and resource targets.

Environment desired/applied release references and per-component outcomes are
persisted separately; legacy absence means no catalog pin. Applied advances
only after all required acknowledgements. Infrastructure operations continue
through existing Preview/admission/execution boundaries.

## Platform source API

All routes below require authenticated PE/Admin and derive Organization from the
session, never submitted JSON. GET `/api/v1/platform/catalog/sources` returns
`{sources:[...]}` (configuration/version, lastChecked and safe code).
PUT `/api/v1/platform/catalog/sources/{source}` accepts `{repository,branch,
tagPattern,manifestPath,includePrereleases,expectedVersion}` and returns
`{source:...}`. Missing optional configuration receives the source defaults.
POST `/api/v1/platform/catalog/sources/{source}/refresh` performs a bounded manual
refresh and returns the observation summary. It does not deploy an Application.
GET `/api/v1/platform/catalog/sources/{source}/releases` returns safe immutable
release summaries (tag, commit, manifest digest, branch tip and observation time).
Public responses exclude Definition internals and observed-tag storage maps.
Invalid input uses INVALID_REQUEST, stale CAS uses STALE_VERSION, moved identity
uses CATALOG_CONFLICT, absence NOT_FOUND and unavailable service
SERVICE_UNAVAILABLE. Per-tag discovery results are fixed codes on the observation.

## Definition document

A Definition file has `{definition, component?, binding?}`. `definition` uses the
existing strict JSON field names (JSON or YAML encoding). Its account and source
fingerprint must be absent; `component` names a module/render entry in this
manifest, `binding` names a platform account slot. Terraform Definitions omit
`driverInputs.values.source`; discovery fills the verified source pin. Workload
Definitions omit `driverInputs.values.variables.render_bundle`; discovery fills
the verified render bundle ID. Existing-cluster Definitions name only a binding.
The existing UC-03 validation pipeline checks the materialized Definition before
publication without modifying the global registry. Missing platform bindings
block publication; account credentials are never in the document.

Source API also accepts `accountBindings: {slot: accountId}` (PE/Admin only).
Environment selection snapshots these slots, independent of later source edits.
Unchanged components retain a prior verified execution pin when full content,
transitive contract and platform Terraform/renderer release identity agree.
The new catalog still records its observed commit and manifest provenance.

## Environment API

GET `/api/v1/applications/{id}/environments/{env}/catalog` uses existing scoped
configuration authorization and returns `{catalog, environmentVersion, releases}`.
`catalog` includes revision, desired/applied selection and LEGACY/PENDING/PARTIAL/
APPLIED status. Available releases expose sourceId, tag and fingerprint.
PUT accepts `{sourceId, tag, expectedEnvironmentVersion, expectedRevision}`;
Organization and account bindings are server-derived. Saving mutates no provider.
Unsupported downgrade within a source is rejected; legacy adoption has no inferred
current version. Preview resolves all affected workloads from the desired release
and pins its digest/revision. A simultaneous delivery ownership transition is
blocked. Applied activation occurs only after the entire pending batch and route
reconciliation succeed. Failures retain acknowledged deployment snapshots and
PARTIAL status; existing unknown-outcome guards continue to block unsafe replay.

V1 has no verified database/storage migration capability. During catalog Preview,
existing PostgreSQL/MySQL/Redis resources and Types exposing claimName/namespace
must have provable RETAIN intent. UPDATE or UNVERIFIED intent blocks before
mutation and directs the operator to a separate reviewed migration. New resources
and unchanged data components remain supported. This deliberately conservative
rule prevents a module/default change from silently performing an engine major
upgrade; catalog publication alone cannot authorize data migration.


Platform release summaries also include scoped `usage` rows with Application,
Environment, desired/applied flags and catalog status. These are a consistent
read projection of Environment bindings; they contain no account configuration
or secret refs. Release lists use numeric SemVer ordering, newest first.
