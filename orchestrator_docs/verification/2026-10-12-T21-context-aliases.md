---
id: VERIFY-2026-10-12-T21
artifact: verification-record
status: current
last_reviewed: 2026-10-12
---

# T21 — Environment context alias removal

Branch: `refactor_ui`; scope LEG-04. Initial working tree clean; no user-owned
changes remain. Canonical ADR-011, UC-03 specification/realization, OC-07,
compatibility matrix and planner reference updated before Claude implementation.

## Outcome and review

Codex reviewed both changed Go files. Product Values removes only app.profile
and app.region. ResolveContext uses existing unknown-value errors; no stored
input rewrite, compatibility resolver or fallback was introduced. Tests cover
absence, direct errors, whole/interpolated placeholder rejection, canonical
Environment expansion despite conflicting Application legacy fields, independent
Environment regions and unchanged Application identity/infrastructure naming.

Whole-repository alias search found no authored seed/product inputs or reference
fixtures using these aliases. Remaining hits are negative tests and documentation
explaining removal; no reference/harness changes needed. Existing legacy scope
and other cleanup tasks remain outside T21. Diagram participants/operations,
matching weights/ties, schemas and API payloads are unchanged.

## Checks and evidence

Claude task session: `codex-T21-20261012`. Handoff/logs outside Git:
`/tmp/poc-T21-handoff-20261012/` (report.txt, targeted.log, vbe-test.log,
vbe-build.log, alias-search.log).

- Claude `cd backend && env -u ORCHESTRATOR_POSTGRES_TEST_URL -u ORCH_KIND_VERIFY go test -count=1 ./internal/planning/...`: exit 0.
- Claude V-BE full Go test suite with both external opt-ins unset: exit 0; conformance and e2e passed, unchanged packages may use cache.
- Claude `cd backend && go build ./...`: exit 0.
- Codex independent `cd backend && env -u ORCHESTRATOR_POSTGRES_TEST_URL -u ORCH_KIND_VERIFY go test -count=1 ./internal/planning ./test/conformance`: exit 0, both uncached.
- `rg -n 'app\.(profile|region)' backend frontend orchestrator_docs` and repository/reference search: classified above.
- Final `python3 scripts/check_docs.py` and `git diff --check`: exit 0.

No browser scenario/video: context/unit-only task, frontend has no alias control.
V-FE/shell validation not applicable: no frontend/shell changes. PostgreSQL
opt-in, kind and AWS checks skipped because no disposable SQL/external mutation
was authorized. No retained DB/cluster/cloud was touched, no deploy or upload.
