# Environment-variable order on kind — issue #63

Goal: prove that the environment variables of a container keep the order the
author added them, after Save, reopen and Deploy, while the runtime values are
unchanged.
Specification: [UC-16 BR-34](../../../orchestrator_docs/usecase/UC-16/specification.md#workload-class-selection--2026-11-05),
[GW-09](../../../orchestrator_docs/architecture/contracts/generic-workload.md#gw-09--authored-variable-order-issue-63).

## Prerequisites

- A Console built from the feature branch, installed in its own namespace on
  `kind-k8s-4f` and seeded through `scripts/bootstrap/seed-api.mjs` (Git, cluster
  account, workload Vault with Kubernetes auth, Fleet CD, the `k8s-cluster`,
  `k8s-namespace` and `workload` Definitions; the workload Definition serves class
  `git-dev`), and a Git workload template source offering "Xử lý nền" registered
  with the installed build.
- The pinned `postgres:16.14-alpine` image present on the node or pullable; it only
  runs `sleep`.
- `npm ci --prefix scripts/bootstrap && npm ci --prefix e2e`, Chromium for Playwright.

`ORCH_TEST_RUN_DIR` is a private directory outside Git with `config.json`
(`secretStoreKey`, `clusterKubeconfigFile`). Console variables:
`ORCH_PLATFORM_URL`, `ORCH_PLATFORM_USERNAME`, `ORCH_PLATFORM_PASSWORD_FILE`;
`ORCH_HEADLESS=1` without a display.

## Run

From the repository root, in order:

```sh
node e2e/scenarios/env-variable-order/run.mjs prepare
node e2e/scenarios/env-variable-order/run.mjs save
node e2e/scenarios/env-variable-order/run.mjs deploy
node e2e/scenarios/env-variable-order/run.mjs verify
node e2e/scenarios/env-variable-order/run.mjs cleanup
```

`prepare` creates Application `env-order-issue63` and selects the Secret Store for
staging. `verify` is read-only. A recorded Deploy attempt is never replayed.

## Assertions

- `save`: workload `env-order` (class `git-dev`) gets literal variables added in the
  order `ZETA`, `ALPHA`, `MID`; the stored draft has `variableOrder` in that order
  and the reopened editor lists the rows in it.
- `deploy`: Preview lists only `env-order`; one Deploy request; Operation `SUCCEEDED`.
- `verify`: Deployment `env-order` is ready with the three values; the deployed
  workload read back from the Deployment Set keeps `variableOrder`, and the
  editor reopens the rows in the authored order.

Rejection of a duplicate or undeclared `variableOrder` name is covered by the
`workloadspec` unit tests.

## Side effects and cleanup

Deploy creates the application namespace, one Deployment, a Fleet GitRepo and the
application's delivery repository on the platform Git account (retained per
ADR-033). The scenario grants Fleet scoped access in the application namespace
(technical setup, labelled with the Application). `cleanup` removes that access,
deletes the Application through the Console and checks the namespace is gone.
Afterwards delete the Console's own namespace and its `ClusterRoleBinding`.
Screenshots and state stay in `ORCH_TEST_RUN_DIR`.
