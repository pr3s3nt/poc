# Rancher-compatible Fleet discovery on kind

Sources: [UC-04 CD-12](../../../orchestrator_docs/usecase/UC-04/specification.md),
[ADR-034](../../../orchestrator_docs/architecture/decisions/ADR-034-simplified-cd-configuration.md),
[shared acceptance procedure](../simplified-cd/README.md).

The user selected kind-only acceptance because no Rancher installation is
available. This scenario uses the real dev Console and Fleet on kind for
baseline/update/rollback/cleanup. Rancher display labels, StatefulSet agents,
mirrored images, version/schema incompatibility and ambiguous registrations are
covered by Go adapter fixtures; those fixtures do not prove live Rancher operation.
No Rancher installation, production cluster or cloud mutation is required.

Reuse the simplified-CD scenario with a new unique run name, private config/state
and evidence directory outside Git. Its prerequisites, UI-only product mutations,
ownership receipts, default restoration and retained Git/Connection lifecycle
apply unchanged. Dev must already run the new backend and current schema; setup
alone is not deployment evidence.

```sh
export ORCH_SIMPLIFIED_CD_RUN_DIR=/private/rancher-fleet/e2e
export ORCH_SIMPLIFIED_CD_NAME=rancher-fleet-unique-run
node e2e/scenarios/simplified-cd/setup.mjs /private/rancher-fleet/acceptance.json
node e2e/scenarios/rancher-fleet/verify-discovery.mjs /private/rancher-fleet/acceptance.json
node scripts/bootstrap/run.mjs --config /private/rancher-fleet/acceptance.json
node e2e/scenarios/simplified-cd/verify.mjs /private/rancher-fleet/acceptance.json
node e2e/scenarios/simplified-cd/rollback.mjs /private/rancher-fleet/acceptance.json
node e2e/scenarios/simplified-cd/verify.mjs /private/rancher-fleet/acceptance.json
node e2e/scenarios/simplified-cd/cleanup.mjs /private/rancher-fleet/acceptance.json
```

Before bootstrap, set `platform.delivery.config.gitConnectionKey` in the private
config to the new `gitKey` recorded by setup. The account/binding names must match
the run name; the Application/subdomain and runtime state/report directories must
be new. Use already loaded baseline/update images and existing platform services.

`verify-discovery.mjs` checks the real form's discovery result against the native
Fleet registration and nonsecret agent ConfigMap. It asserts pinned physical
UID/controller version/agent identity and the same persisted immutable binding.
It never reads Fleet credential Secrets or creates a registration. It saves safe
topology evidence and a screenshot outside Git. No fixture/API interception is
used by the browser run. For optional direct read-only adapter verification:

```sh
cd backend
ORCH_FLEET_TOPOLOGY_KUBECONFIG=/private/dev-kind-kubeconfig \
  go test ./internal/adapters/delivery/fleet -run TestFleetTopologyKindReadOnly -v
```

Side effects and cleanup are owned by the shared scenario: one temporary
Application and its namespace/data; reusable Connections/bindings, private reader
Secret and Git history stay; the previous platform default is restored and existing
Environment pins are compared. Stop on unknown writes or failed ownership proof;
keep private receipts and diagnose before resuming. A dated verification record
must state actual results and the absence of a real Rancher acceptance run.
