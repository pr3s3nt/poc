# Determined rollout failure releases the Environment — issue #55

Goal: prove [UC-06 CD-10](../../../orchestrator_docs/usecase/UC-06/specification.md):
a delivered workload whose rollout fails (CrashLoopBackOff) ends FAILED and
releases the Environment, the Console shows the rollout reason with a log link,
and the user can fix the configuration and deploy again or delete the
Application. It also proves that a Deploy held as INTERRUPTED by an earlier
release resolves with one Recover after the upgrade.

## Topology

One Application `rollfail-issue55` with one workload `crash` (template "Xử lý nền",
class `git-dev`, no resources). The container image is the digest-pinned
`postgres:16.14-alpine` already present on the kind node; only its arguments
change:

| Step | Arguments | Expected rollout |
|---|---|---|
| prepare | `sh -c 'echo missing subcommand; exit 0'` | exits at once, CrashLoopBackOff |
| fix | `sh -c 'echo started; exec sleep 2147483647'` | Ready |

## Prerequisites

- A Console built from the branch under test, installed in its own namespace on
  `kind-k8s-4f` per [kind Console manifests](../../../deploy/kind-k8s-4f/README.md)
  and seeded with `scripts/bootstrap/seed-api.mjs` (Git, cluster account, workload
  Vault with Kubernetes auth, Fleet CD, cluster/namespace/workload Definitions).
- `npm ci --prefix scripts/bootstrap && npm ci --prefix e2e`, Chromium for Playwright.
- `ORCH_TEST_RUN_DIR`: private directory outside Git with `config.json`
  (`clusterKubeconfigFile`, `secretStoreKey`). Console variables
  `ORCH_PLATFORM_URL`, `ORCH_PLATFORM_USERNAME`, `ORCH_PLATFORM_PASSWORD_FILE`,
  optionally `ORCH_HEADLESS=1`.

## Run

```sh
node e2e/scenarios/rollout-failure-release/run.mjs prepare
# Optional, on a Console still running the previous release:
ORCH_EXPECT=INTERRUPTED node e2e/scenarios/rollout-failure-release/run.mjs deploy
#   upgrade the Console to the branch image, then:
node e2e/scenarios/rollout-failure-release/run.mjs recover
# On the branch release:
ORCH_EXPECT=FAILED node e2e/scenarios/rollout-failure-release/run.mjs deploy
node e2e/scenarios/rollout-failure-release/run.mjs fix
ORCH_EXPECT=SUCCEEDED node e2e/scenarios/rollout-failure-release/run.mjs deploy
node e2e/scenarios/rollout-failure-release/run.mjs cleanup
```

`deploy` refuses to start while the recorded attempt is not terminal; it never
replays an attempt.

## Assertions

- Every stage goes through the Console form, Preview, a single Deploy request,
  the Settings recovery banner or the deletion dialog.
- `deploy` with `ORCH_EXPECT=FAILED`: Operation FAILED, no active operation on the
  Environment, Deployment `failureCode` is `ROLLOUT_DEGRADED` or
  `ROLLOUT_NOT_READY`, and its detail page shows the rollout message with
  "Xem log".
- `deploy` with `ORCH_EXPECT=INTERRUPTED` (previous release): the Environment
  stays held. `recover` then ends RECOVERED and releases the Environment.
- `deploy` with `ORCH_EXPECT=SUCCEEDED` after `fix`: Operation SUCCEEDED.
- `cleanup`: deletion SUCCEEDED and the Application namespace is absent.

## Side effects and cleanup

Creates the Application, its namespace and Fleet delivery tree; grants the Fleet
access Role/RoleBinding for the namespace (technical setup). `cleanup` removes that
access, deletes the Application through the Console and checks the namespace is
gone. Run state, screenshots and operation records stay in `ORCH_TEST_RUN_DIR`.
The isolated Console namespace is removed separately by the operator.
