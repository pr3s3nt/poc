---
id: VERIFY-APPLICATION-OWNERSHIP-20261105
artifact: verification-record
status: current
last_reviewed: 2026-11-05
related: [UC-00, UC-01, ADR-027]
---

# Application owner access

Branch: `features/login`, based on `b0cdd093`.

## Behavior and implementation

Developer access requires an exact creator User ID and Organization match.
Platform Engineer/Admin can access every Application in their Organization.
Creation derives the immutable owner from the session; request JSON cannot set
an owner, Organization or role. Lists and all nested Application HTTP routes
apply the same rule, before lifecycle dispatch. Hidden Applications return 404;
missing sessions return 401. Existing action-specific restrictions still apply.
Application responses use `Cache-Control: no-store`.

Schema 9 adds a nullable, same-Organization owner FK and an immutable-owner
trigger. Retained ownerless Applications remain platform-only; no creator is
inferred. PostgreSQL owner setup is required for retained installations before
running this code. Snapshot persistence retains owners and rejects invalid
owner references. Fresh local/test acceptance fixtures have an explicit known
Developer owner; reseeding preserves retained owners.

## Verification

Tests use disposable PostgreSQL 16 databases with separate owner, Console and
State roles. Provider/execution collaborators in existing resource deletion
HTTP tests are fake; these tests verify authorization and lifecycle behavior,
not a live cloud/Kubernetes cleanup.

- Passed all tests in `internal/delivery/http`, `internal/adapters/store`,
  `internal/domain/application`, `internal/application/application` and
  `internal/application/catalogrelease`, including the same ownership HTTP
  contract on memory and PostgreSQL with `REQUIRE_POSTGRES=1`.
- Passed focused existing resource/application deletion HTTP tests: owner and
  Platform Engineer reads, Developer action restrictions, cross-Organization
  hiding, lifecycle and safe operation progress.
- Passed new schema-9 persistence/upgrade/rollback tests, the active/frozen
  schema comparison and generic-workload upgrade tests (21.019s).
- Passed `TestUpgradeSupportedPopulatedSources` including schema 8 and
  `TestUpgradePreservesStateAndRuntimeIsolation` across the older supported
  releases. The combined run had one failure in the unrelated
  `TestVersionSevenPreservesConnectionsAndAdmitsGit` template-source assertion;
  that exact failure also reproduces on `b0cdd093`.
- Passed `go build ./...`, documentation checker, migration immutability guard,
  PlantUML validation/rendering and `git diff --check`.

The full `go test ./...` run was attempted and is not green: renderer/template,
retired Score fixtures and other historical scenarios fail. Representative
failures reproduced unchanged in an isolated worktree of `b0cdd093`:
`TestInspectRejectsUnsafeSourcesWithoutFetching`, the V03 catalog capability/
placement tests, the V05 script/runner assertion mismatch,
`TestVersionSixCapturesPopulatedFiveAndProtectsSnapshots` (retired template
sources), `TestVersionSevenPreservesConnectionsAndAdmitsGit` (the same retired
source assertion), and `TestUpgradeHTTPLoginPreviewDeploy` (deployment renderer).
The feature does not claim that the entire suite passes.

## Rollout boundary

No retained installation, running Console, Kubernetes or cloud resources were
upgraded. No browser/UI deployment is part of this change. Existing ownerless
apps require a separately specified ownership assignment process if Developers
must regain access; this change does not add reassignment or shared membership.
