---
id: VERIFY-2026-09-21-ROOT-LAYOUT
artifact: verification-evidence
status: evidence
last_reviewed: 2026-09-21
---

# 2026-09-21 — Root backend/frontend layout migration

## Scope

- Moved the Go module and its tests/acceptance workloads from `implementation/`
  to `backend/`.
- Moved the Orchestrator Web Console from `implementation/frontend/` to root
  `frontend/`.
- Updated CLI defaults, integration scripts, runbooks, code maps and links.
- Removed the ignored local `final_idp/` clone and duplicate root planner bundle;
  retained the tracked conformance reference in `orchestrator_reference/`.

## Result

- `go test ./...`: passed, including HTTP E2E and all 33 planner fixtures.
- `go build ./...` and `go vet ./...`: passed.
- Frontend clean install, typecheck, lint, 23 tests and production build: passed;
  two live external tests remained intentionally skipped.
- Backend started from `backend/` with the default sibling UI path;
  `/api/v1/healthz` returned success and `/ui/` returned HTTP 200.
- Integration shell scripts passed `bash -n` after sibling frontend path changes.
- Documentation metadata/link/package/PlantUML validation passed.

## Non-blocking observation

`npm ci` reported two moderate dependency vulnerabilities. No dependency or
lockfile change was made as part of the path-only refactor; remediation requires
a separate dependency review.

## External verification not run

kind and AWS happy paths were not executed because this refactor did not
authorize cluster/cloud mutation. Their scripts were syntax-checked and all
local consumers of the changed paths passed.
