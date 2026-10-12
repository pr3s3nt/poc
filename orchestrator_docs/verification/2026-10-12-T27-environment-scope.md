---
id: VERIFY-2026-10-12-T27
artifact: verification-record
status: current
last_reviewed: 2026-10-12
---

# T27 — Environment-only infrastructure scope and missing-only seed

Branch `refactor_ui`; LEG-02. Working tree initially clean. Codex updated
ADR-001/011/012/013, UC-01/03/05/06/08, domain/operation/schema contracts,
planner reference and compatibility matrix before Claude implemented code/tests
through owned tmux session `codex-T27-20261012`. ERD and cloud sequence PNGs were
regenerated with PlantUML. No reference fixtures or matching weights/ties changed.

## Outcome and review

Product has only ENVIRONMENT execution infrastructure scope. Removed
ScopeLegacyApplication, Context.LegacyInfrastructure and legacy cloud-name/path
and runtime-existence branches. Environment Validate/planning/target admission,
JSON Save/Bind/reopen and SQL Save/Bind reject explicit retired/unknown scopes.
Errors do not quote submitted scope values; no binding conversion is supported.
JSON reopen rejects bad scope before unconfigured defaults can erase it and
leaves the snapshot file unchanged. Unset scope retains the current documented
ENVIRONMENT default until the separate T28 baseline task.

@infra resolves to environments.<app>.<env>[.gN]. Current generation-0 namespace,
descriptor, resource-scope/hash and bounded AWS naming semantics remain; tests
compare the exact gen0 name digest and generations 0–5 for name/path collisions.
Existing generation isolation/namespace and ownership suites pass. Trusted
internal cluster retains its connection-addressed descriptor and Application
resource scope under ADR-013; APPLICATION resource scope is distinct from the
removed LEGACY_APPLICATION binding mode. Hardcoded Application VPC/EKS
references remain rejected with @infra guidance.

Seed uses explicit ENVIRONMENT bindings and creates missing Definitions without
upgrading exact earlier templates or rewriting existing authored records. Tests
preserve original/interim old templates over repeated Apply calls. No RD-01
profile changes or new matcher ranking/tie behavior were introduced.

Transitional SQL migration 10 validates infrastructure_scope='ENVIRONMENT' and
maps retired-data CHECK failure to a safe reset-required error without row values.
Migration 6's old scope CHECK remains historical ledger text, replaced by
migration 10 before serving; T28 replaces the full chain with a clean baseline.
SQL constraint/fresh/reopen/rollback tests compile but are unexecuted because
no disposable PostgreSQL database is authorized. This is not live SQL proof.

The conformance loader now supplies a synthetic ENVIRONMENT binding. No
additional path/name adapter was needed: all 33 fixtures pass unchanged, including
14 specificity, 22 class selection, 25 no match and 26 ambiguous match. The
existing ReferenceCluster harness boundary remains test-only; match.go and
orchestrator_reference files are untouched.

## Checks and evidence

Logs/report are outside Git at `/tmp/poc-T27-handoff-20261012/`.

- Independent uncached `go test -count=1 ./...`: exit 0,
  `codex-go-tests.log` and `.exit`; includes conformance and HTTP/e2e.
- Independent `go build ./...`: exit 0, `codex-go-build.log` and `.exit`.
- Claude uncached full Go suite/build/vet, targeted uncached and race suites:
  exit 0; `go-test-all.log`, `go-build.log`, `targeted-uncached.log`,
  `targeted-race.log`, `report.txt`. PostgreSQL/kind opt-ins unset.
- Claude verbose conformance: exit 0, `conformance-v.log`; all 33 fixture
  subtests run, no fixture skips. Accepted artifacts and rejected outcomes
  retain the existing assertions.
- Initial full suite failure in TestHTTPUsesTheConfiguredRunID retained in
  `failed/go-test-all-first-run-e2e-fail.log`: it expected the old cloud-name
  suffix. Assertion now checks the provider-safe name for the configured run
  and distinguishes another run; current product algorithm was preserved.
- Frontend typecheck/lint/test/build: exit 0, 429 tests; `fe-*-review.log`
  and `fe-exit-review.txt`. Codex reviewed the one-row Settings removal and logs.
- Headed T07 regression with Vietnamese captions: exit 0; runner evidence at
  `/tmp/poc-T27-evidence-T07-20261012`, run ID
  `refactor-20261012002327-30710`. `run.json` declares fake adapters;
  `result.txt` PASS, `assertions.txt` records 21 passing checks, `marks.json`
  contains 28 phase marks. Raw/captioned videos are `T07-raw.mp4` and
  `T07-captioned.mp4`; captions are `.srt`/`.ass`. Raw capture is 1440x900,
  386.2 seconds; captioned video adds a separate strip below the browser.
  Existing T07 uses memory credentials/local fake adapters; empty/list/error,
  stale/BUSY, usedBy and transition/recovery projections are explicitly mocked.
  Real UI Save/configuration operations use the local fake backend; this is
  not live Vault, PostgreSQL, Kubernetes or migration proof.
- Codex independently decoded the full captioned MP4: exit 0,
  `codex-video-decode.log` and `.exit`. Raw and captioned full decode also passed
  in Claude's `decode-raw-review.log` and `decode-cap-review.log`. Reviewed frames
  staging-saved, secret-saved, busy-locked and transition-open, plus captioned
  production draft at 87s. Browser/cursor and Vietnamese strip are visible;
  no legacy Settings binding label, plaintext secrets or obscured UI in reviewed
  frames. Labels/captions match the recorded fake/mock assertions.
- Documentation checker including PlantUML and `git diff --check`: exit 0.
  Whole-repository searches find no product ScopeLegacyApplication enum,
  LegacyInfrastructure helper or old seed-upgrade helpers. Remaining retired
  scope literals are negative tests and the explicitly historical migration 6
  ledger constraint; historical documentation/evidence is preserved.

## Limits and handoff

Codex review found one leftover Settings row for the retired binding mode.
UC-01 UI documentation was updated before Claude removed it; frontend gates and
headed T07 regression then passed without locator or shell changes. Core backend
scope/name/seed tests remain unit/API/planning coverage; the Settings row cleanup
is why this otherwise backend task includes a browser recording.

No disposable DB was authorized, so PostgreSQL opt-in tests were skipped with
ORCHESTRATOR_POSTGRES_TEST_URL unset; kind opt-in was unset too. No DB/cluster/cloud/
Vault mutation or evidence upload occurred. No shell was changed, so bash -n was
not needed. No pre-existing user-owned changes were present or staged. Dev reset
is required before use: T28 prepares the clean baseline; T29 owns the separately
approved retained-state reset/rebuild. This task does not authorize that run.

One commit with prefix `T27:` includes implementation, canonical docs, traceability,
current state and the DONE row in refactor.md. Find its actual SHA via
`git log --grep '^T27:'`; push destination is origin/refactor_ui. Task tmux session
is closed after successful handoff; other sessions remain untouched.
