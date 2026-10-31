---
id: ADR-031
artifact: architecture-decision
status: current
last_reviewed: 2026-10-30
---

# ADR-031 — Versioned Git catalog releases (issue #28)

Issue #28 and the user's implementation instruction authorize catalog releases
selected independently per Environment. This design adds no deployment authority.

A source belongs to one Organization and specifies an HTTPS repository, tracked
branch, tag glob, manifest path and prerelease policy. Credentials remain in
[platform Git access](../../usecase/UC-03/git-access.md). Discovery defaults to
15 minutes with bounded runs and explicit manual refresh. Stable SemVer tags
sort numerically; prereleases require opt-in. No tag is selected automatically.

Discovery captures a branch tip and peels tags in one private hardened repository.
Only commits proved ancestors of that observed branch tip may become releases.
The branch tip and observation time are retained as evidence. Tag identity is
(source, tag): moving an observed tag is a conflict, including a previously
invalid tag. Deletion or remote failure never removes an existing release.

A strict bounded `catalog.yaml` v1 lists stable component IDs, kinds and clean
relative paths. Terraform entries name their resource type. Definition files
contain reusable Definition configuration with symbolic component and binding
references. All components live at the release commit; nested module sources
follow the existing immutable source contract. Unknown fields, duplicates,
unsafe paths, unsupported versions and incomplete references reject publication.

A release contains the original manifest bytes/hash, commit SHA, verified
component hashes/cache pins and immutable Definition snapshots. Inspection may
populate an immutable cache, but a release is visible only after the complete
set validates and publication commits atomically. Releases are never mutated or
deleted while referenced by desired/applied configuration, plans, outcomes or
cleanup provenance. Mutable global Definitions are never overwritten by discovery.

Platform bindings map a release's symbolic account slots to Organization-scoped
accounts. Account versions/criteria/dependencies use existing validation. Git
never carries account credentials. A pinned Environment resolves exclusively
against its release snapshots, with no fallback to mutable global Definitions;
legacy Environments retain existing matching. No fabricated adoption provenance.

Selecting a release saves desired configuration under Environment CAS and busy
guards. Preview includes selection revision and release digest in its hash and
reconstructs all affected workloads. Existing configuration is preserved; removed,
retyped, missing-required or otherwise incompatible values block Preview. No
unsupported destruction, replacement or database major upgrade is admitted.

Execution uses only Preview pins and verified cache. Existing locks, fencing,
state identity and unknown-outcome holds remain authoritative. Per-component
acknowledgements retain their original/new artifact pins. Applied release advances
only after all required components succeed; failure records PARTIAL and retains
individual outcomes. Retry never blindly repeats unknown apply. Cleanup uses
original acknowledged artifacts, even when desired selection differs.

[Specification](../../usecase/UC-03/catalog-releases.md) owns observable behavior;
[contract](../contracts/catalog-releases.md) owns manifest and service boundaries.
