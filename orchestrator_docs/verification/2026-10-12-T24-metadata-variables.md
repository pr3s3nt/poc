---
id: VERIFY-2026-10-12-T24
artifact: verification-record
status: current
last_reviewed: 2026-10-12
---

# T24 — Metadata-only Variables

Branch `refactor_ui`; LEG-07. Initial working tree clean. Codex updated UC-12
BR-08/realization, ADR-012, shared operation/domain/schema contracts before
Claude code/test implementation via task session `codex-T24-20261012`.
UC-12 sequence/VOPC and ERD structure already express the current storage split;
no diagram structure changed. Current state/code map/traceability updated.

## Outcome and review

Variables carry only nonempty metadata Value; Secrets carry only explicit
StoreKey and opaque immutable ValueRef. Removed LegacyVariable/UsesStore,
materialize, legacy read/error branches, Legacy DTO field and obsolete
LEGACY_VALUE_UNREADABLE HTTP/Console mapping. JSON and PostgreSQL revision
reads and writes validate entries; retained store-backed Variables are rejected
without store access or silent conversion. Deployment/transition delivery checks
Secret kind directly. Matching/reference fixtures are untouched.

Spy tests prove Variable create/update/rename/list/delete do not resolve a store,
including beside Secret entries and with unconfigured selection. Secret DTOs
remain redacted. Store changes copy/verify only desired Secrets and preserve
Variables and historical owning-store refs. Unavailable/mismatched Secret owner
fails safely, leaving selection/revision and attempt cleanup intact. Existing
CAS/version/BUSY/fencing tests remain passing. New HTTP tests prove store-free
Variable CRUD with a Secret sibling, redacted DTOs and old-format GET/PUT/PATCH/DELETE
rejection as safe 500 INTERNAL_ERROR without reference/value details or conversion.
The historical revision is re-read after a switch and compared entry by entry. PostgreSQL SQL constraints remain
transitional until the T28 fresh baseline; retained state is not migrated/reset.

## Checks and evidence

Reports, failed and successful logs are outside Git in
`/tmp/poc-T24-handoff-20261012/`.

- Independent full uncached Go tests: exit 0, `codex-go-tests.log`; includes
  conformance. PostgreSQL/kind opt-ins unset.
- Independent final uncached configuration/e2e suites: exit 0,
  `codex-final-tests.log`, after HTTP review follow-up.
- Independent Go build: exit 0, `codex-go-build.log`.
- Independent uncached race checks on configuration/JSON store/conformance:
  exit 0, `codex-race.log`.
- Independent frontend typecheck/lint/426 tests/build: exit 0,
  `codex-fe-{typecheck,lint,tests,build}.log`.
- Claude full Go tests/build, go vet and frontend checks: exit 0; `report.txt`,
  `status.txt`, `be-test.log`, `be-build.log`, `fe.log`; `report-final.txt` records
  follow-up HTTP/service checks and full final backend gates. Initial failing test run
  retained as `be-test-first-fail.log`; two old Variable-ref fixtures corrected
  to metadata values without changing reference fixtures.

No new browser scenario/video: Settings UI behavior is unchanged from T07;
this task changes domain/service/persistence/API and removes an obsolete error
mapping. No PostgreSQL integration, live Vault, Kubernetes or cloud mutation:
not authorized in this local task. Retained DB/cluster/store data remain intact.
