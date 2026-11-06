---
id: VERIFY-WORKLOAD-FREQUENCY-LAYOUT-20261106
artifact: verification-record
status: current
last_reviewed: 2026-11-06
---

# Workload form ordered by editing frequency

User-approved ASCII layout, 2026-11-06. Branch `features/bo_sung_default`, starting
commit `9a6ce614`. The four initial uncommitted files were the agent's earlier
layout draft in this same task; there were no user-owned changes to preserve.
Canonical presentation: [UC-16 screens](../usecase/UC-16/ui/screens.md#wd--runtime-groups-and-inherited-values),
[WD-09](../usecase/UC-16/workload-defaults-specification.md).

## Result

Name and container image/environment edits precede advanced runtime policies.
Replica stays inside Scaling & updates. Service/networking and dependencies now
collapse with summaries. Startup contains role/order, command/args and working
directory. Pod security and the final execution-kind/policy/class/annotations
section follow the common fields. CronJob exposes its schedule separately and
initially open, with job policy behind its own disclosure; StatefulSet storage
remains conditional. Environment variables are the only initially open group
for an ordinary Deployment.

Existing shell, shared buttons, styles, locale catalogs and disclosure patterns
were reused. The reference was the live pre-change draft at the same Console URL,
plus AppShell, shared UI design and UC-16 components. Visual comparison confirms
that image/environment fields are reachable immediately after the workload name;
rare settings no longer occupy the first viewport. No new explanatory paragraphs
were added to the form. Draft state and inherited-value serialization remain owned
by the existing editor/backend resolver. Dependency and final-configuration groups
retain mounted editor state after first opening so closing cannot discard raw edits.

## Checks

- All 133 focused workload tests pass. Existing tests now open the corresponding
  group before editing; data, identity, false/zero, error and no-op assertions remain.
- Typecheck, lint and production build pass. Existing PlatformSettingsPage ref/effect
  warning and bundle chunk-size warning remain.
- Full frontend suite (`npm test -- --maxWorkers=2`): 968/970 tests passed, 98/100
  files passed. Only the two previously reproduced baseline failures remain:
  `test/e2e/terraform-v2-labels.test.mjs` and `test/e2e/v04-internal-full.test.mjs`.
  The earlier high-concurrency App navigation timing failure did not recur.
- Documentation checker and diff whitespace checks pass.
- No Go product source, API contract, SQL or Kubernetes manifests changed. Go/DB/
  Kubernetes acceptance was not repeated for this presentation change.

## Real runtime dependency

Browser validation of custom CronJob resolution exposed missing IANA timezone
files in the Alpine backend image. Both explicit and inherited Asia/Bangkok
resolution returned INVALID_WORKLOAD before the package was installed; the image
had no `/usr/share/zoneinfo/Asia/Bangkok`. The production Dockerfile now installs
`tzdata` with its existing runtime packages. The retained issue37 image was rebuilt
from the existing dev base plus that package and the unchanged backend binary.
Its timezone file and successful real CronJob resolution were checked. No full
release image build from scratch or application Deploy was performed in this
follow-up; prior [issue #37 runtime acceptance](2026-11-06-workload-defaults.md)
remains a separate observation.

## Browser and handoff

Exact Tailscale Console page (authentication required):
[Edit draft web](http://desktop-bdjp2rq-1.tail342836.ts.net:18339/ui/applications/b1b983d7-cea1-4e4d-9a9e-63638919db9c/environments/staging/workloads/web).

At `2026-11-06T07:02:03Z`, the actual page/backend were checked through that MagicDNS URL from this host,
without intercepted responses. Checks cover group order/initial state, Enter-key
open/close, explicit replica zero and reset, startup edit retention, conditional
CronJob/StatefulSet groups, Asia/Bangkok resolution, and VI/EN switching. After
restoring temporary edits, Save returned success and the desired document matched
the original exactly. The final page was returned to Vietnamese. No Preview or
Deploy was invoked; the retained authoring Application remains draft-only.

Private browser receipts, screenshots and build logs remain in
`/home/thanhnt1/.local/state/issue37`, including `layout-browser.json`,
`layout-vi-final.png`, `layout-en.png` and `layout-cron.png`. Source-check logs are
`/tmp/issue37-layout-*.log`. Keep Compose project `issue37-defaults` and Tailscale
Serve port 18339 running. Data remains in its isolated volumes; restart instructions
are in the previous issue #37 verification record. User tmux sessions and shared
stacks/routes were untouched.
