---
id: VERIFY-2026-11-09-ROLLOUT-FAILURE-RELEASE
artifact: verification
status: current
last_reviewed: 2026-11-09
related: [UC-06, UC-01, UC-09]
---

# Determined rollout failure releases the Environment — issue #55

## Scope

Branch `features/rollout-failed-release`, head `c7d62d67`. Requirement:
[UC-06 CD-10](../usecase/UC-06/specification.md). Scenario:
[rollout-failure-release](../../e2e/scenarios/rollout-failure-release/README.md).

## Automated checks

- Delivery tests: Degraded rollout and not-Ready-at-deadline end FAILED without
  hold; observation error, cancelled operation, CD transfer keep the hold;
  Recover of a Degraded or never-Ready rollout ends FAILED without resubmitting;
  transfer recovery keeps the hold; the corrected redeploy names the published
  failed revision; never-published failures are skipped. Package passes.
- Real PostgreSQL 16 (`REQUIRE_POSTGRES=1`): a determined failure is persisted
  FAILED, releases the Git publication lease and lets the Environment be released
  and claimed again.
- `deployment` and `pending` packages fail the same 22 tests as unmodified `main`
  `cc4320af` (identical list, environment-related baseline); no new failure.
  `envops` and HTTP delivery packages pass. Build, vet and gofmt pass.
- Frontend: typecheck, build and 993/993 tests pass (`--maxWorkers=4`); lint keeps
  the existing PlatformSettingsPage warning. Documentation checker passes.

## Kind run

A Console was installed in its own namespace on `kind-k8s-4f` and seeded through
the API (Git, cluster account, workload Vault, Fleet CD, cluster/namespace and
template workload Definitions from `orchestrator-workload-templates@v0.4.2`). The
workload exits at once (CrashLoopBackOff), then is corrected through the form.

| Step | Console build | Result |
|---|---|---|
| Deploy crash-looping workload | `main` `cc4320af` | INTERRUPTED after 343 s, Environment held (reproduces the report) |
| Recover from Settings | first branch build | 504 after 15 min: recovery waited 20 min inside one request; held again. Fixed in `52eaf1d6` |
| Recover from Settings | `52eaf1d6` | RECOVERED after 318 s, Environment released |
| Deploy crash-looping workload (new app) | branch | FAILED after 340 s, Environment released, `ROLLOUT_NOT_READY`, detail page shows the rollout message and "Xem log" |
| Fix arguments, Deploy | `52eaf1d6` | INTERRUPTED: Fleet refused the replacement (ownership pin of the failed revision). Fixed in `c7d62d67` |
| Bad Deploy, fix, Deploy, delete (third app) | `c7d62d67` | FAILED after 340 s and released; SUCCEEDED after 32 s; deletion SUCCEEDED, namespace absent |

A crash-looping first rollout keeps the Deployment `Progressing=True`
(`NewReplicaSetAvailable`), so Fleet reports NotReady, never Degraded; the failure
is concluded at the 5-minute readiness deadline.

## Findings outside this scope

1. **Template sources become UNAVAILABLE after a backend build change.** Every
   upgrade to a different backend image marked all previously registered template
   sources UNAVAILABLE (also on the shared kind Console); Deploy then fails with
   `TEMPLATE_UNAVAILABLE` until the template is registered again and the workload
   Definition revised.
2. **Deleting an Application whose first Deploy never succeeded** is refused
   (`CLEANUP_REFUSED`): its namespace resource stays READY, never UNREFERENCED, so
   no cleanup root is eligible. Not changed by this issue (the Environment was held
   before it); relates to Environment rollback to empty in #49.
3. **Unknown push right after a published failure:** if the replacement push fails
   ambiguously, recovery cannot yet prove the new revision absent because the unit
   pin belongs to the failed revision; the Environment stays held (case of #56).
4. CrashLoopBackOff is detected only at the readiness deadline; earlier detection
   would need Pod restart evidence from the provider.

## Cleanup

The third Application was deleted through the Console. The first two Applications
(one refused by finding 2, one held by the intermediate build) remain in the
isolated Console for review; the isolated namespace, its cluster role binding and
those Applications' namespaces and Fleet controllers are removed with the test
installation. Delivery Git repositories are retained per ADR-033.
