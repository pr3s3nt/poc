# Workload name validation on kind — issue #60

Goal: prove that a workload name which is not a Kubernetes DNS label is refused
when the workload is saved, both in the editor (no request is sent) and by the
API (no draft is created), while a valid name saves, previews and deploys, and
an existing workload shows its name only in the page title.
Specification: [UC-16 BR-01](../../../orchestrator_docs/usecase/UC-16/specification.md#quy-tắc-nghiệp-vụ),
[editor screen](../../../orchestrator_docs/usecase/UC-16/ui/screens.md#t08--form-cơ-bản-và-nâng-cao-nhãn-tiếng-việt),
[API mapping](../../../orchestrator_docs/usecase/UC-16/ui/api-mapping.md).

## Prerequisites

- A Console built from the feature branch, installed in its own namespace on
  `kind-k8s-4f` and seeded through `scripts/bootstrap/seed-api.mjs` (Git, cluster
  account, workload Vault with Kubernetes auth, Fleet CD, the `k8s-cluster`,
  `k8s-namespace` and `workload` Definitions; the workload Definition serves
  class `git-dev`), and a Git workload template source offering "Xử lý nền".
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
node e2e/scenarios/workload-name-validation/run.mjs prepare
node e2e/scenarios/workload-name-validation/run.mjs check
node e2e/scenarios/workload-name-validation/run.mjs save
node e2e/scenarios/workload-name-validation/run.mjs deploy
node e2e/scenarios/workload-name-validation/run.mjs verify
node e2e/scenarios/workload-name-validation/run.mjs cleanup
```

`prepare` creates Application `name-check-issue60` and selects the Secret Store
for staging. `check` and `verify` are read-only and can be repeated. A recorded
Deploy attempt is never replayed.

## Assertions

- `check`: a new workload shows "Không đổi được sau khi lưu."; `My App`, `MyApp`,
  `my_app` and `-app` mark the name invalid with the DNS-label message; Save with
  an invalid or empty name focuses the name and sends no request; a direct
  `PUT …/workloads/My App` returns 400 `INVALID_WORKLOAD` with field
  `metadata.name` and leaves the draft version and drafts unchanged.
- `save`: `web-check` (class `git-dev`) is saved through the form.
- `deploy`: Preview lists only `web-check`; one Deploy request; Operation `SUCCEEDED`.
- `verify`: Deployment `web-check` is ready; the edit page shows "Sửa web-check"
  without a name input.

## Side effects and cleanup

Deploy creates the application namespace, one Deployment, a Fleet GitRepo and the
application's delivery repository on the platform Git account (retained per
ADR-033). The scenario grants Fleet scoped access in the application namespace
(technical setup, labelled with the Application). `cleanup` removes that access,
deletes the Application through the Console and checks the namespace is gone.
Afterwards delete the Console's own namespace and its `ClusterRoleBinding`.
Screenshots and state stay in `ORCH_TEST_RUN_DIR`.
