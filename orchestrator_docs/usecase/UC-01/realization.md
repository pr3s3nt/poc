---
id: UC-01-REALIZATION
artifact: use-case-realization
status: current
last_reviewed: 2026-10-07
---

# UC-01 — Use Case Realization

ApplicationService.CreateApplication creates identity, two unconfigured
Environments/empty Sets atomically without resolving defaults. Existing
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
