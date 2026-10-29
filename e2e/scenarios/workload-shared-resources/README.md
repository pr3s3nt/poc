# Workload shared resources

Verify [UC-16 BR-28–30](../../../orchestrator_docs/usecase/UC-16/specification.md)
and [resource UI](../../../orchestrator_docs/usecase/UC-16/ui/screens.md).
Product UI and HTTP handlers use a disposable in-memory backend with fake
execution adapters. This does not verify database provisioning or deployment.

Prerequisites: Node 22+, Go, installed `score-k8s`, Chromium; `cd e2e && npm ci`
and `npx playwright install chromium`. Build Console with `npm --prefix frontend
run build`. Start `go run ./test/fixtures/deployment-review` from `backend/`,
passing `-ui-dir` (absolute frontend/dist), `-cache-dir` and `-addr-file` paths
outside Git. Use the resulting loopback URL and fixture developer account with
its synthetic password in a private file, per [shared Console guide](../../README.md).

Run from repository root:

```sh
ORCH_PLATFORM_URL=http://127.0.0.1:PORT \
ORCH_PLATFORM_USERNAME=developer \
ORCH_PLATFORM_PASSWORD_FILE=/private/fixture-password \
ORCH_HEADLESS=1 ORCH_EVIDENCE_DIR=/tmp/shared-resources-evidence \
node e2e/scenarios/workload-shared-resources/run.mjs
```

Assertions: generated shared identity, reuse under another alias with locked
configuration, stable edit/save, explicit private scope, app/environment
isolation, VI/EN at 1512/1280px with expanded/collapsed sidebar and no overflow.
Component tests cover the inline dialog, conflict/unavailable metadata and
import/private preservation. Failure exits nonzero; screenshots and assertions
stay outside Git. No secrets, browser session or provider state are recorded.

Side effects: creates two uniquely named fixture applications and desired
workload drafts. Never Preview/Deploy or mutate cluster/cloud/retained data.
Run only against the disposable fixture; reruns use a new timestamp. Browser
session closes in finally. Stop the task-owned fixture process to discard all
in-memory applications; remove its private cache/evidence only if no longer
needed. No shared infrastructure cleanup is performed or required.
