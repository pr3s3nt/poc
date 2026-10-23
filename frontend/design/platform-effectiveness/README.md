# Platform effectiveness — UI proposal

Review-only prototype for one proposed use case: Theo dõi và báo cáo hiệu quả
platform. Not an approved product specification or a production feature.

The prototype uses React/strict TypeScript and deterministic synthetic events.
All metrics, identities, sources and exports are explicitly demo data. No product
API, credentials, cloud resources or database are accessed. Baseline edits live
only in React memory; reload resets them. No labor savings are inferred from
elapsed deployment time.

## Proposed interactions

- Application and 7/30-day filters; populated and empty scenarios.
- DORA cards calculated per application for production deployments, with source
  records, formulas and coverage in an accessible dialog.
- Self-service/usage tab; only answered support records enter the self-service
  denominator. Incomplete data is visible.
- Optional baseline editor for request lead time; comparison is elapsed time,
  never person-hours. Values are proposals for review, not default requirements.
- CSV export includes selected scope, demo labels and underlying records.

DORA references: https://dora.dev/guides/dora-metrics/
Platform value references: https://humanitec.com/blog/impact-of-internal-developer-platforms
and https://docs.port.io/guides/all/create-roi-dashboard/ .

## Build and serve

From frontend:

```bash
npx tsc -p design/platform-effectiveness/tsconfig.json
npx vite build design/platform-effectiveness --outDir /tmp/platform-effectiveness-dist
node design/platform-effectiveness/serve.mjs /tmp/platform-effectiveness-dist 100.77.224.24 18097
```

Server binds only the requested Tailscale address and serves built assets only.
No repository directory is exposed. No authentication is implemented; access is
subject to the existing tailnet ACL. Stop only this prototype's process/session
when the review ends. Preview runtime details are handed off in the conversation.

## Review validation — 2026-10-23

- Strict prototype TypeScript, scoped ESLint and standalone Vite build passed.
- Chromium at 1512px desktop and 390px mobile: no page errors or horizontal
  page overflow; all tabs, 7/30-day/application filters, matching source counts,
  dialog/Escape, baseline save/reset, empty state and CSV download passed.
- Desktop/mobile screenshots inspected; downloaded CSV includes mock label,
  selected application/period and source rows.
- HTTP 200 verified through 100.77.224.24:18097 both inside WSL and from the
  Windows host. Remote user device access is not independently verified.
- Documentation checker and diff whitespace check passed. No PlantUML changed.
- Product frontend/backend full suites and cloud/database tests were not run:
  this is an isolated UI prototype; product code and runtime contracts unchanged.

Runtime preview: tmux session `effectiveness-mock-18097`, built assets in
`/tmp/platform-effectiveness-dist`. Other sessions/services remain unchanged.
