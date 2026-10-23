---
id: IMPL-VISUAL-DEPLOYMENT-REVIEW
artifact: implementation-handoff
status: current
last_reviewed: 2026-10-23
---

# Visual deployment review — implementation handoff

## Status and authorization

User approved the desktop prototype and requested docs/realization first, then
one new tmux Codex session using exactly `gpt-6-sol`, reasoning `medium`, to code.
This is a scoped feature task, not a request to continue any roadmap. Current
state: design ready for implementation; IMP-029 open. No AWS/Kubernetes mutation,
production deployment, retained database reset or extra coding agents authorized.

## Read order / canonical sources

1. [ADR-019](../architecture/decisions/ADR-019-visual-deployment-review.md).
2. [UC-05 specification](../usecase/UC-05/specification.md) VIS-* and
   [realization](../usecase/UC-05/realization.md), sequence/VOPC/UI/API mapping.
3. [UC-09 specification](../usecase/UC-09/specification.md) OBS-* and
   [realization](../usecase/UC-09/realization.md), sequence/VOPC/UI/API mapping.
4. [Shared review contract](../architecture/contracts/deployment-review.md),
   [render contract](../architecture/contracts/workload-rendering.md),
   [database schema](../architecture/database/schema.md) review additions.
5. UC-06/07/08 REV-*; existing operation contracts/ADR-017 for execution guards.
6. [Accepted desktop prototype](../../frontend/design/deployment-preview/README.md).
   Reuse visual interactions and actual Console styling; never copy fake data,
   timer/scheduler/scenario/failure controls into product.

## Implementation order and entry points

- Extend Go safe review values/ports and existing persistence fields; implement
  schema baseline/digest and JSON/PostgreSQL parity without resetting user data.
- Source projection: registry verified cache reader, digest/path/bounds and
  structural redaction; no request fetch, Terraform or provider invocation.
- Preview rendering: extend installed score-k8s adapter/shared normalization with
  offline symbolic review; no hardcoded replacement renderer. Build scoped
  baseline resolution and bounded safe artifact diff inputs.
- Assemble review for standalone `application/preview` and final pending
  `application/pending` graph; serialize explicit safe DTOs in delivery/http.
- Capture reviewRequestId and desired plan review at admission; capture sanitized
  manifest on workload render; preserve data through status updates/reopen.
- Extend deployment query with safe nodeProgress/review and scoped run filter.
  Existing progress/blocked projections are starting points, not authority to
  fake unavailable facts or add scheduler concurrency.
- React + strict TS: reusable graph/inspector/diff/progress within existing
  ApplicationHome, ScorePreview and DeploymentDetail UI; exact run correlation,
  stale-response and GET polling behavior. Preserve current editing/Deploy guards.

## Acceptance checklist (all pending)

- [ ] Internal existing cluster + namespace/resource graph, update/retain, safe source and manifest diffs.
- [ ] Definition-selected cloud graph, VPC/EKS only when present, managed DB/cache outside EKS, per-node account/region.
- [ ] Exact source/bundle pin, symbolic output provenance, renderer parity, strict secret/path/content limits.
- [ ] Valid baseline provenance, unavailable old history, no mutation from Preview/query, no arbitrary source download.
- [ ] Persist/reopen desired/applied review safely; failed/partial workload cannot replace successful baseline.
- [ ] Actual stage/time/wait/blocked/readiness and run association; no timer/fake-ready state.
- [ ] Desktop Console parity, red/green code diff, copy-after, expand, keyboard; no new mobile design.
- [ ] Focused Go tests/build, frontend typecheck/changed-file lint/focused tests/build, persistence parity with disposable PostgreSQL if available/authorized, docs/diff and changed diagrams.
- [ ] Browser flow uses product HTTP DTOs; explicitly label mocks. No AWS/kind run or claims from fixture tests.

Writer edits backend/frontend/tests/scripts in its own checkout only. Coordinator
owns canonical docs, review and commit/push. Do not modify docs/AGENTS/prototype
or commit/push from writer. Report a concrete blocker rather than weakening a
contract, hiding failed checks, substituting a model or adding scope. Keep session
and work intact on interruption. Runtime/session receipt is recorded separately
in the dated verification handoff once launch is confirmed.

## Current coding session

[Design/launch receipt](../verification/2026-10-23-visual-preview-design-handoff.md)
records verified CLI model/effort/permissions and initial quota retry.
Writer is active in tmux `preview-gpt6-sol`, branch `codex/visual-preview`,
checkout `/home/thanhnt1/projects/preview/poc-preview-writer`, based on `ee170c1`.
Implementation and independent product validation remain pending.
