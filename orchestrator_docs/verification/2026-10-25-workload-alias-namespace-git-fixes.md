---
id: VERIFY-2026-10-25-WORKLOAD-ALIAS-NAMESPACE-GIT-FIXES
artifact: verification
status: current
last_reviewed: 2026-10-25
---

# Shared aliases, namespace recreation and delayed Git source metadata

The user requested corrections for F3/F4/F5 found on `main` at `f3dc811`,
followed by commit/push. Codex implemented and reviewed the changes; the existing
Claude tmux session `kiemtra` executes validation and real-kind acceptance.
Product code remained frozen during final acceptance. E2E images were built
from working-tree fingerprint `d93e3455` (backend `509b5704`, frontend
`dfd79bfb`); only documentation changed after that tested source revision.

## Changes

| Finding | Correction |
|---|---|
| F3: shared resource names become `shared_N` after Deploy | Private per-workload `resourceAliases` stores original shared names and reference paths. Reconstruction restores a reference only when it still resolves identically, and never authorizes literal readback. Metadata stays outside planner documents/hashes; legacy sets keep the generated-name fallback. |
| F4: switching a saved template to Git before metadata arrives leaves URL/path blank | Pending initialization reads the saved bundle independently of the active mode. Late catalog/detail responses preserve a typed ref and focus; source edits, cancellation and another edit invalidate initialization. |
| F5: CREATE after namespace DESTROYED reuses an obsolete baseline | Immutable encrypted baseline handles include instance and namespace UID. Cleanup prefers the current UID's proof and retains exact historical fallback. Missing/corrupt proof, a replaced ServiceAccount or foreign objects remain guarded. |

Design/data references: [UC-16 BR-17](../usecase/UC-16/specification.md),
[Terraform state contract](../architecture/contracts/terraform-state.md),
[schema](../architecture/database/schema.md#deployment_sets),
[code map](../implementation/code-map.md#verified-feature-defect-fixes--f3f4f5)
and [traceability](../traceability/matrix.md#f3f4f5-regressions).

## Automated checks executed by Claude

- Initial backend `go test ./...` and `go build ./...`: passed, 68 packages.
  Final affected packages were then checked again with real PostgreSQL and race
  coverage after the binding-path refinement.
- Real PostgreSQL affected packages: adapter/snapshot/persistence, recovery,
  workloadconfig, pending, deployment, configuration and environment passed.
  Terraform executor completed 85/85 top-level tests with no skips across two
  groups: the original package run reached its cumulative 10-minute budget
  after 66 passes, and the remaining 19 passed in a separate 253.7-second run.
  No assertion or test was removed to complete the groups.
- `-race -count=3`: workloadconfig, pending, recovery, environment and
  deploymentreview passed; focused namespace/baseline coverage passed 57 test
  invocations with zero failures/skips/race reports.
- Full frontend: 76 files / 918 tests passed with `--maxWorkers=2`. The default
  parallel run hit an existing `observability.test.tsx:78` timing failure,
  independently reproduced on unmodified `f3dc811`; isolated runs passed 5/5.
  After the final focus-preservation refinement, the 40 related frontend tests,
  typecheck, lint and production build passed again.
- Final gate on the final source revision passed: `go test ./...` (68 packages),
  `go build ./...`, full frontend `npm test -- --maxWorkers=2` (76 files,
  918/918 tests), documentation checker (415 Markdown files / 359 artifact IDs)
  and `git diff --check`. The code-only fingerprint stayed `678dc130` through
  acceptance and this final gate; only documentation changed afterward.
- Full `REQUIRE_POSTGRES=1 go test ./...` and AWS were not run; real PostgreSQL
  coverage targets the affected packages listed above.

## Real Kubernetes acceptance

Console: retained Compose project `e2e-main`, loopback port 18872. Cluster:
explicit `kind-k8s-4f` context. New app/environment fixtures are used; existing
volumes and the pre-fix ownership artifact remain intact.

| Scenario | Result |
|---|---|
| F3: two workloads use different aliases for one shared Redis, plus a private Redis | Passed API/editor readback, unchanged-save/no-op Preview and backend restart. |
| F5: product cleanup, same-instance recreation with a different namespace UID, first-attempt success, repeated twice | Passed both cycles. Three namespace incarnations retain separate immutable baseline artifacts; prior digests stay unchanged. Each intermediate cleanup verified namespace absence and zero leftover PVs. |
| F4: immediate edit/source switch and a controlled delay of real metadata replies while typing a ref | Passed 16/16 timing trials, an additional controlled hold of both real list/detail GETs with ref/focus preserved, and normal inspect/save (Definition v2 to v3). |
| Final cleanup of acceptance workloads/resources/namespace | Passed: workloads removed, both Redis resources and namespace DESTROYED through UI; namespace absent and zero PVs. Disposable PostgreSQL container/volume and baseline comparison worktree were also removed. |

Two harness issues are retained in the evidence: the first setup waited for an
old app's Preview response; the initial cleanup did not wait for each workload's
DELETE response/state. The script was corrected to validate the new scope and
each row before continuing. These attempts are not counted as product passes.

The namespace from the pre-fix run lacks proof for its recreated incarnation.
The correction does not infer ownership from its live name/shape or rewrite the
old artifact. A new UI cleanup attempt still failed at CHECKING, retained the
same live namespace UID and left the old baseline digest unchanged. This legacy
namespace remains a separately recorded limitation. The `e2e-main` stack with
the fixed images and Metrics Server remain available.

Visual review compared the existing Console references with the new deployed
editor and Git-source screenshots. Alias selections show the original names;
the delayed Git source fills URL/path while the typed ref remains focused.
Existing shell, controls and text are reused; no additional product guidance or
layout was introduced.

## Evidence

Claude session: `62bd5b9e-457e-4f39-9e17-04e3742e1b42`. Local scratchpad
`testfix/` contains `go-test-all.log`, `go-pg-affected-run1.log`,
`go-pg-tfexec-remaining.log`, `go-race-focused.log`, `go-race-namespace.log`,
`fe-test-maxworkers2.log`, `fe-final-focused.log`, build logs and image/diff
fingerprints. The final gate is recorded in `final-go-test.log`,
`final-go-build.log`, `final-frontend-test.log` and `docs-post-report.log`.
`testfix/e2e/` contains runtime facts, assertions and screenshots;
`evidence/results.txt` retains every attempt. Private credentials, Terraform
state and ownership artifacts are not committed.

Final business acceptance is 13/13: F3 has four checks, F5 has six, F4 has three.
The 16 timing trials are subcases of the F4 source check; phase-completed markers
and the legacy guard observation are not added to this total.
