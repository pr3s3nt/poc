---
id: UC03-DEFINITION-VERSIONS
artifact: specification-extension
status: current
last_reviewed: 2026-11-10
---

# Controlled Definition and artifact upgrades

User decision 2026-11-03 adds the following to CM-01–04. Form authoring remains
available. A save never deploys a workload or modifies provisioned infrastructure.

DV-01. Create records the first immutable Definition revision as Active. Editing
creates a Proposed revision; the mutable editor CAS counter is distinct from the
revision identity. Configuration, matching rules and exact module/bundle pins are
immutable within a revision. Promote changes the default for unselected/new
Environments only. Archive prevents new selection; existing pins remain usable.
An Environment without a SUCCEEDED Deployment and without an explicit selection
is unselected: it always resolves the current Active revision, even when a
system-captured freeze (authoring/promotion guard or owner migration) exists for
it. Only explicit selection or a successful deployment pins it (issue #68).

DV-02. Each Environment freezes its Definition configuration independently of
later authoring/promotion. It stores desired configuration, last fully applied
configuration and a selection CAS counter. Users explicitly choose a revision,
review differences and Preview/Deploy; staging and production are independent.
Definition families have one candidate revision per Environment, avoiding ties
between historical versions. Current criteria do not silently replace pinned
criteria. Type/account/target/dependency validation continues to apply.

DV-03. Record the exact selected Definition snapshots/revision IDs in new Plans.
Preview tokens cover selection and snapshots. Selection changes require current
Environment/selection versions and refuse busy/deleting scopes. Upgrade-only
changes reconstruct affected workloads even without Score edits. Apply advances
only after complete success; partial outcomes retain actual resource Plans and
previous full applied selection. Unknown outcomes/fencing remain authoritative.
Direct deployment may not bypass a pending Environment-wide revision upgrade.

DV-04. UI adds Versions and Usage to both Definition and rendering forms, with
Proposed/Active/Archived status, compare, promote and archive controls. Environment
Settings shows frozen/desired/applied versions, new-version availability, diff,
selection and Preview link. Preview also states when a newer Active revision exists
that a pinned Environment has not selected. Missing historical evidence is labelled unknown,
never falsely reported as successfully applied. Errors preserve drafts and stale
or unknown mutation outcomes require reload rather than automatic replay.

DV-05. Module/bundle pins remain immutable. Terraform binary/provider and renderer
identities are part of their artifact contracts. Retain verified old artifacts and
compatible executable versions; never reinterpret a pin with the current binary.
For Git template sources the renderer identity is the stable fingerprint of
[ADR-041](../../architecture/decisions/ADR-041-stable-template-renderer-pin.md),
not the whole backend executable.
If the matching old toolchain is unavailable, refuse execution with guidance.
Historical cleanup keeps original state/identity/artifact guards.

DV-06. Database/storage updates require verified migration capability. Until one
exists, upgrades to existing PostgreSQL/MySQL/Redis or PVC-capability resources
must have provable RETAIN intent. UPDATE/UNVERIFIED is blocked for both direct
Definition and Catalog upgrades. Reverting configuration is not data restoration;
there is no automatic database rollback or target/state adoption.

DV-07. Owner migration preserves existing rows/identities and captures current
Definition configurations/release bindings for old Environments without deployment.
It does not invent previously lost revision history or claim current configuration
matches actual resources. Only a successful new deployment establishes applied
snapshot evidence. New Environments freeze on successful deployment or explicit
selection. JSON/memory stores implement equivalent persistence semantics.

## Realization

Form -> Definition service -> shared validation -> transaction: editor CAS,
immutable revision INSERT and policy CAS. Revision actions never mutate snapshots.
Environment Settings -> authorized selection -> Environment lock/CAS -> frozen
snapshot. Planner resolves only the selected snapshot, pins it in Plan/token and
keeps existing resource identities. Fenced all-success completion records applied
selection. Repositories enforce revision immutability and organization isolation.

## HTTP and UI contract

PE/Admin GET `/resource-definitions/{key}/versions` returns history/generation and
scoped Usage. POST `/resource-definitions/{key}/versions/{id}/promote` or `/archive`
uses `{expectedGeneration}` and returns current history. Archived Active promotion
is rejected. Latest editing draft and active default are shown separately.

Scoped Environment GET `/applications/{id}/environments/{env}/definition-versions`
returns selection, Environment version, available revisions and differences.
PUT accepts `{definitionKey,revisionId,expectedRevision,expectedEnvironmentVersion}`.
POST `/.../definition-versions/discard` restores the last full applied or frozen
baseline policy. Organization, snapshots and pins are server-derived. Select is
configuration-only, not deployment. Existing authorization/error codes apply.

[Sequence](definition-versions-sequence.puml) ([PNG](definition-versions-sequence.png)).
