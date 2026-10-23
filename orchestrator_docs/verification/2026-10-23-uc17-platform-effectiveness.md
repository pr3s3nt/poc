---
id: VER-20261023-UC17-EFFECTIVENESS
artifact: verification-evidence
status: evidence
last_reviewed: 2026-10-23
related: UC-17, issue #12
---

# UC-17 platform effectiveness — đợt 1

## Outcome

UC-17 report, records, CSV and append-only baselines are implemented end to
end on the platform-measured metrics; external-source metrics are
`NOT_COLLECTED`. Scope decisions: [issue #12 comment](https://github.com/pr3s3nt/poc/issues/12#issuecomment-6361990505).
Console is desktop-only by user decision (2026-10-23); no mobile check.

## Checks

| Check | Result |
|---|---|
| `go build ./...` | Pass |
| `go test ./...` with disposable local PostgreSQL 16 container, `REQUIRE_POSTGRES=1` | 56 packages pass. Four fail only on missing host tools: `redis-server` (`examples/acceptance-app/{backend,jobcache,worker}`), `terraform` (`internal/bootstrap` one test). |
| Same four packages: acceptance-app without `REQUIRE_*`; `internal/bootstrap` with checksum-verified Terraform 1.9.8 and `REQUIRE_POSTGRES=1` | Pass; acceptance-app Redis cases skip without the flag (module unchanged) |
| New Go tests | Domain formulas/timezone/retry/baseline; HTTP role, tenant, drill-down, records, errors, baseline CAS; PostgreSQL restart + Console UPDATE/DELETE denied (42501); JSON snapshot reload |
| `npm run typecheck`, `npx eslint src`, `npm run build` | Pass |
| `npm run lint` | Fails only in preexisting `design/resource-definition-*`, `test/e2e/ec2-role-real.mjs` (untouched) |
| `npx vitest run --maxWorkers=8` | 61 files / 819 tests pass. Default parallelism on this 56-core host times out unrelated heavy Resource Definition tests at 5 s; they pass in isolation |
| `python3 scripts/check_docs.py`, `git diff --check` | Pass; PlantUML rendered locally with PlantUML 1.2024.7 (smetana layout) |

## Browser (product UI, fake-adapter data)

Product server via the e2e fake-adapter harness (temporary build-tagged file,
not committed), seed `local` profile. History was created through
`POST /api/v1/deployments`: successful DEPLOY/UPDATE runs, an UPDATE that failed
planning and then succeeded (one work item, two attempts), an unresolved
failed worker run, and a failed run in a second Application. The report is
labelled “Dữ liệu kiểm thử”; this is not production evidence.

Chromium 1512×1000, platform-engineer: sidebar entry and active state;
organization default; no tabs; DORA frequency 4 with lead time/CFR/recovery/
rework “Chưa thu thập”; active applications 1/3, attempts 7 (3 failed); metric
dialog lists four records and Escape closes it; drill-down URL
`?application=acceptance`, workload breakdown, workload records; “Mở” opens the
UC-09 Deployment detail; baseline saved, shown after reload; CSV downloaded with
scope, period, timezone, origin, sources, units and records; no horizontal
overflow. Only browser error: the expected 401 of the pre-sign-in session probe.

Visual comparison with the Console Applications page: same AppShell, sidebar,
cards, borders and green primary Button. Fixed during review: dialog record
table clipped its last columns; “Mở” wrapped; baseline inputs inherited bold
labels. Screenshots: [organization](../../frontend/design/platform-effectiveness/screenshots/product-organization.png),
[application](../../frontend/design/platform-effectiveness/screenshots/product-application.png),
[metric detail](../../frontend/design/platform-effectiveness/screenshots/product-metric-detail.png),
[baseline](../../frontend/design/platform-effectiveness/screenshots/product-baseline.png).

Copy review (“what can move to help?”): definitions, original DORA terms and
reference links are in dialog `details`; the page keeps only units, scope,
period/timezone, coverage warnings and “Chưa thu thập”.

## Not run

No AWS, Kubernetes, kind or shared database. The user's running
`orchestrator-dev-none-*` stack was not touched; its database predates the new
baseline table and needs the documented development reset before this build
can open it. The previous preview session `effectiveness-mock-18097` was left
running unchanged and still serves the superseded mock.
