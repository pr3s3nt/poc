---
id: VERIFY-2026-09-22-IMP008-DELTA-SNAPSHOT
artifact: verification-evidence
status: evidence
last_reviewed: 2026-09-22
related: I06-06, IMP-008, UC-05, UC-06, UC-07, UC-09
---

# 2026-09-22 — IMP-008 Deployment Delta Snapshot

## Scope

- Iteration: [I06-06](../iterations/M01-contract-hardening/I06-06-imp008-delta-snapshot/README.md).
- Canonical inputs: Deployment Delta Snapshot contract in
  [domain objects](../architecture/domain/domain-objects.md),
  `deployment_delta_snapshots` in [database schema](../architecture/database/schema.md),
  UC-05 BR-05/06, UC-06 BR-10 and UC-07 BR-07. No requirement, business
  rule, realization, architecture, schema or diagram was edited. Only the
  "Trạng thái implementation hiện tại" sections of the UC-05, UC-06 and UC-07
  specifications were brought in line with the implemented Delta.
- Change under verification:
  - Typed `DeploymentDeltaSnapshot`, `DeltaDocument`, `ModuleDelta` and
    `JSONPatchOperation` in `backend/internal/domain/deployment/delta.go`.
  - `DeltaBuilder.BuildHumanitecDelta`, `DiffDeploymentSets`,
    `ApplyHumanitecDelta` and `VerifyDelta` in `backend/internal/planning/delta.go`.
  - `backend/internal/planning/jsonpatch` diffs objects by lexical key order
    and arrays by index, removes the tail from the highest index and appends
    with `/-`; apply supports array add/remove.
  - `Plan.Delta []jsonpatch.Op` is replaced by a transient typed
    `Plan.Delta deployment.DeltaDocument` that is not serialised into the
    persisted plan. Plan hash inputs are unchanged.
  - `DeltaSnapshotRepository` port; JSON snapshot store keeps
    `deploymentDeltaSnapshots`, writes each Snapshot once, validates its
    `documentHash` and enforces one Snapshot per Deployment. `SaveDeltaSnapshot`
    and `GetDeltaSnapshot` deep-copy the Snapshot through its JSON form, so
    stored state never aliases a caller's value.
  - `DeploymentService` saves the Snapshot with the Candidate Set, plan and
    Deployment in transaction A and sets `Deployment.DeltaSnapshotID`.
  - UC-09 `QueryService` returns `delta` and `deltaDocumentHash` from the
    Snapshot instead of `plan["delta"]`.
- Not implemented (out of scope): D05 standalone/mutable Delta, Delta
  create/update/archive API, deploy by `delta_id`/`set_id`, async/full-set/
  incremental mode, PostgreSQL adapter or data migration, IMP-009.

## Characterization before the change

`TestCharacterize_FlatWholeDocumentDelta` was added and run before any
implementation change:

```bash
cd backend && go test -count=1 ./internal/planning/ -run TestCharacterize -v
```

Result: `PASS`. It confirmed that `Plan.Delta` was one RFC 6902 patch rooted at
the whole Deployment Set (`/modules/...`, `/shared/...`) and that arrays were
replaced whole. The baseline `go test -count=1 ./...` passed and 33 of 33
fixtures passed. The test was deleted once the flat Delta was removed, because
the behavior it pinned no longer exists.

## Contract tests

| Requirement | Tests |
|---|---|
| Module add, remove (sorted IDs), update relative to module | `TestDelta_ModuleAddCarriesFullModule`, `TestDelta_ModuleRemoveListsSortedIDs`, `TestDelta_ModuleUpdateIsRelativeToTheModule`, `TestPlan_RemoveWorkloadDeltaListsModule` |
| Shared add/update/remove relative to shared object; unchanged entries preserved | `TestDelta_SharedPatchIsRelativeToSharedObject`, `TestDelta_CombinesModulesAndShared` |
| Nested object diff and lexical key order | `TestDiffWalksObjectKeysInLexicalOrder`, `TestDiffNestedObjectRecursesInsteadOfReplacing` |
| Array replace by index, append `/-`, tail remove highest first | `TestDiffArrayReplacesCommonIndexes`, `TestDiffArrayRecursesIntoCommonObjectElements`, `TestDiffArrayAppendsWithDashPath`, `TestDiffArrayRemovesTailFromHighestIndex`, `TestDelta_ModuleArrayDiffIsIndexBased` |
| No-op `{}` | `TestDelta_NoOpIsEmptyObject`, `TestPlan_RedeployingSameScoreIsNoOpDelta`, `TestDeployWorkload_RedeployWithoutChangePersistsEmptyDelta`, `TestDeltaDocument_EmptyBranchesAreOmitted`, fixture `01-noop` |
| `base + delta = candidate` | `VerifyDelta` in every plan; `TestPlan_DeltaKeepsInvariant`; conformance `assertDelta`; `TestDeployWorkload_PersistsOneDeltaSnapshotPerDeployment` on persisted sets |
| Apply rejects inconsistent documents | `TestApplyHumanitecDelta_RejectsInconsistentDocuments`, `TestApplyRejectsReplaceOfMissingKeyAndBadIndex`, `TestDeltaDocument_ValidateRejectsAmbiguousDocuments` |
| Snapshot save/load, immutability, fingerprint | `TestDeltaSnapshotSaveReloadAndImmutability`, `TestDeltaSnapshotRejectsTamperedHash`, `TestTransactionRollsBackDeltaSnapshot`, `TestDeploymentDeltaSnapshot_HashIsDeterministicAndSurvivesReload` |
| Repository immutability against aliasing | `TestDeltaSnapshotSaveDoesNotAliasCallerValue`, `TestDeltaSnapshotGetDoesNotAliasStoredState` |
| One-Deployment association | `TestDeploymentReferencesExactlyOneDeltaSnapshot`, `TestDeployWorkload_PersistsOneDeltaSnapshotPerDeployment` |
| Plan hash/serialization determinism; no delta in persisted plan | `TestPlan_DeltaIsHumanitecShapedAndDeterministic` |

The conformance harness now also loads `expected/delta.yaml` and compares the
Delta document, including operation order, for the 27 accepted fixtures. A
mutation check that reversed object key order made 3 fixtures fail
(multi-operation patches), which shows the assertion is active; the mutation
was reverted.

## Commands after the change

Run under `backend/` with `go1.27.1 linux/amd64`:

| Command | Result |
|---|---|
| `gofmt -l` on changed Go files and on the module | no output |
| `go test -count=1 ./test/conformance/ ./internal/planning/...` | pass |
| `go test -count=1 ./test/conformance/ -run TestPlannerMatchesChallengeFixtures -v` | 33 of 33 fixture subtests `PASS`, no `SKIP` or `FAIL` |
| `go test -count=1 ./...` | pass, no `FAIL` |
| `go build ./...` | pass |
| `go vet ./...` | pass |
| `go vet -tags integration ./test/integration/...` | pass |

Documentation checks: `env REQUIRE_PLANTUML=1 python3 scripts/check_docs.py`
and `git diff --check` passed after the documentation updates.

## Limitations

- The 33-fixture bundle has no array diff, so array semantics are covered by
  product tests only.
- Rejected fixtures still assert rejection status only (IMP-006).
- The Deployment is still created in `PLANNING` before the Snapshot exists, and
  a planning failure leaves a `FAILED` Deployment without a Snapshot. The store
  requires a Snapshot only when a Deployment leaves `PLANNING`, so the schema
  `NOT NULL` rule is not enforced yet (IMP-011).
- Persistence is the in-memory map plus JSON snapshot file (IMP-001). Snapshot
  files written before this change have no Snapshot entries; their deployments
  show `delta: null` in UC-09. No migration was done.
- Snapshot `application_id` ownership remains the D08 decision.
- No kind or AWS run; no external mutation was in scope.
- IMP-009 remains open.
