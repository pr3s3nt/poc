---
id: GIT-RENDER-BUNDLE-IMPLEMENTATION
artifact: implementation-handoff
status: current
last_reviewed: 2026-10-24
---

# Git render bundles — implementation handoff

This is the executable work breakdown for accepted required design, not completion
evidence. [ADR-021](../architecture/decisions/ADR-021-git-render-bundles.md) owns
decisions; [contract](../architecture/contracts/git-render-bundles.md) owns engine/
source/API shapes; UC specifications own behavior. [IMP-034](deviations.md#imp-034--git-owned-render-bundles)
tracks the current gap. No roadmap continuation or approval checkpoint is implied.

## Read in order

1. Root AGENTS.md, INDEX, ADR-021 and Git contract (RB-C01..07).
2. UC-03/05/06/07 context, specification RB sections, realization and sequence;
   workload-rendering contract's compatibility boundary.
3. Database schema/ERD Git bundle addition; shared UI design and UC-03 UI.
4. [Acceptance runbook](../operations/git-render-bundles-acceptance.md).
5. Implementation paths below. Do not infer a different product contract from
   today's installed-only code. Do not implement unrelated open issues.

## Code map and required work

| Area | Existing anchors | Required result |
|---|---|---|
| Immutable source registry | `backend/internal/adapters/terraform/registry/git_inspect.go`, Git transport/cache under that registry | Share hardened source transport where suitable; new render-bundle domain/port/registry. No Terraform HCL/state or release-ID coupling |
| Domain/validation | `backend/internal/domain/resource/rendering.go`, `backend/internal/application/catalog/service.go` | Scoped verified Git metadata, union with legacy installed selection, literal typed `render_inputs`, same Create/CAS/criteria validation |
| API | `backend/internal/delivery/http/resource_definitions.go`, `render_bundles_test.go` | RB-C07 inspect/detail/catalog operations, strict DTO/error/RBAC/scope, existing Definition CRUD |
| Persistence | `backend/internal/adapters/postgres/`, memory/JSON store, `backend/internal/ports/` | Registry table + immutable/scoped operations, fresh-baseline/grants/fingerprints, retained plan deserialization; no retained reset |
| Plan/pending | `backend/internal/planning/rendering.go`, `intent.go`, `backend/internal/application/pending/preview.go`, deployment snapshot loader | Read verified metadata, pin source/runtime/effective inputs, bundle-only pending updates, unchanged infrastructure identity |
| Rendering | `backend/internal/adapters/scorek8s/renderer.go`, `provisioners.yaml`, `backend/internal/adapters/kubernetes/renderer.go`, `backend/internal/ports/execution/execution.go` | Git engine with neutral input and killable template worker; native output-only score-k8s provisioners; ordered patches; no hidden native policy render on Git path |
| Secrets/invariants | same renderers, `backend/internal/application/deployment/service.go` | Extract shared private secret bridge; typed authorized references; independent semantic guard instead of whole-manifest native equality |
| Preview/review | `backend/internal/application/deploymentreview/review.go`, `baseline.go`, `frontend/src/features/preview/DeploymentReview.tsx` | Pinned symbolic render, safe source/commit provenance and manifest diff for supported custom fields; unknown/redacted still truthful |
| Console | `frontend/src/features/platform/RenderingTemplatesPage.tsx`, `workloadTemplate.ts`, `metadata.ts`, Git module source component and typed-input editors | Git default + existing bundle selection, inspect/pin/schema form, source/version/edit/retry/CAS round-trip, VI/EN catalogs |
| Runtime packaging | `backend/Dockerfile`, root Compose + relevant local/EC2 overlays and bootstrap flags | Durable render cache; explicit score-k8s capability; source-independent Git execution; legacy mode preserved |
| Author reference | new `backend/render-bundles/standard/` | Complete standard bundle, examples and expected render fixtures; no instructions to copy Go into a Git repo |
| Verification | existing `full-app-kit.mjs`, `full-app-flows.mjs`, `preview-kind-live.mjs`, `preview-kind-check.mjs`, `backend/test/integration/v2run/` | New isolated real runner and end-user browser scenario; reuse assertions/config/helpers without copying obsolete profile or recording requirements |

Paths for new files/packages can follow repository conventions; contract/behavior
and evidence are mandatory. Shared transport extraction must preserve Terraform
tests; changing the Terraform product flow is outside scope.

## Ordered checkpoints (agent proceeds without approval)

1. **Registry and persistence:** scoped immutable verified records, durable cache,
   source inspection and exact API shape. Tests for malformed/unsafe source,
   pin/digest mismatch, duplicate/CAS/scope and default semantics.
2. **Engine/reference bundle:** separate secret bridge and semantic guards;
   normalization/defaults/provisioners/patches all read from pinned cache. Standard
   bundle reproduces current explicit Score/Secret/service behavior. Test helper
   determinism, CLI provisioner priority, patch semantics and timeout reaping.
3. **Plan and history:** snapshot metadata, source/input pin, no Git/provider I/O
   in planner/Preview, stale admission and in-flight immutability, applied before
   side, restart and explicit old-pin re-selection.
4. **Console:** use current shell and Source/Inspect pattern. Git succeeds with
   no installed bundles. Required typing/JSON round-trip, old bundle edit, safe
   errors and source/short commit. No hard-coded translated strings.
5. **Real acceptance:** prepare the isolated environment and execute A→B→restart→A
   and product cleanup as below. Diagnose/fix/re-run affected checks on failure;
   do not count retries of an unfixed failure as progress or switch to fakes.
6. **Handoff:** review diff, update code map/CURRENT_STATE/deviation/traceability,
   add dated actual evidence, required checks, task-only commit/push. Report issue,
   commit, live scenario/evidence and cleanup. Do not claim full delivery if the
   real-adapter acceptance is blocked. Do not close another issue automatically.

## Required focused automated coverage

Tests must prove behavior, not mirror helper implementation. Use real PostgreSQL
for persistence/CAS and zero skipped required integration cases. Fakes remain
allowed in existing unit tests, never in acceptance or as substitutes for it.

- Inspection: full ref resolution/content hash/repeated inspect, no hooks/path
  traversal/symlink/remote include/cmd provisioner, bounds, schema/helper/version
  rejection, cross-org access, late reply and no Definition/provision side effect.
- Contract: explicit false/0/empty values preserved, absent-only defaults,
  type/unknown-key checks, reserved identity/Secret/Score guard, literal dollar
  escaping and repeated deterministic output, provisioner precedence.
- Engine: changing normalization/defaults/provisioner/patch source changes the
  expected supported rendering outcome or binding without recompiling Go. Include
  separately authored valid normalizers, not just editing comments. Compare the
  standard bundle with current native semantics for all supported Score fields,
  VSO and legacy Agent paths. Agent metadata must not leak into safe previews.
- Pending/admission: bundle/input-only UPDATE, source HEAD change alone no change,
  stale Preview, no runtime Git, cache repair same pin, unavailable/drift fail before
  provision, no fallback, cancel reaps worker, admitted execution keeps original pin.
- Persistence: source pin/input round-trip across restart, immutable metadata,
  same-org associations and SQL privileges, old installed selections still load,
  applied manifest baseline and original removal identity remain unchanged.
- UI: create/edit/JSON/per-pin draft/retry/CAS, missing installed registry with
  available Git source, safe version/status, language keys, supported manifest
  customization displayed with honest unknowns. Keep existing test suite passing.

Commands for the implementation task:

```bash
python3 scripts/check_docs.py
git diff --check
# Runner must supply a disposable real PostgreSQL instance, then:
cd backend && REQUIRE_POSTGRES=1 go test ./... && go build ./...
cd ../frontend && npm run typecheck && npm run lint && npm test && npm run build
# bash -n each changed/new shell script; validate changed PlantUML and regenerate PNGs.
```

The exact new live entrypoint must be
`bash backend/test/integration/git-render-bundle-kind.sh`; implement it with
`--evidence <new-absolute-directory>` and optional `--run-id <safe-id>`. It prepares
all owned prerequisites, invokes `frontend/test/e2e/git-render-bundle-kind.mjs`,
verifies business/runtime assertions and cleanup, exits nonzero on any missing
required assertion/skip. Rerun with a new ID after diagnosing/fixing; resume only
its own recorded resources and pins.

## Documentation preparation evidence (not feature acceptance)

On 2026-10-24, the documentation author read the actual shell/styles/components
and opened `/ui/platform/rendering-templates` plus Resource Definitions → Create
in Chromium against `orchestrator-manifest` on localhost:13003. Existing Git
module authoring provides the source/inspect pattern; rendering page provides
criteria/placement/edit controls. No catalog mutations were made. This installed
stack exposes no available renderer and is **not** a valid acceptance environment.

UI review found long repeated introductory/help text on rendering authoring.
For the touched source section, keep labels/actions/status visible and put source
limitations/technical pins in help/details; preserve shell, navigation and matching
controls. Future agent must compare the implemented screens visually with these
product references at 1512px and 1280px and record the result. No UI implementation
or live feature test was performed during this documentation task.
