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

Live procedure and assertions are being added. No completed deployment, alert,
persistence or lifecycle acceptance is claimed by the existence of this fixture.
Runtime credentials, ownership receipts and browser evidence must remain outside
Git. Cluster UID and resource UID checks precede cleanup. Retain the isolated
cluster/feature stack while the authorized verification is in progress; teardown
must not remove shared/preexisting resources or another agent's services.
