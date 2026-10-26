# Orchestrator backend

Go product module for the Orchestrator. [Canonical documentation](../orchestrator_docs/INDEX.md)
owns requirements, architecture and operational permissions; this module implements them.
The React/TypeScript [Web Console](../frontend/) is distinct from the
[acceptance application](examples/acceptance-app/README.md), which contains the
frontend, backend, PostgreSQL jobs and worker used for verification.

## Build and validation

Use the Go version in go.mod and the locked frontend dependencies. Build the Web
Console before serving its dist directory through the API process.

```bash
cd backend
go build ./...
REQUIRE_POSTGRES=1 go test ./...
cd ../frontend
npm ci
npm run typecheck
npm run lint
npm test
npm run build
```

SQL tests require a disposable real PostgreSQL server configured through the
private ORCHESTRATOR_POSTGRES_TEST_URL environment variable. They provision
separate Owner, Console and State logins and must not skip when required.
Acceptance cache tests use a real redis-server. Terraform tests use the pinned
CLI described in the runbook; race/vet/conformance and documentation checks
follow the change-impact requirements in [AGENTS.md](../AGENTS.md).

## Configure and deploy

Platform Engineers register Resource Types, verified Terraform module
Definitions, workload rendering templates and Driver Accounts. Matching and typed
resource references determine dependencies, each account/region and each target.
An existing-cluster Definition uses its own Internal account; a new EKS Definition
uses its own AWS account and region. Kubernetes consumers inherit the resolved
cluster target. An Environment has no deployment Connection or executionProfile.
Its configuration keys and Secret Store selection remain independent.

Developers import Score into workload drafts, inspect Score and pending-change
Preview, then deploy through the Console. Preview is pure; deployment uses the
pinned plan and persists status, target identity and typed output references.
Update, restart/reuse and reverse dependency cleanup retain the original pins.
The current one-cluster execution guard remains; this refactor does not add
multi-cluster execution or PE patch authoring.

Read [UC-03](../orchestrator_docs/usecase/UC-03/README.md),
[UC-05](../orchestrator_docs/usecase/UC-05/README.md),
[UC-06](../orchestrator_docs/usecase/UC-06/README.md) and
[UC-16](../orchestrator_docs/usecase/UC-16/README.md) for authoring, Preview,
deployment and draft contracts. Starting the product does not authorize an
external deployment or reset retained data. Startup refuses incompatible stores;
it does not migrate, reset or regrant them.

## Real acceptance runs

Runner E2E mới đặt tại [`e2e/`](../e2e/README.md) ở root repository. Các runner
hiện có trong `test/integration/` giữ path cho tới migration/refactor liên quan;
Go integration helpers/tests cần module Go tiếp tục nằm trong backend.

The [Terraform acceptance runbook](../orchestrator_docs/operations/terraform-v2-acceptance.md)
and [AWS runbook](../orchestrator_docs/operations/aws.md) own fixture handles,
pinned Terraform/score-k8s/kubectl tools, permissions, estimates, finite deadlines,
independent supervisors, ownership and service-API cleanup proofs.

- Existing cluster: test/integration/v04-internal-full.sh; full acceptance app,
  real PostgreSQL/Redis, jobs/cache/config/secret/error checks and protected cluster.
- New EKS in the same execution: test/integration/terraform-v2-acceptance.sh with
  a private V05 configuration and completed integrated V04 admission.
- Managed services: the same runner with A02 (Aurora/ElastiCache), proven cleanup,
  then a separate A03 (Aurora/in-cluster Redis) configuration/run identity.

Use the headed scenario with Vietnamese captions and same-run assertions/video;
review public artifacts for secrets before attaching them. Historical M04 and
Environment-profile scripts are retired and their old commands are not current
operational entry points. Do not delete a namespace or trust an empty tag index
as a substitute for original-state/UID/provider-API cleanup.

[Current state](../orchestrator_docs/CURRENT_STATE.md),
[code map](../orchestrator_docs/implementation/code-map.md) and
[deviations](../orchestrator_docs/implementation/deviations.md) describe actual
implementation and validation status; prepared code is not a completed runtime gate.
