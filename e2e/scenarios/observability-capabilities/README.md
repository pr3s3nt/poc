# Capability-stage observability acceptance

Scope: issue #35 stages 0–5, [UC-09](../../../orchestrator_docs/usecase/UC-09/specification.md)
and [external contract](../../../orchestrator_docs/architecture/contracts/external-observability.md).
This fixture is a generic instrumented HTTP application and an isolated webhook
receiver. It does not implement or prove MCP Gateway/Admin behavior.

Prerequisites: explicit authorization for a fresh task-owned kind cluster, current
Console with real PostgreSQL/Vault/CD adapters, pinned observability stack and
Fleet, private credentials/kubeconfig, Node/E2E dependencies from the shared
[E2E guide](../../README.md), Docker and kubectl/Helm. Never target shared resources.
Build the standard-library Go fixture with:

```sh
docker build -t observability-fixture:issue35 e2e/scenarios/observability-capabilities/fixture
kind load docker-image --name YOUR_OWNED_CLUSTER observability-fixture:issue35
```

The fixture counts actual HTTP traffic, exposes a Prometheus histogram/gauge and
emits JSON plus plain log lines. POST `/control?fail=true` changes its actual HTTP
and readiness behavior; `fail=false` restores it. `/receive` records actual
Alertmanager webhook bodies, inspectable at `/receipts`. No mock provider responses
or synthetic notification receipts are used. These endpoints are test-only and
must stay on the isolated cluster/private forwards.

Run from the repository root with the shared Console environment variables,
`ORCH_TEST_RUN_DIR` (existing owner-only directory outside Git), unique
`ORCH_TEST_NAME`, and `ORCH_CLUSTER_KUBECONFIG_FILE`. The isolated platform must
have a READY account named `obs35-kind` and an already verified default Fleet
binding. `prepare` creates one owned Application, app-scoped Definitions from the
packaged namespace module/current standard templates, and configures the recorded
real connection/profile/binding through Console. It asserts NOT_DEPLOYED.

```sh
node e2e/scenarios/observability-capabilities/run.mjs prepare
ORCH_TEST_ACCESS_FILE=/private/namespace-access.json \
  node e2e/scenarios/observability-capabilities/run.mjs deploy
node e2e/scenarios/observability-capabilities/run.mjs observe
# After a backend rebuild, re-inspect identical content and select its new pin:
node e2e/scenarios/observability-capabilities/run.mjs refresh-renderer
# Each configuration update must be followed by deploy, observe and noop:
node e2e/scenarios/observability-capabilities/run.mjs profile-update
node e2e/scenarios/observability-capabilities/run.mjs noop
```

`deploy` imports a Generic Workload through Console and uses Preview/Deploy.
Namespace RBAC setup uses the shared helper only after product namespace creation;
its rules must include PrometheusRule/PodMonitor alongside ordinary workload access.
`observe` reads the real scoped monitoring API and opens Console. A lost deployment
response is retained as pending and must be diagnosed before any retry. Receipts
pin app/namespace identity; same-name unrecorded applications are never adopted.

Traffic/failure, notifications, persistence and lifecycle acceptance procedures
are still being completed; these phases alone do not prove those gates. Runtime
credentials, ownership receipts and browser evidence remain outside Git. Record
cluster/resource UIDs before cleanup. Retain the isolated cluster/feature stack
while verification is in progress. Withdraw workloads using product delivery,
remove only recorded namespace RBAC/collector allowlist entries, then delete the
owned Application. Never delete shared/preexisting resources or another session.
