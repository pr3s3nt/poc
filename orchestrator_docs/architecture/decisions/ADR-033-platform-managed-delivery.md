---
id: ADR-033
artifact: architecture-decision
status: current
last_reviewed: 2026-11-02
related: [ADR-032, ADR-030, ADR-027]
---

# Platform-managed application delivery repositories

Directory layout and managed Fleet controller granularity are superseded by
[ADR-036](ADR-036-application-fleet-gitrepo.md). Repository allocation, immutable
Application binding, retention and credential boundaries below remain current.

## Approved scope

The user approved one deployment repository per Application, hidden from
Application users. Platform Engineers configure CD/Git once. Application users
configure workloads, Preview and Deploy without repository, branch, path, token
or CD-connection forms. Terraform ownership and Catalog/module sources remain.
Existing Applications are tests; no old-application migration, Direct adoption,
repository relocation or compatibility handoff is required. Do not erase test
data implicitly. Versioned database schema upgrades still follow ADR-027.

## Platform policy

An Organization chooses one verified platform CD binding as its default.
Verification/discovery remains read-only. New Applications pin that immutable
binding version for staging and production in their creation transaction; no
Git/cluster calls occur while creating an Application. Missing default is an
actionable platform-configuration error. A default change affects new Applications
only and cannot silently move admitted objects or repositories. The selected CD
must match Definition-derived placement; it never retargets Terraform/workloads.

The initial repository host adapter is GitHub (including explicitly configured
HTTPS Enterprise API/clone roots). Platform config includes repository owner,
name prefix, private writer credentials and Fleet connection details. Existing
Fleet and Git reader Secret/least-privilege platform permissions are prerequisites;
this change does not install controllers or grant cluster-wide workload rights.
Discovery fills controller/registration identities where uniquely provable.

## Repository allocation

The first admitted Deploy reserves one durable allocation before contacting Git.
For new Applications, [ADR-038](ADR-038-readable-idp-names.md) uses the immutable
readable Application identity with `-deploy` before its short random suffix;
existing allocations retain their names.
Historical Applications use the immutable Application UUID, not the display name.
Creation is private and initialized. Staging and production share this repository
and its acknowledged default branch; each uses `environments/<env>/manifests`.
Neither application DTOs nor their forms expose the repository's private identity.

Allocation stores Organization/Application/binding pins, a unique creation marker,
remote repository ID, HTTPS clone URL and branch. Before retrying any creation,
observe the deterministic destination and validate its exact creation marker.
Concurrent Environment deploys converge on one allocation/repository. A foreign
repository is never adopted; a changed remote ID after acknowledgement is refused.
Unknown HTTP outcomes are resolved by observation, not by generating a new name.
Git commits remain pinned per admitted delivery revision. Existing publication
leases serialize writers; paths separate Environments. Repo/branch resolution is
read-only during observation/removal and can never create repositories from a GET.

Repository history is retained after application cleanup. Namespace manifests
still follow CD withdrawal, receipts, UID-checked cleanup and VSO child ownership.
No automatic remote repository deletion or new Job replay is introduced.

## Storage and release

Add versioned schema 5 with platform default and application repository records,
tenant-scoped foreign keys, immutable allocation identity and runtime grants.
Keep frozen versions 1–4 unchanged. Test a populated version-4 upgrade, atomic
rollback, repeat setup, tenant isolation, privilege isolation and allocation races.
Old data is preserved but not converted into the new managed application flow.

The platform UI reuses CD configuration and existing AppShell/forms. Main fields
are name, cluster account and GitHub owner/credential. Discovery and advanced
connection details remain platform-only. Set-default is explicit after READY.
Application Settings removes delivery authoring. Errors refer users to platform
setup rather than asking for a repository URL. Fixture UI or simulated Git HTTP
does not prove live GitHub/Fleet deployment.

## Shared platform Git connection

CD setup reuses Organization-scoped Platform Settings Git access by default. Backend resolves the current credential on each new host/publisher operation without exposing or copying tokens into browser forms. Scope must cover the configured repository owner. Credential rotation applies to subsequent operations; operators re-verify CD after rotation. Separate credentials remain an explicit override. Account selection automatically performs read-only Fleet discovery with a retry action and visible cluster summary. Verification reports bounded actionable reasons for missing reader Secret, namespace or ServiceAccount; setup does not create cluster resources. READY bindings retain the explicit Use by default action.

Shared CD Git derives repository ownership from the explicit account/group URL scope in Platform Settings (for example `https://github.com/my-team`). Host-only and repository-specific scopes do not define an eligible owner. Settings displays the derived destination with guidance; CD hides owner authoring for shared Git and shows the inherited destination. Registration resolves this value server-side before validation, overriding any submitted owner. Authentication username is never assumed to be the owner. Existing binding versions retain their pinned owner; changed destinations apply when registering a new version. No separate owner column or schema migration is needed.
