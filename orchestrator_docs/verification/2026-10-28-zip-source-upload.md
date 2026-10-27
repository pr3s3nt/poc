---
id: VERIFY-ZIP-SOURCE-UPLOAD-20261028
artifact: verification-record
status: evidence
last_reviewed: 2026-10-28
related: [ADR-025, UC-03]
---

# ZIP module and render-bundle upload — local verification

Branch `features/themfile`, based on main `4764b11`. This verifies the source
preparation feature in [ADR-025](../architecture/decisions/ADR-025-zip-source-upload.md).
It is not evidence of Terraform apply, a Kubernetes deployment or cloud execution.

## Automated checks

- Focused source reader/registry/catalog/HTTP/review tests passed. Coverage includes
  unsafe/archive limits, duplicate/conflicting paths, corrupt content, cancellation,
  strict multipart/RBAC/scoped Type, slow-upload transport deadline, oversized
  HTTP 413, Organization-scoped ZIP capability discovery/registration and
  no Definition side effects,
  repackaged ZIP identity, local child modules, trusted release-equivalent identity,
  restart/update/tamper and cache repair. ZIP module materialization verifies the
  pinned artifact and a test binary; it does not execute real Terraform.
- Real PostgreSQL runs use newly owned `postgres:16-alpine` containers and
  REQUIRE_POSTGRES=1. The first run passed the PostgreSQL package (including ZIP
  source-kind reopen), but the existing tfexec cleanup-order test reported DESTROY_FAILED during
  host contention and the package hit the default 10-minute limit. Its evidence remains at
  `/tmp/themfile-zip-validation.3VzZHZ/`; that owned container was removed.
  The second full run (`GOMAXPROCS=4 go test -p 2 ./...`) passed every other
  package, including PostgreSQL, but tfexec's 85-test package again reached the
  total 10-minute runner cap at `TestTerminalRecordFailuresHoldTheEnvironment`;
  there was no earlier assertion failure in that run. Evidence remains at
  `/tmp/themfile-zip-validation.oCCM8C/`, owned container removal verified.
  The final tfexec run partitions the complete test inventory into three
  non-overlapping groups, retaining the same 10-minute runner limit per group,
  all business deadlines/assertions, REQUIRE_POSTGRES=1 and REQUIRE_TERRAFORM=1.
  That attempt accounted for 82 passing tests and three affected by temporary
  storage exhaustion (two errors explicitly reported `no space left on device`).
  `/tmp` was a nearly full 12-GiB tmpfs. The three affected tests then passed on
  disk-backed TMPDIR under `/var/tmp`, with the same assertions/deadlines and
  required real PostgreSQL/Terraform. Reconciliation of the complete inventory
  confirms **85/85 passed, zero skipped**. Summary and original group evidence:
  `/tmp/themfile-zip-validation.8pbHpz/final-summary.json`. Corrected-run evidence:
  `/var/tmp/themfile-zip-validation.isLtYL/` and
  `/var/tmp/themfile-zip-validation.k2uAhz/`. All owned containers were removed;
  corrected-run temporary state/binary directories were removed. Existing user
  temporary files were left in place.
- Race checks passed for sourcezip and both source registries. After the final
  source-selector metadata fix, all 42 rendering-page tests, typecheck, lint and
  production build passed again.
- Frontend: typecheck and lint passed; all **925 tests** passed with two workers;
  production build passed (existing chunk-size advisory). An initial unlimited
  worker run overloaded the host and was stopped; required assertions/timeouts
  were retained on the successful rerun.
- Documentation checker and PlantUML syntax passed, changed diagrams were
  regenerated, and final whitespace validation passed.

## Browser and visual comparison

Ran [the ZIP scenario](../../e2e/scenarios/zip-source-upload/README.md) against an
isolated loopback test-profile backend with JSON state, real packaged module
registry/ZIP reader, real score-k8s 0.15.0 capability and real durable caches.
Deployment adapters were fake and were never invoked. The first successful run used Vite; the final run served the production build
directly from the backend after Organization isolation was verified in HTTP tests.

PASS: upload `modules/namespace` and `bundles/standard` from the same ZIP; verify
ZIP source/revision/directory, catalog persistence, rendering Definition Create,
reload/reopen and source detail. No Git inspect request occurred. Backend/SQL
registry tests separately cover restart of verified metadata and cache reuse.

Compared existing product shell/cards/source selectors against the resulting
screens at 1512px and 1280px. Both forms retain the established layout and have
no horizontal overflow. ZIP controls show file, directory, one explicit action
and result; directory semantics/limits and full provenance stay in details.
No unrelated screen cleanup was made. Initial reference rendering used product
pages with empty fixture API responses, not live-platform evidence.

Final evidence is outside Git at `/tmp/themfile-zip-ui-evidence-r2/`
(the earlier successful Vite run remains at `/tmp/themfile-zip-ui-evidence/`): upload screenshots
at both widths, successful browser log, server log and cleanup report. The first
browser runner attempt used an overly strict select locator and stopped before
upload; the locator was corrected to the actual accessible combobox. The
successful run kept business assertions.

Cleanup verified: only owned backend/Vite processes stopped, loopback ports
18528/5178/5179 closed (the final run needed only 18528) and disposable state/cache/password directory removed.
Immutable catalog records were scoped to that discarded local platform.
No retained installation, user tmux, cloud or Kubernetes resources were changed.

Final ZIP Organization isolation checks passed: the HTTP suite proves a second
Organization cannot discover or register the original uploaded module, but can
inspect its own copy with a distinct source identity. Registry race checks passed
again. `TestZIPSourceKindsSurvivePostgresReopen` passed on a new disposable real
PostgreSQL after the final JSON ownership field was added; evidence
`/tmp/themfile-zip-validation.cm2H4K/`, owned container removal verified.
