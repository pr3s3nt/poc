---
id: VERIFY-2026-09-22-IMP010-CONFORMANCE-CATALOG
artifact: verification-evidence
status: evidence
last_reviewed: 2026-09-22
related: I06-05, IMP-010, UC-03
---

# 2026-09-22 — IMP-010 conformance catalog semantics

## Scope

- Iteration: [I06-05](../iterations/M01-contract-hardening/I06-05-imp010-conformance-catalog/README.md).
- Change under verification: `backend/test/conformance/loader.go` no longer
  turns a challenge Definition without criteria into a wildcard `{}`. It leaves
  that Definition out of the challenge catalog instead.
- New tests: `backend/test/conformance/loader_test.go`.
- Not changed: `Definition.Validate`, production planner, domain invariants and
  product catalog fail-fast behavior (D07). UC-03 specification, realization and
  diagrams were not edited.

## Input shapes

Rule source: UC-03 BR-07 and planner challenge `PROBLEM.md` §13.

| Input shape | Expected adapter behavior | Test |
|---|---|---|
| `criteria` key absent | Definition left out of challenge catalog | `TestReadDefinitionsCriteriaShapes/missing` |
| `criteria: []` | Definition left out of challenge catalog | `TestReadDefinitionsCriteriaShapes/empty-list` |
| `criteria: [{}]` | Kept as `Criterion{}`; passes `Validate`; matches any context with score 0 | `TestReadDefinitionsCriteriaShapes/wildcard` |

`TestReadDefinitionsKeepsOrderAroundSkippedDefinitions` checks that skipped
Definitions do not drop or reorder neighbouring Definitions.
`TestProductPlannerRejectsCriteriaLessDefinition` checks that the product
planner still rejects a catalog Definition with `nil` or empty criteria.

## Characterization before the fix

Run before editing `loader.go`:

```bash
cd backend && go test ./test/conformance/ -run 'TestReadDefinitions|TestProductPlanner' -v
```

Result: `missing`, `empty-list` and the ordering test failed because the loader
produced `Criteria:[{}]` for both shapes. `wildcard` and the product planner
guard passed.

## Commands after the fix

Run under `backend/` with `go1.27.1 linux/amd64`:

| Command | Result |
|---|---|
| `go test ./test/conformance/ -run 'TestReadDefinitions\|TestProductPlanner' -v` | pass (3 tests, 3 subtests) |
| `go test -count=1 ./test/conformance/ ./internal/planning/...` | pass |
| `go test -count=1 ./test/conformance/ -run TestPlannerMatchesChallengeFixtures -v` | 33 of 33 fixture subtests `PASS`, none skipped |
| `go test -count=1 ./...` | pass, no `FAIL` |
| `go build ./...` | pass |
| `go vet ./...` | pass |

Documentation checks: `env REQUIRE_PLANTUML=1 python3 scripts/check_docs.py`
and `git diff --check` passed after the documentation updates.

## Limitations

- No fixture in the 33-fixture bundle has missing or empty `criteria`; the
  adapter branch is covered by the loader unit tests, not by a fixture.
- Rejected fixtures still assert rejection status only (IMP-006).
- No kind or AWS run; the change is limited to the test harness adapter.
- IMP-008 and IMP-009 remain open.
