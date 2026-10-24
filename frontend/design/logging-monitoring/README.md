# Logging & Monitoring — UI review prototype

Interactive product UI proposal using synthetic JobFlow data. This is a mock,
not an implemented collection pipeline. No backend APIs, database, credentials,
Kubernetes or AWS resources are accessed. It does not install Metrics Server.
All state resets on reload except the existing shell/language preferences.

## Scope and interactions

- Application home shows Logging & Monitoring immediately below the environment
  tabs. Error counts, unusual-event counts and CPU/memory peaks have miniature
  charts and open scoped detail views. Workload shortcuts remain available.
- staging/production, workload, time range and custom dates change the fixture
  data. Log search and stdout/stderr filters work together; load older rows,
  follow synthetic new logs, inspect a line and copy its message.
- CPU/memory history uses separate charts with explicit mCPU/MiB units and
  per-workload series. Mouse or arrow keys inspect a point; its action opens
  logs around that timestamp. The workload table shows the latest sample.
- The yellow prototype toolbar selects healthy, not deployed, waiting, data
  gap, missing metrics, error and loading states. Missing metrics never become
  zero; logs remain available. A collection gap breaks the chart lines.
- Both Vietnamese and English use local prototype catalogs, preserving the
  real shell's language selector. Sidebar links and unrelated actions show a
  prototype notice instead of invoking product flows.

The suggested 7-day log / 15-day metric retention is shown for review. Fixture
history is generated, not observed; chart paths are illustrative, downsampled
series rather than proof of actual sampling, durability or provider support.
Collector/storage design and external verification remain separate work.

## Product references

Imports the real [AppShell](../../src/app/AppShell.tsx),
[Button](../../src/shared/ui/Button.tsx), [Status](../../src/shared/ui/Status.tsx),
and [Console styles](../../src/styles/index.css). Application header, environment
tabs, workload rows and content panels follow
[ApplicationHomePage](../../src/features/applications/ApplicationHomePage.tsx).
No shared product component, stylesheet, routing or navigation is changed.
This is a partial Application home proposal; resource/preview/edit flows are
outside this review mock.

Review captures: [Application home](screenshots/home.png),
[Logs](screenshots/logs.png), [Metrics](screenshots/metrics.png),
[Event drill-down](screenshots/events.png).

The current Application home was opened in Chromium with intercepted synthetic
API responses before design. The reference capture lives outside Git at
`/tmp/orchestrator-logging-monitoring/console-reference.png`. Primary buttons
reuse the current green Button style; this follows the product and the existing
IMP-029 deviation rather than changing the common design.

## Build and serve

From `frontend/`:

```bash
npx tsc -p design/logging-monitoring/tsconfig.json
npx eslint design/logging-monitoring/main.tsx design/logging-monitoring/serve.mjs
npx vite build design/logging-monitoring --base / --outDir /tmp/orchestrator-logging-monitoring-dist --emptyOutDir
node design/logging-monitoring/serve.mjs /tmp/orchestrator-logging-monitoring-dist 127.0.0.1 18101
```

Open `http://127.0.0.1:18101/`. The optional host argument can bind an assigned
Tailscale address for review. The server serves only built HTML/CSS/JS from the
output directory; it does not expose repository files or proxy any API. Stop
only this mock's process when finished. Desktop review follows the current
Console; this task does not introduce a new mobile layout.

## Review validation — 2026-10-25

- Strict prototype TypeScript, scoped ESLint, standalone Vite build,
  documentation checker and whitespace checks passed.
- Chromium review covered filters, combined search, log details/Escape,
  follow/pause, environment switching, older log pagination, custom-range
  validation, chart mouse/keyboard inspection, sidebar collapse and VI/EN.
- Waiting, not-deployed, loading, retry, missing metrics and collection gaps
  were checked. Missing intervals break chart paths; metrics older than log
  retention cannot link to an invalid log interval.
- Desktop 1512px and 1280px captures have no horizontal page overflow. Shell,
  spacing, environment tabs, cards and buttons were compared with the actual
  Console reference. New content remains inside the application area.
- Copy review removed introductory instructions, repeated status explanations
  and tool configuration from the normal view. Retention, units, missing-data
  messages and the synthetic-data banner remain because they affect meaning.
- No browser errors or product API calls were observed. HTTP 200 verified on
  `100.77.224.24:18101`; access from another user device was not independently
  verified. Runtime session: `logging-monitoring-mock-18101`.
- Full product frontend/backend suites and database/cluster/cloud checks were
  not run: only this isolated UI mock changed. No PlantUML changed; the docs
  checker reports PlantUML unavailable locally.

## Overview revision — 2026-10-25

The user's requested overview adds clickable error and unusual-event counts,
CPU/memory peak percentages, compact trends and recent noteworthy rows. The
overview moves above the workload table so these signals are visible on entry.
Its selected 15m/1h/6h/24h interval carries into detail navigation. Log cards
open ERROR-filtered logs; individual log events also select the workload. CPU
and memory cards focus the matching chart/workload. The event card opens the
complete list behind its count, with another click selecting the relevant series.

All counts and peaks derive from the same synthetic log and metric fixtures as
the detail views. Peaks use explicit per-workload fixture limits; the current
percentage and recovered state are separate from the historical maximum.
Metric time ranges now sample the same absolute-time signal, so changing the
window does not move an old spike into a new interval. Changing detail filters
clears the prior drill context rather than leaving a mismatched explanation.

The 80% threshold and ERROR classification are illustrative UI choices shown
in accessible “Cách đọc” help, not approved runtime detection requirements.
Each high metric/workload pair counts once, plus the fixture restart event.
This revision does not implement alert delivery, a rules engine or new backend
behavior. Missing metrics disable metric cards while logs remain accessible;
unknown collection results show dashes rather than zero errors.

Focused browser review checked count-to-detail equality, all four summary
links, individual event links, interval preservation, 15-minute normal state,
missing/error/waiting data, production scope, VI/EN and 1280px layout/collapse.
No browser errors or product API calls were observed. Scoped strict TypeScript,
ESLint, build, documentation and whitespace checks passed. The overview was
compared visually with the previous mock and the imported Console shell; help
contains the counting/threshold explanation so the normal view stays concise.
