# Orchestrator Web Console

React + TypeScript strict + Vite frontend for the orchestrator administrative
console. This directory is independent from the acceptance application
frontend under `backend/examples/acceptance-app/frontend/`.

## Install and validate

```bash
cd frontend
npm ci
npm run typecheck
npm run lint
npm test
npm run build
```

The production bundle is written to `frontend/dist/`. The Go backend serves it
under `/ui/` and exposes the same-origin JSON API under `/api/v1/`.

## Development

Start the Go API from `backend/`, then run:

```bash
cd frontend
npm run dev
```

Vite serves the console on `http://127.0.0.1:5173/ui/` and proxies `/api` to
`http://127.0.0.1:8080` by default. Override the target with
`ORCHESTRATOR_API_ORIGIN` when needed.

Feature code belongs under `src/features/`; application shell/routing under
`src/app/`; use-case-independent transport/UI under `src/shared/`; design tokens
and global styles under `src/styles/`.

## Languages

Console text comes from JSON catalogs in `public/locales/` (see ADR-020).
Components call `t('<feature>.<file>.<message>', params)` from `src/shared/i18n`;
`vi.json` is the default and fallback. To add a language, copy `vi.json` to
`<code>.json`, translate the values (keep keys and `{placeholders}`), and add
`{"code": "<code>", "name": "<native name>", "intl": "<BCP 47 tag>"}` to
`manifest.json`. No code change is needed; `npm test` checks every catalog has
the same keys and placeholders as `vi.json`.
