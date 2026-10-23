---
id: VERIFY-VISUAL-PREVIEW-IMPLEMENTATION-20261023
artifact: verification-record
status: evidence
last_reviewed: 2026-10-23
---

# Visual deployment review — implementation and independent review

## Outcome and execution boundaries

The approved desktop review is implemented in the existing Console: final
multi-workload graph, real dependency edges/placement, sanitized pinned Terraform
source/inputs, symbolic selected score-k8s manifests, red/green code changes and
persisted per-node progress. Each Deploy click has an environment-scoped UUID;
queries correlate exact operation/deployment records. Desired review is retained
in the admitted plan; safe rendered review is captured before Apply and only an
exact READY workload/resource association becomes a successful baseline.

The requested tmux writer used `gpt-6-sol`, medium, session
`01a1665a-0b6e-7802-b063-59facdc4610a`, then exited 0. Coordinator independently
reviewed and continued code/test fixes under the user's explicit follow-up
request. The writer worktree and transcript remain available; no extra
coding agent or model substitution was used. Initial launch details remain in
[the receipt](2026-10-23-visual-preview-design-handoff.md).

All execution adapters in the browser fixture were fake. HTTP, authentication,
application/Definition/workload forms, planner, installed score-k8s renderer,
persistence projections and Console were product code. PostgreSQL verification
used a separately labeled Docker fixture with tmpfs data and loopback-only port;
Redis tests used extracted task-local binaries. No retained database, real
cluster or cloud account was mutated. No real Terraform provider plan or
provisioning claim follows from these results.

## Independent fixes

- Compare pending review with the original batch base, not its final intermediate
  step; include explicit workload/file removals and stable cross-revision paths.
- Reject nested values in scalar manifest fields; hide unclassified literal env
  inputs before rendering. Replace symbolic namespace only in its structural
  field, preserving ordinary image/text values that happen to contain the sentinel.
- Use exact generation, current resource association, plan hash and READY workload
  snapshot for historical baseline. Missing safe history is unavailable.
- Query the exact scoped operation without recovery/Sweep writes or latest-20
  truncation; enforce case-insensitive UUID identity and immutable deployment
  correlation consistently in memory and PostgreSQL.
- Match observed node pins before merging batch progress; newer failures and
  contradictory facts cannot be hidden by older success. Full renderer digest and
  placement/module pins are compared; an unrelated later batch peer without a
  workload snapshot cannot erase the original workload's observed READY state.
- Draw actual graph edges and placement groups; label existing clusters as
  bindings. Collapse technical tables, shorten visible names, add keyboard tabs,
  bounded diff, removal lines, elapsed timestamps and clipboard failure handling.

## Checks observed

| Check | Result / evidence |
|---|---|
| Frontend full suite, bounded workers | PASS: 62 files, 816 tests; `npm test -- --maxWorkers=2 --reporter=dot` |
| Final affected Console suites | PASS: ApplicationHomePage/review/observation 31 tests; later ScorePreview/review 29 tests; final observation/ApplicationHome 27 tests, including both added correlation regressions (the final source contains two more focused cases than the 816-test checkpoint) |
| TypeScript strict / production build | PASS; build retains the existing advisory about a JavaScript chunk over 500 KiB |
| Changed-file ESLint | PASS, including task prototype and local browser runner |
| Full frontend lint | FAIL: 46 preexisting errors in resource-definition-module-forms, resource-definition-v3 prototypes and ec2-role-real.mjs; these five files are unchanged by this task |
| Go build | PASS: `go build ./...` including disposable browser fixture |
| Affected backend race suite | PASS: deploymentreview, pending, preview, deployment, store, postgres, persistence conformance, registry and HTTP |
| Final regressions | PASS: symbolic namespace with sentinel-like image unchanged, existing-cluster binding, uppercase UUID and immutable PostgreSQL correlation; envops/pending/review suites pass |
| PostgreSQL | PASS: actual disposable PostgreSQL16, persistence snapshot parity/reopen and terminal immutability; race suite postgres 193.285s, no skips |
| Full Go suite and failed-package retry | PASS after failed-package retry: 60 packages and 1,747 named test/subtest outcomes across the original full run plus retries, zero skipped test cases; tfexec completed in 892.263s with the larger timeout. Later focused regressions also pass |
| Final browser | PASS: 21 assertions at desktop 1512×1100, actual HTTP/UI and selected score-k8s, fake execution only |
| Docs / diff | PASS: `python3 scripts/check_docs.py` (370 Markdown files / 319 artifact IDs) and `git diff --check`; the 13 design PlantUML files were rendered during the design stage, unchanged by this code review |

The initial unconstrained frontend run had timing failures under simultaneous
Go load; the complete bounded-worker rerun passed all 816 tests. The initial
full Go run had fixture failures (Redis absent; trust-auth PostgreSQL cannot
reject a wrong password), one outdated exact response-key assertion, and the
unrelated long tfexec package hit Go's default 10-minute timeout while tests
were progressing. The assertion was updated for the additive `review` field,
task-local Redis was supplied, host password authentication was enabled only
inside the disposable fixture, and only failed packages were rerun with a
30-minute package timeout. Original failure logs remain retained; assertions
and product guards were not reduced.

## Browser coverage and review images

The tracked runner is `frontend/test/e2e/deployment-review-local.mjs`, backed by
`backend/test/fixtures/deployment-review`. It registers scoped Definitions and
creates two Workloads through Console forms, verifies their common final graph,
executes the product pending deployment using fake adapters, checks one exact
operation/two exact deployment records, confirms persisted READY review, updates
an image, verifies the exact prior applied deployment as baseline, red v1/green
v2 lines, safe copy and expand/collapse. Historical detail remains associated
with that deployment despite newer drafts.

A separate preview-only Application registers Definition-selected VPC/EKS/Aurora
and the installed workload renderer. The planner returns those exact nodes and
module pins/account/region; Aurora has no Kubernetes placement. The browser
renders the real returned graph, without cloud execution.

- [Internal preview](artifacts/visual-preview-20261023/new-preview.png).
- [Red/green update diff](artifacts/visual-preview-20261023/update-diff.png).
- [Cloud preview](artifacts/visual-preview-20261023/cloud-preview.png).
- [Persisted deployment detail](artifacts/visual-preview-20261023/deployment-detail.png).

The fake bootstrap lacks a Terraform-state cleanup reader; its existing resource
inventory panel reports unavailable. That is a fixture limitation and is separate
from the graph/review assertions. It is not represented as successful inventory
or cleanup against real resources.

## Limits, evidence and cleanup

- Review is a partial, redacted, non-executable projection. HCL attributes and
  comments and unclassified manifest literals are hidden; equality of hidden
  values is not asserted. Unsupported history remains unavailable.
- IMP-029 retains `.tf.json`: the existing registry rejects that format at
  registration; this task does not add registry format support or bypass it.
- ElastiCache follows a selected supported Definition as any other external
  resource; the browser selects Aurora and does not claim ElastiCache execution.
- Schema fields are in the fresh PostgreSQL baseline. Existing installations
  require operator-managed upgrade; none was reset or migrated here.
- No mobile implementation, real Kubernetes/AWS acceptance, production deploy or
  external-service notification was included.

Private execution evidence is retained at
`/tmp/visual-preview-review-20261023/`: full Go JSON logs, original failures,
failed-package retry, race/check logs, browser results/screenshots and fixture
cleanup receipt. Writer evidence remains at
`/tmp/poc-visual-preview-writer-20261023/`.

Disposable PostgreSQL cleanup verified: the task's label-matched container and
private network were removed; no retained database was touched. Task HTTP/browser
fixture processes were stopped. Final inspection retained existing tmux sessions
`demo`, `deploy_k8s` and `preview` without modification. The original writer tmux
target `preview-gpt6-sol` was no longer listed; its exit-0 receipt, CLI session ID,
worktree and transcripts are preserved. No session was closed by this review.

Implementation commit: `f0de74d` on `codex/visual-preview`, based on `ee170c1`;
merged into `features/preview` with the coordinator's launch receipt and final
documentation/evidence. No unrelated user-owned files were staged.
