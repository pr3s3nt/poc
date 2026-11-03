# Platform-managed CD UI

Spec: [ADR-033](../../../orchestrator_docs/architecture/decisions/ADR-033-platform-managed-delivery.md).
Operations: [CD runbook](../../../orchestrator_docs/operations/delivery-reconciliation.md).

Prerequisites: running current Console/backend with schema 7; a Platform Engineer
account; Vietnamese locale; dependencies installed per [E2E guide](../../README.md).
Set the shared `ORCH_PLATFORM_URL`, `ORCH_PLATFORM_USERNAME`,
`ORCH_PLATFORM_PASSWORD_FILE`, optionally `ORCH_HEADLESS=1`, then run:

```sh
node e2e/scenarios/platform-managed-cd/run.mjs
```

Assertions: real authenticated default/API reads, four-field Fleet/Git connection form with inline connection actions,
no concrete repository authoring, no CD selector in an existing app's Settings.
Side effects: sign-in/session only, no configuration/App/repository/cluster writes.
Cleanup: helper closes the browser; evidence/session/credentials stay outside Git.
This checks UI/access only. It does not claim real repository creation, Fleet
reconciliation, self-heal or teardown. Those need authorized GitHub/Fleet resources;
new adapter/service tests use explicit simulated HTTP/hosting collaborators.
The old delivery-reconciliation runner targets historical per-Application authoring
and is not an acceptance runner for ADR-033.
