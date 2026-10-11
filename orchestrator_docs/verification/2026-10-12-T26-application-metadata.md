---
id: VERIFY-2026-10-12-T26
artifact: verification-record
status: current
last_reviewed: 2026-10-12
---

# T26 — Identity-only Application metadata

Branch `refactor_ui`; LEG-01. Initial working tree clean. Codex updated
ADR-011/012, UC-01 BR-04/realization, UC-06 PRE-02/MS-04/realization, shared
operation/domain/schema contracts and ERD/class diagrams before Claude
implemented code/tests through task session `codex-T26-20261012`.
UC-01/06 sequences already resolve Environment destinations; no sequence change.
Changed PlantUML PNGs regenerated with the installed PlantUML tool.

## Outcome and review

Application carries only ID/key, Organization, name, subdomain and version.
Removed Profile, ConnectionKey, Region, RuntimeStatus and ConfigurationProvider
from domain and SQL/JSON Save/Get/List. Create remains name/subdomain only and
creates staging/production UNCONFIGURED. Environment owns execution/store
selections; no Application or Organization-default target fallback is introduced.

Removed Application-to-Environment JSON/SQL binding backfill and Application
Save target-preservation logic. The JSON startup guard rejects retained meaningful
or malformed target/provider fields with a safe reset-required error, without
printing Application keys or submitted values. Absent/null/empty retired fields
and the empty-target UNCONFIGURED marker hold no binding and are not converted.
Environment defaults initialization is explicitly separate from target migration.

Transitional SQL migration 9 rejects Applications holding target/provider data
before dropping the five obsolete columns. Migration failure rolls back the
transaction; no binding data is copied. Fresh/reopen and rejection-without-mutation
integration tests were updated, but their PostgreSQL execution remains unverified
because this task has no authorized disposable DB. The migration chain is retained
until T28; this change must not be deployed to retained dev state before T29 reset.

Codex review caught a first JSON guard treating non-string retired values as empty;
Claude corrected it and added numeric/object/array/bool cases. Shared persistence
contracts verify identity Save/Get/List and independent Environment bindings.
Workload-history contracts now save Application metadata and compare historical
Deployment ConnectionKey/generation and workload TargetRef snapshots before/after
save and JSON reopen. SQL restart tests share these assertions but are skipped.
Existing unconfigured Preview/Deploy, scoped targets, version/CAS/fencing and
snapshot/history suites pass. Humanitec fixtures and matching/ranking/ties are
untouched; harness already supplies explicit Environment context and app.id.

## Checks and evidence

Logs/report are outside Git at `/tmp/poc-T26-handoff-20261012/`.

- Independent full uncached `go test -count=1 ./...`: exit 0,
  `codex-go-tests.log`; includes conformance and HTTP/e2e. PostgreSQL/kind opt-ins unset.
- Independent `go build ./...`: exit 0, `codex-go-build.log`.
- Independent uncached race checks on Application domain/service, JSON store,
  persistence contracts and conformance: exit 0, `codex-race.log`; opt-ins unset.
- Claude full Go tests/build/vet and race checks: exit 0; `report.txt`,
  `test-full.log`, `test-final.log`, `race.log`, `test-review2.log`.
  `test-review1.log` retains an initial failed new test assertion, corrected
  before final checks. The final follow-up section of the report supersedes
  its earlier guard/helper descriptions.
- Documentation checker including PlantUML and `git diff --check`: exit 0.
- Repository search found no Application target field accesses, old
  `migrateLegacyBindings` helper or Application-binding SQL backfill. No changes
  under `orchestrator_reference/` or `frontend/`.

No browser scenario/video or frontend gates: T26 changes backend/domain/persistence;
Console behavior remains as recorded in T07. No PostgreSQL integration, live DB,
Vault, Kubernetes or cloud mutation was performed. No credentials or `.env` read.
Retained resources remain intact. Development reset is required before using old
bound state; approval/execution belongs to T29. T27 still owns LEGACY_APPLICATION
scope and seed-template upgrade cleanup; T28 owns the fresh baseline and unrelated
connection/history/default upgrade branches.
