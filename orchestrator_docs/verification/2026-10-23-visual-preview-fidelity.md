---
id: VERIFY-VISUAL-PREVIEW-FIDELITY-20261023
artifact: verification-record
status: evidence
last_reviewed: 2026-10-23
---

# Deployment review — correction against the approved desktop prototype

## Trigger and outcome

User reported that the running product did not match the approved prototype.
The earlier functional handoff did not establish visual fidelity. Coordinator
opened the actual prototype screenshots and runtime captures side by side,
recorded IMP-030 before changing code and corrected the integration directly.
Base: `ebfc0c1`, branch `features/preview`. No backend or scheduler was changed.

The review now has its own full-width Application panel, matching the prototype's
summary/graph/inspector/action hierarchy. Nodes have type icons, subtitle and
separate intent/state. The inspector has a readiness card, readable properties
from safe server artifacts, grouped provider dependencies with every reason,
and collapsed technical metadata. Code has file selection, one active file,
diff/after modes, line counts, safe copy and expand. Changes include bounded
red/green before/after excerpts. Dependency levels include workloads and do not
claim scheduler concurrency. Unknown placement is explicitly unresolved.

Admission switches from Preview to one observed graph while preserving the
existing disabled busy action, stale-response guards and exact correlation.
The same shared component improves standalone Preview and historical detail.
No prototype scenario controls, timers, fake readiness or mobile work was added.

## Actual visual comparison

At 1512px the runtime review panel is 1100px wide, with a 702px graph and 340px
inspector, matching the approved prototype's proportions. At 1280px the panel is
940px wide, with a 572px graph and 310px inspector. Both Internal and cloud have
no document-level horizontal overflow. Graph contents fit by measured scaling;
large graphs retain a readable minimum scale and can scroll. Different real
node counts and redacted artifacts are not replaced with prototype fixtures.

Coordinator visually inspected these real runtime captures against the approved
`frontend/design/deployment-preview/desktop.png` and code/cloud captures:

- [Internal overview, 1512px](artifacts/visual-preview-fidelity-20261023/internal-1512.png).
- [Internal overview, 1280px](artifacts/visual-preview-fidelity-20261023/internal-1280.png).
- [Cloud overview](artifacts/visual-preview-fidelity-20261023/cloud-1512.png).
- [Code with file tabs](artifacts/visual-preview-fidelity-20261023/code-1512.png).
- [Expanded red/green diff](artifacts/visual-preview-fidelity-20261023/code-expanded-1512.png).
- [Changes before/after](artifacts/visual-preview-fidelity-20261023/changes-1512.png).

This establishes coordinator comparison and the revised running result; the
user's re-review is still pending. It is not a claim of pixel identity for
arbitrary data or of new user approval.

## HTTP Tailscale defect found during verification

Actual Chrome at `http://100.77.224.24:18093` reports `isSecureContext=false`,
`crypto.randomUUID` undefined and `crypto.getRandomValues` available. The old
Deploy click therefore failed on this local review URL. Added a cryptographic
RFC4122 v4 fallback using getRandomValues; secure contexts keep native randomUUID.
There is no Math.random fallback and the ID remains fresh per explicit click.
Clipboard is also unavailable in this HTTP context; its existing safe manual-copy
message remains visible instead of an unhandled error. Native copy is covered
by the component checks and earlier localhost browser execution.

## Validation

- Full Console checkpoint: `npm test -- --maxWorkers=2 --reporter=dot` PASS,
  62 files / 818 tests. Later focused checks cover the final presentation and two
  added regression cases: 5 files / 104 tests PASS, including file/node scope,
  cryptographic HTTP UUID, stale requests, admission, standalone and history.
- Strict TypeScript and production build PASS. Existing bundle-size advisory
  remains. Changed-file ESLint PASS. Full lint still reports the same 46 errors
  in five unchanged prototype/old E2E files outside this task.
- Read-only browser comparison PASS at 1512px and 1280px: Internal/cloud, runtime
  card, icon hierarchy, no page overflow, linked Code selection, exactly one
  active artifact, Service/Deployment switching and expand/collapse; no pageerrors.
- Actual Tailscale HTTP browser: 22 assertions PASS, using new scoped disposable
  Applications/Definitions. Product authentication/forms/Preview/API, per-click
  UUID, one graph across admission, fake-adapter deployment, both workload READY
  observations, exact baseline diff, HTTP clipboard fallback, and Definition-driven
  VPC/EKS/Aurora graph remain functional. No route mocks or cloud calls.
- Documentation checker and `git diff --check` PASS. Diagrams and backend code
  are unchanged, so their prior verification was reused; no Go suite, Kubernetes,
  AWS or retained-database mutation was rerun for this presentation correction.

The first focused run exposed three label assertions that referred to the old
heading/raw descriptor; they were updated while preserving scope assertions.
A later focused run found the busy button had disappeared when switching to the
single graph; the UI was fixed to retain that action and the existing tests pass.
No assertions about fencing, duplicate admission or stale responses were removed.

## Running review and ownership

Built assets are served by the already-running isolated fake-adapter backend at
`http://100.77.224.24:18093/ui/`, tmux `preview-review-server`. The backend was
not restarted and the user's existing review drafts/Applications were preserved.
Only separately scoped verification Applications were added. Existing user tmux
sessions were left intact; no external cloud/cluster or retained database was used.
Runtime scripts, browser results and logs remain under
`/tmp/poc-preview-tailscale-20261023/`, including `visual-refinement/result.json`,
`visual-http-e2e/result.json`, `visual-tests-all.log` and final focused/build/lint
logs. No runtime state or binary is committed.
