# Class choices on kind — issue #61

Goal: prove that a class dropdown offers only what the catalog can serve. With a
catalog whose workload Definition serves only class `git-dev` (no default), a new
workload shows no Default choice, takes `git-dev` without a click, saves it, and
previews and deploys. GET `/resource-types` reports `defaultAvailable` per Type.
Specification: [UC-16 BR-33](../../../orchestrator_docs/usecase/UC-16/specification.md#workload-class-selection--2026-11-05),
[editor screen](../../../orchestrator_docs/usecase/UC-16/ui/screens.md#class-và-metadata-dependency),
[API mapping](../../../orchestrator_docs/usecase/UC-16/ui/api-mapping.md#metadata-class-và-source--i04-07).

## Prerequisites

- A Console built from the feature branch, installed in its own namespace on
  `kind-k8s-4f` and seeded through `scripts/bootstrap/seed-api.mjs` (Git, cluster
  account, workload Vault with Kubernetes auth, Fleet CD, the `k8s-cluster` and
  `workload` Definitions with criterion class `git-dev` only, and the
  `k8s-namespace` Definition without class), and a Git workload template source
  offering "Xử lý nền". Register the template with the installed build: a pin
  registered by another build is not used (ADR-041 transition).
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
node e2e/scenarios/class-choices/run.mjs prepare
node e2e/scenarios/class-choices/run.mjs check
node e2e/scenarios/class-choices/run.mjs save
node e2e/scenarios/class-choices/run.mjs deploy
node e2e/scenarios/class-choices/run.mjs verify
node e2e/scenarios/class-choices/run.mjs cleanup
```

`prepare` creates Application `class-check-issue61` and selects the Secret Store
for staging. `check` and `verify` are read-only. A recorded Deploy attempt is
never replayed.

## Assertions

- `check`: `workload` reports classes `[git-dev]` and `defaultAvailable: false`;
  `k8s-namespace` reports `defaultAvailable: true`; on a new workload the
  "Class Workload" dropdown has only `git-dev` and it is selected.
- `save`: `class-check` is saved through the form without touching the class and
  stores `metadata.class: git-dev`.
- `deploy`: Preview lists only `class-check`; one Deploy request; Operation `SUCCEEDED`.
- `verify`: Deployment `class-check` is ready; the edit page shows class `git-dev`.

The Platform Engineer dropdowns (value source, companion resources, Kubernetes
cluster class) are covered by unit tests, not by this scenario.

## Side effects and cleanup

Deploy creates the application namespace, one Deployment, a Fleet GitRepo and the
application's delivery repository on the platform Git account (retained per
ADR-033). The scenario grants Fleet scoped access in the application namespace
(technical setup, labelled with the Application). `cleanup` removes that access,
deletes the Application through the Console and checks the namespace is gone.
Afterwards delete the Console's own namespace and its `ClusterRoleBinding`.
Screenshots and state stay in `ORCH_TEST_RUN_DIR`.
