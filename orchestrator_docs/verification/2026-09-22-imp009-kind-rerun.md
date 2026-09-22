---
id: VERIFY-2026-09-22-IMP009-KIND-RERUN
artifact: verification-evidence
status: evidence
last_reviewed: 2026-09-22
related: I06-07, IMP-009, UC-06
---

# 2026-09-22 — IMP-009 kind rerun on final renderer policy

## Scope

- Iteration: [I06-07](../iterations/M01-contract-hardening/I06-07-imp009-container-resources/README.md).
- Reruns the internal happy path after the review fixes recorded in
  [2026-09-22 IMP-009 container resources](2026-09-22-imp009-container-resources.md):
  BR-11 request precedence (declared request > limit of the same field >
  platform default `10m`/`32Mi`) and the `containerResources` comment/test
  change. That earlier record and its run `kind-20260922093228-12536` are not
  changed by this record.
- Code under test: working tree on top of commit `f3a4538` with the I06-07
  changes, uncommitted at the time of the run.
- What this run proves: the three **seeded** acceptance workloads render and
  run with the expected live container resources — backend declared
  requests/limits, worker partial (declared memory request and memory limit,
  no CPU request or limit), frontend omitted. None of them exercises the
  limit-fallback branch (request missing, limit declared) or a declared
  request above its limit; those remain covered by unit tests only
  (`TestRender_LimitsOnlyBelowDefaultUseLimitAsRequest`,
  `TestRender_DeclaredRequestAboveLimitIsPreserved`).

## Commands

Run under `backend/` with `go1.27.1 linux/amd64`, after the final code change:

| Command | Result |
|---|---|
| `gofmt -l .` | no output |
| `go test -count=1 ./...` | pass, no `FAIL` |
| `go build ./...` | pass |
| `go vet ./...` | pass |
| `go vet -tags integration ./test/integration/` | pass |
| `go test -count=1 ./test/conformance/ -run TestPlannerMatchesChallengeFixtures -v` | 33 of 33 fixture subtests `PASS`, no `SKIP` or `FAIL` |
| `bash test/integration/kind-verify.sh` | exit 0 |

Documentation checks: `env REQUIRE_PLANTUML=1 python3 scripts/check_docs.py`
and `git diff --check` passed after the documentation updates.

## kind run

- Preflight: `kind get clusters` listed `idp-internal`; current context
  `kind-idp-internal`; node `idp-internal-control-plane` `Ready` (v1.36.1); no
  namespace labeled `orchestrator.io/run-id` existed.
- Run ID `kind-20260922114440-17489`, namespace
  `acceptance-kind-20260922114440-17489`. The cluster was not created or
  deleted and the current context was not changed.
- Deployments through the HTTP API: backend `SUCCEEDED` in 17s, worker
  `SUCCEEDED` in 4s, frontend `SUCCEEDED` in 7s. Backend, worker, frontend and
  `shared-acceptance-db-0` were `Running`.
- Live container resources asserted by `TestKindInternalVerification`:

  | Workload | Seeded Score case | Live `resources` |
  |---|---|---|
  | backend | declared requests `50m`/`64Mi`, limits `500m`/`256Mi` | `{"limits":{"cpu":"500m","memory":"256Mi"},"requests":{"cpu":"50m","memory":"64Mi"}}` |
  | worker | partial: requests memory `48Mi`, limits memory `128Mi` | `{"limits":{"memory":"128Mi"},"requests":{"cpu":"10m","memory":"48Mi"}}` |
  | frontend | omitted | `{"requests":{"cpu":"10m","memory":"32Mi"}}` |

- Job flow returned `processed:KIND-1790077534567952250`.
- Secret handling: Secrets `backend-env`, `worker-env` and
  `shared-acceptance-db-credentials` listed by name only (values not read); no
  plaintext `PGPASSWORD` in the backend Deployment; the deployment view shows
  the database password as `***redacted***`. `/ui/deployments/<id>` returned
  200; live-console test 1 passed, 1 skipped by design (`it.runIf` branch for
  creating a new deployment).
- The PostgreSQL image archive could not be preloaded (`ctr: content digest …
  not found`); as the script allows, the kubelet pulled the image.

## Cleanup proof

- The EXIT trap deleted the run namespace and reported
  `namespace acceptance-kind-20260922114440-17489 is gone`; its run-id object
  query was empty.
- After the run, `kubectl get namespace acceptance-kind-20260922114440-17489`
  returned `NotFound` and
  `kubectl get all,pvc,secret,cm,namespace -A -l orchestrator.io/run-id=kind-20260922114440-17489`
  returned `No resources found`.
- `kind get clusters` still lists `idp-internal`; current context is still
  `kind-idp-internal`.

## Limitations

- Limit fallback and declared request above limit are not exercised live.
- AWS was not run.
