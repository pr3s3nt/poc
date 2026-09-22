---
id: VERIFY-2026-09-22-IMP009-CONTAINER-RESOURCES
artifact: verification-evidence
status: evidence
last_reviewed: 2026-09-22
related: I06-07, IMP-009, UC-05, UC-06
---

# 2026-09-22 — IMP-009 Score container resources

## Scope

- Iteration: [I06-07](../iterations/M01-contract-hardening/I06-07-imp009-container-resources/README.md).
- Canonical inputs: UC-05 BR-07, UC-06 BR-11,
  `ContainerResourceRequirements`/`ComputeResources` in
  [domain objects](../architecture/domain/domain-objects.md) and the
  Container resources row of the
  [compatibility matrix](../implementation/humanitec-compatibility.md).
- Canonical clarifications made with the change. The UC-05 BR-07 validation
  boundary and the first BR-11 omitted-value policy (missing request always
  gets the platform default) were approved by the user before
  implementation. After review, BR-11 was revised before the code was changed
  (see "Review follow-up"); the text below states the revised rule:
  - UC-06 BR-11: each
    request field is the declared request, else the declared limit of the same
    field, else the platform default `cpu: 10m` or `memory: 32Mi`; limits have
    no default; derived values are never written into the Score fragment or
    Candidate Deployment Set. UC-06 realization MS-11 and the domain-object
    note repeat this.
  - UC-05 BR-07: declared fields must be non-empty strings; `null`, JSON
    numbers, empty strings and other branches/keys are rejected. The planner
    does not validate Kubernetes quantity semantics; the Kubernetes API does so
    at apply time.
- Change under verification:
  - `environment.ComputeResources` and `environment.ContainerResourceRequirements`
    in `backend/internal/domain/environment/document.go`, carried as
    `Container.Resources` (`omitempty`, so modules without resources serialise
    unchanged).
  - `backend/internal/planning/score/resources.go` validates the raw
    `containers.*.resources` tree after the strict (`DisallowUnknownFields`)
    decode. `Document.Fragment` copies the requirements into the module.
  - `containerResources` in `backend/internal/adapters/kubernetes/renderer.go`
    replaces the hard-coded `requests: {cpu: 10m, memory: 32Mi}` with declared
    values plus the BR-11 request policy. The module input is not modified.
  - Seeded acceptance Scores (`backend/internal/seed/scores.go`): backend
    declares full requests/limits, worker a partial set, frontend none.
  - `TestKindInternalVerification` asserts live container resources;
    `kind-verify.sh` records `container-resources.txt`.
- No CPU/memory Resource Graph node, no UC-08 or cloud executor change, no
  persistence schema change.

## Contract tests

| Requirement | Tests |
|---|---|
| Full, partial and omitted requests/limits parse into typed values | `TestParse_ContainerResourcesFullRequestsAndLimits`, `TestParse_ContainerResourcesPartialRequestsAndLimits`, `TestParse_ContainerResourcesOmitted` |
| Values preserved verbatim, no quantity normalisation | `TestParse_ContainerResourcesPreserveValuesVerbatim` |
| Unknown branch/key, number, boolean, object, `null` value/branch/resources, empty string rejected | `TestParse_ContainerResourcesRejectsInvalidShapes` (15 cases), `TestParse_ContainerResourcesRejectsInvalidShapesThroughFromMap` |
| Per-container requirements, multi-container | `TestFragment_ContainerResourcesStayPerContainer`, `TestRender_ContainerResourcesArePerContainer` |
| Candidate Set, Delta `modules.add` and `base + delta` keep exact values | `TestPlan_PreservesContainerResourceRequirements` |
| No Resource Graph node; graph and batches unchanged | `TestPlan_ContainerResourcesAddNoGraphNodes` |
| Change is a module-relative patch carrying the Score string | `TestPlan_ContainerResourceChangeIsModuleRelativePatch` |
| Renderer: declared values, missing request filled from limit then default, limits only when declared, empty branches | `TestRender_DeclaredContainerResources`, `TestRender_PartialContainerResourcesFillMissingRequests`, `TestRender_OmittedContainerResourcesKeepDefaultRequestsWithoutLimits` |
| Limits-only below default (`cpu: 5m`, `memory: 16Mi`) render request = limit; a declared request wins over the limit and is preserved even above it (the Kubernetes API rejects that pair at apply time) | `TestRender_LimitsOnlyBelowDefaultUseLimitAsRequest`, `TestRender_DeclaredRequestAboveLimitIsPreserved` |
| Renderer does not mutate module input or write derived requests back | `TestRender_DoesNotMutateModuleResources`, `TestRender_LimitFallbackDoesNotWriteBackRequests` |
| Deploy path: rendered manifests, persisted current set and Delta Snapshot reloaded from the JSON state file | `TestDeployWorkload_RendersDeclaredContainerResources` |

The new parser, renderer and planning tests were written first and failed to
compile against the old model before the model change.

## Commands

Run under `backend/` with `go1.27.1 linux/amd64`:

| Command | Result |
|---|---|
| `gofmt -l .` | no output |
| `go test -count=1 ./internal/planning/score/... ./internal/adapters/kubernetes/... ./internal/application/deployment/... ./internal/planning/... ./test/conformance/` | pass |
| `go test -count=1 ./test/conformance/ -run TestPlannerMatchesChallengeFixtures -v` | 33 of 33 fixture subtests `PASS`, no `SKIP` or `FAIL` |
| `go test -count=1 ./...` | pass, no `FAIL` |
| `go build ./...` | pass |
| `go vet ./...` | pass |
| `go vet -tags integration ./test/integration/` | pass |
| `bash -n test/integration/kind-verify.sh` | pass |
| `bash test/integration/kind-verify.sh` | exit 0 |

Documentation checks: `env REQUIRE_PLANTUML=1 python3 scripts/check_docs.py`
and `git diff --check` passed after the documentation updates.

## kind run

- Cluster `idp-internal` (kind v0.32.0, Kubernetes v1.36.1), context
  `kind-idp-internal`. The cluster existed before the run; the script did not
  create or delete it and the current context stayed `kind-idp-internal`.
- Run ID `kind-20260922093228-12536`, namespace
  `acceptance-kind-20260922093228-12536`.
- Deployments through the HTTP API: backend `SUCCEEDED` in 18s, worker
  `SUCCEEDED` in 4s, frontend `SUCCEEDED` in 7s. Backend, worker, frontend and
  `shared-acceptance-db-0` were `Running`.
- Live container resources (`kubectl get deployment … -o jsonpath`), asserted
  by `TestKindInternalVerification`:

  | Workload | Score | Live `resources` |
  |---|---|---|
  | backend | requests `50m`/`64Mi`, limits `500m`/`256Mi` | `{"limits":{"cpu":"500m","memory":"256Mi"},"requests":{"cpu":"50m","memory":"64Mi"}}` |
  | worker | requests memory `48Mi`, limits memory `128Mi` | `{"limits":{"memory":"128Mi"},"requests":{"cpu":"10m","memory":"48Mi"}}` |
  | frontend | omitted | `{"requests":{"cpu":"10m","memory":"32Mi"}}` |

- Job flow returned `processed:KIND-1790069612641573454`.
- Secret handling: Secrets `backend-env`, `worker-env` and
  `shared-acceptance-db-credentials` exist by name only (values not read); the
  backend Deployment has no plaintext `PGPASSWORD`; the deployment view shows the
  database password as `***redacted***`. Web Console `/ui/deployments/<id>`
  returned 200 and the live-console test passed (the create-and-deploy case is
  skipped by design when an existing deployment ID is supplied).
- The PostgreSQL image archive could not be preloaded (`ctr: content digest …
  not found`); as the script allows, the kubelet pulled the image.

## Cleanup proof

- The EXIT trap deleted the run namespace; the script reported
  `namespace acceptance-kind-20260922093228-12536 is gone`.
- After the run, `kubectl get namespace acceptance-kind-20260922093228-12536`
  returned `NotFound` and
  `kubectl get all,pvc,secret,cm,namespace -A -l orchestrator.io/run-id=kind-20260922093228-12536`
  returned `No resources found`.
- `kind get clusters` still lists `idp-internal`.

## Limitations

- The planner validates shape only. A value that is not a valid Kubernetes
  quantity passes planning and fails at `kubectl apply`.
- The limit-fallback branch of BR-11 (request missing, limit declared) is
  covered by unit tests only; no seeded workload exercises it and kind was not
  rerun after the fix (see "Review follow-up").
- The 33-fixture bundle has no container resources; coverage comes from product
  tests only.
- AWS was not run; the cloud executor contract did not change. The seeded
  acceptance Scores now declare resources, so the next AWS run will use them.

## Review follow-up

Review found that the first policy (missing request always gets the platform
default) produced invalid manifests when a limit was below the default, for
example `limits.cpu: 5m` rendered `requests.cpu: 10m`. BR-11 was revised
first, then the renderer: request declared > limit of the same field >
platform default. Declared values stay verbatim and derived values are not
written into the Candidate Set.

- The kind run above (`kind-20260922093228-12536`) happened **before** this fix.
  Its seeded workloads never hit the limit-fallback branch (backend declares
  both requests; worker declares the memory request and neither CPU request
  nor CPU limit; frontend declares nothing), so the new policy renders the
  same manifests for them. `TestDeployWorkload_RendersDeclaredContainerResources`
  still asserts those manifests through the fake adapters.
- The new branch (`limits.cpu: 5m`, `limits.memory: 16Mi`, both) and a
  declared request above its limit (preserved; the Kubernetes API rejects it at
  apply time) are verified by unit tests only. kind was not rerun within this
  record; a later rerun on the final code is recorded separately in
  [IMP-009 kind rerun](2026-09-22-imp009-kind-rerun.md).
- After the fix, under `backend/`: `gofmt -l .` no output; `go test -count=1
  ./...` pass; `go build ./...`, `go vet ./...` pass; `bash -n
  test/integration/kind-verify.sh` pass. `env REQUIRE_PLANTUML=1 python3
  scripts/check_docs.py` and `git diff --check` pass.
