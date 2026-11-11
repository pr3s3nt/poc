# Workload .env — issue #72

Real Console/backend acceptance for [ENVFILE-01–06](../../../orchestrator_docs/usecase/UC-16/envfile-specification.md),
using the existing kind cluster and [namespace safety](../../../orchestrator_docs/operations/kind.md).

Prerequisites: feature Console seeded through API/Definitions (default workload,
namespace/cluster and in-cluster PostgreSQL); Dev Platform Vault with Kubernetes
auth/VSO, Fleet default binding and Git access. `postgres:16-alpine` must be
available. Install `npm ci --prefix scripts/bootstrap && npm ci --prefix e2e`,
Chromium, kubectl; use the authorized kind-k8s-4f kubeconfig in `KUBECONFIG`.

From repository root set `ORCH_PLATFORM_URL`, `ORCH_PLATFORM_USERNAME`, private
`ORCH_PLATFORM_PASSWORD_FILE`, `ORCH_HEADLESS=1`, and private external
`ORCH_TEST_RUN_DIR`. Run `node e2e/scenarios/workload-envfile/run.mjs <stage>` for
`prepare`, `author`, `deploy`, `verify`, `cleanup`, in that order.

Creates one owned Application, shared REGION, two create-only synthetic Secrets,
two workloads and two PostgreSQL resources through UI. Authoring checks all five
sources, 36 rows, alias rename, markers, bulk creation, order and reopen. One
Environment Preview/Deploy covers both workloads. Read-only runtime assertions
check references, literal values and secret delivery without printing values;
VI/EN screenshots omit unsaved secret literals. Full unit checks cover errors,
file merge and uncertain/partial secret creation separately.

No mocks. Namespace-scoped Fleet access uses the shared ownership/UID helper.
A recorded Deploy is never replayed; inspect failed attempts before repair.
Cleanup uses product Application deletion then successful namespace absence
lookup. Keep screenshots/state outside Git. Generated delivery Git repository
follows product retention, not arbitrary repository deletion. Feature stack
survives workload cleanup for user acceptance. No cloud/other-cluster mutation.
