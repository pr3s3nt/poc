---
id: ADR-022
artifact: architecture-decision
status: current
last_reviewed: 2026-10-25
---

# ADR-022 — Diagnostics belong to Infrastructure Accounts

The user requested executable readiness checks when installing the platform in
another cluster/cloud account, accepted the account-integrated mock direction
and authorized implementation, a Claude Code pull request, independent user-flow
E2E acceptance and merge to main on 2026-10-25.

## Decision

Infrastructure Accounts own a separate diagnostic-report aggregate. Connection
verification retains its existing meaning and lifecycle. Diagnostics never
rewrite that status, replace credentials, provision an Application or commit a
Deployment Set. General checks need no Application; use-case checks add selected
capabilities and optional Application/Environment context. Results are observations
of a pinned scope at a time, not a permanent account readiness flag.

The backend resolves the saved account identity and executes bounded probes
from its own runtime. Known deployment requirements reuse Definition matching,
planning, credential and adapter contracts. Missing context or unsupported proof
produces UNKNOWN; a failed dependency produces BLOCKED. Optional infrastructure
is required only by the selected configuration. Fake mock outcomes never enter
production wiring.

Reports and their lifecycle are durable, Organization/account scoped, with
separate ownership/lease and cancellation from Environment deployment operations.
No network probe holds a database transaction. Conditions are read-only; explicit
Kubernetes execution checks use only a unique run-owned namespace and verified
cleanup. Cloud provisioning under diagnostics is not implicit in execution mode;
capabilities without a supported bounded probe report UNKNOWN explicitly.

## Consequences

- The existing Console shell, Infrastructure Accounts route and role gate remain.
  The account detail gains Information and Diagnose tabs; the list exposes a
  scoped/time-stamped latest report. Existing management forms remain usable.
- A dedicated persistence port and report table avoid modifying connection
  verification JSON/version merely to save diagnostic history.
- The current fresh-baseline database policy remains: new schemas include the
  diagnostic aggregate; runtime never changes or resets an existing schema.
  Acceptance uses new owned databases. No retained installation is reset.
- Readiness claims require observed evidence. Authentication success, absent
  permissions, fake adapters and unexecuted cleanup cannot stand in for proof.

Requirements belong to [UC-04](../../usecase/UC-04/specification.md#account-diagnostics--diag-0114).
Protocol and probe semantics belong to the
[diagnostic contract](../contracts/account-diagnostics.md); storage belongs to
[schema](../database/schema.md#account-diagnostics--adr-022).
