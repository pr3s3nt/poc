---
id: VER-2026-09-30-UC00-UC01-LOCAL-ONBOARDING
artifact: verification-evidence
status: evidence
last_reviewed: 2026-09-30
---

# UC-00/UC-01 local onboarding — 2026-09-30

Local verification on the current checkout passed:

- `cd backend && go test ./... && go build ./...` passed, including
  authentication/session, strict create request, Organization isolation,
  atomic Application/Environment creation and no-deploy-side-effect tests.
- `cd frontend && npm run typecheck && npm run lint && npm test && npm run build`
  passed: 11 Vitest files and 32 tests.
- `cd backend && bash test/integration/onboarding-playwright-local.sh` passed
  with run ID `onb-20260930012210-22300`.
- Headless Chromium signed in with the local Developer account, submitted only
  Name/Subdomain, observed the success callout and both `staging`/`production`,
  signed out, confirmed protected access returned to Sign in, signed in again,
  restarted the backend on the same JSON state and reopened the same
  Application. No preview/deploy mutation occurred.
- `python3 scripts/check_docs.py`, `git diff --check` and shell syntax checks
  passed after documentation reconciliation.

The PostgreSQL onboarding integration tests are present and compile in the
default suite, but their live cases require `ORCHESTRATOR_POSTGRES_TEST_URL` and
were skipped in this run. Existing PostgreSQL system-store evidence covers the
normalized adapter and restart/backup path; this record makes no new live
PostgreSQL or cluster claim. Docker, kind and cloud were not used.
