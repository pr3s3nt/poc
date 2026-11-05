---
id: VERIFY-20261105-CONSOLE-USER-GUIDE
artifact: verification-record
status: evidence
last_reviewed: 2026-11-05
---

# Console user guide integration

Scope: [GUIDE-01–05](../architecture/ui/user-guides.md). Adds sidebar navigation,
`/ui/guides`, the approved Developer Markdown and 15 curated product illustrations.
Developer, Platform Engineer and Admin can read the Developer guide. No separate
Platform Engineer article is authored. Original captures, sessions and runtime
screenshots stay outside Git; only the user-approved illustrations are product
assets under `frontend/public/guides/acceptance-app/`.

## Checks

- Frontend typecheck, lint and production build pass. Lint retains one existing
  `PlatformSettingsPage.tsx` effect-ref warning; Vite retains a large-chunk warning.
- Guide component, AppShell and route cases pass, including access policy,
  empty-state illustration, image retry, keyboard modal/Escape/focus and anchors.
  The App guide navigation/reload test passes and confirms GET-only access.
- Focused final run: 76 passed, 1 failed across five files. The failure is the
  existing App Definition workload deep-link assertion described below.
- Full `npm test`: 967 passed, 4 failed across 97 files (971 tests). Three failures
  were reproduced on the unmodified base commit `8df0c9cf` in an external archive:
  `terraform-v2-labels` expects retired Score labels; `v04-internal-full` expects
  the unrecorded ID `ui.workloadspec.imported`; App's Definition deep-link test
  expects an empty resource type but receives `workload`. No assertions weakened.
- The fourth full-suite failure was `MatchingPreviewPanel` not finding Manifest
  Templates during initial loading. That entire file passes in both the focused
  final run and the unmodified-base run. This run does not claim a green full suite.
- Documentation checker, `git diff --check` and new scenario Node syntax pass.
  Go/Kubernetes/cloud checks not run: no backend/runtime deployment behavior changed.

Logs and browser evidence:
`/home/thanhnt1/.local/state/user-guide-console/` and
`/tmp/dev-guide-capture/` (outside Git). Baseline archive is
`/tmp/user-guide-baseline/`; it does not modify the user's working tree.

## Real browser verification

[Console guide](http://desktop-bdjp2rq-1.tail342836.ts.net:5180/ui/guides)
was checked through the exact Tailscale MagicDNS origin on 05-11-2026 using
`e2e/scenarios/user-guide/run.mjs` and the shared Console login helper.
The existing real backend remains on port 3043; this frontend uses its actual
same-origin proxied APIs, not fixtures.

Both DEVELOPER and PLATFORM_ENGINEER runs passed: navigation active state, full
article, 15 images loaded from the Console origin, section anchor, image viewer
keyboard/Escape/focus, collapsed sidebar and reload. Fresh signed-out contexts
show sign-in instead of article content. Reading the article issued zero
application mutation requests. No new app, secret, workload draft, deploy or
cluster/cloud changes were performed in this integration task.

Visual source: existing AppShell, ApplicationsPage, workload editor, shared
styles, Button and Dialog. Inspected DEV screenshot against the existing shell:
sidebar retains its 260px expanded width, active link, identity and footer.
Contents and article reuse content panels; images remain readable at full size
in the viewer. Maintainer/source references are excluded from the product article;
the introductory draft history was shortened without changing tutorial steps.
The earlier capture limitation (no fresh Preview/Deploy screenshots) remains explicit.

## Retained service and restart

Frontend Vite remains running on loopback port 5180. A new Tailscale Serve mapping
for port 5180 was added; all prior mappings/services were preserved. This verifies
access from this host through Tailscale, not from another tailnet device.

To restart from the repository root, first confirm the port still belongs to this
frontend and the backend port still targets the expected Console:

```bash
ORCHESTRATOR_API_ORIGIN=http://127.0.0.1:3043 \
__VITE_ADDITIONAL_SERVER_ALLOWED_HOSTS=desktop-bdjp2rq-1.tail342836.ts.net \
npm --prefix frontend run dev -- --port 5180 --strictPort
```

Existing mapping (already installed; do not replace unrelated Serve config):

```bash
sudo tailscale serve --bg --http=5180 http://127.0.0.1:5180
```

Text/assets are tracked and production build includes them. Sessions are
disposable; application data remains in the unchanged backend. This is the new
Console frontend for review, not an update to the older Console frontend on 3043.
