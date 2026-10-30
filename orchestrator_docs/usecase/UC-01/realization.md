---
id: UC-01-REALIZATION
artifact: use-case-realization
status: current
last_reviewed: 2026-10-28
---

# UC-01 — Use Case Realization

The V04–V06 replacement collaboration below supersedes deployment Connection/
transition participants in the historical staged description which follows.

ApplicationService.CreateApplication creates identity, two unconfigured
Environments/empty Sets atomically without resolving defaults. ApplicationRepository
Save/Get/List persist only identity/current metadata; no target/provider fields
or Application-to-Environment legacy backfill remain (T26). JSON/SQL reopening
preserves independently selected Environment bindings and historical target snapshots. Existing
ApplicationService.SetConnection becomes versioned change before runtime exists;
normal Save cannot bypass the dedicated command. EnvironmentSettingsService
selects SecretStoreConnection using ConfigurationService's immutable copy control.
EnvironmentTransitionService previews/claims/persists staged target transitions.

SettingsController owns scoped bounded DTOs, not credentials. ExecutionTargetResolver
and SecretStoreResolver independently resolve selected scoped READY identities.
EnvironmentOperationRepository claims admission in one local transaction with
version/config/draft/token checks; the lease spans a pending batch or migration.
External providers execute outside transactions under that operation owner.

EnvironmentTransitionService retains source targets/resources, creates isolated
destination generation, coordinates PostgreSQL transfer, full redeploy/readiness,
route cutover and final pointer commit. Failed stage preserves source authority
and records compensation/recovery results. Explicit cleanup resolves retained
source credentials and rejects current/referenced generations.

Trace: MS-01..08 create; ES-01..04 choices/versions/claim; ES-05 transfer or new
runtime; ES-06 safe view/progress. Validate CAS races, stale-preview admission,
claim persistence, store copy failure, generation isolation, restored records and
cleanup ownership according to [ADR-012](../../architecture/decisions/ADR-012-environment-stores-and-transitions.md).

## V04–V06 replacement collaboration

ApplicationService creates identity, staging/production and empty Sets atomically
without default Connection/profile/region. EnvironmentSettingsService retains
scoped Secret Store selection/copy through ConfigurationService, expected
versions and Environment operation ownership. Controllers publish safe metadata
and reject retired deployment-binding inputs instead of filling defaults.

EnvironmentTransitionService and deployment-Connection commands cease to be
execution entry points after V06.08. UC-05/06 planning resolves matched resource
references; UC-08 executes with original per-resource target/account/state pins.
The operation repository still fences concurrent mutations and final commits;
no network call is held inside a database transaction. Reopen retains supported
metadata and immutable target provenance, never reconstructs a retired binding.
See ADR-017 and the updated sequence/VOPC for this final collaboration; delivery
status remains in CURRENT_STATE rather than inferred from these diagrams.

## ADR-030 collaboration extension

Settings controller saves desired delivery under Environment CAS; applied selection changes only after the admitted delivery/transition finishes.

Canonical shared collaboration: [delivery sequence](../../architecture/components/delivery-reconciliation-sequence.puml),
[participants](../../architecture/components/delivery-reconciliation-vopc.puml),
[infrastructure sequence](../../architecture/components/infrastructure-drift-sequence.puml).
These supplement the existing direct-path diagrams for this new design slice;
[CDR contract](../../architecture/contracts/delivery-reconciliation.md) owns port/API semantics.
