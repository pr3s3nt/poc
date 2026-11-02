# Catalog-only live UI acceptance — historical

This runner records the earlier Catalog-only behavior. Its removed-editor
assertions are superseded by [UC-03 CM-01–04](../../../orchestrator_docs/usecase/UC-03/catalog-releases.md#definition-authoring-and-catalog-modules).
Use the [Definition/Catalog scenario](../definition-catalog/README.md) for current
read-only form verification; do not use this historical runner as a current gate.

Uses a running Console, its real backend/database, HTTPS Git discovery and a READY
Internal account and all Resource Type schemas referenced by the release
(including MySQL in the sample). Run bootstrap prerequisites first. No response interception, fixture API or fake executor is used.
The Console must run the current change with Kubernetes adapters. Use only an
authorized cluster/account; this runner creates and deploys a dedicated Application.

Install dependencies per [E2E guide](../../README.md). Publish the repository's
root `catalog.yaml` under a new SemVer tag on the tracked branch. Configure
Console auth via the standard environment variables, then:

```bash
ORCH_PLATFORM_URL=http://localhost:3001 ORCH_PLATFORM_USERNAME=platform-engineer \
ORCH_PLATFORM_PASSWORD_FILE=/private/console-password ORCH_HEADLESS=1 \
ORCH_CATALOG_TAG=v0.20261102.1 ORCH_EVIDENCE_DIR=/private/catalog-live \
node e2e/scenarios/catalog-only/run.mjs
```

The defaults consume `https://github.com/pr3s3nt/poc.git`, branch `main`, source
`catalog-only-20261102`, account `k8s-4f`, tags `v0.20261102.*`. Override using
`ORCH_CATALOG_REPOSITORY`, `ORCH_CATALOG_BRANCH`, `ORCH_CATALOG_SOURCE`,
`ORCH_CATALOG_ACCOUNT`, `ORCH_CATALOG_TAG_PATTERN`. Git documents bind only symbolic
account slots. No credential is stored in Git or evidence.

The runner registers/refreshes the source through UI, opens read-only release
components, creates a dedicated app, selects the release, imports a real nginx
Score and Preview/Deploys through UI. Provide `ORCH_CATALOG_APP` on follow-up runs
to select a newer tag for that same owned Application. It verifies saved selection,
Preview pins, applied release, production isolation and resource IDs. It retains
screenshots and nonsecret metadata outside Git, including on failure.

Side effects: source/release caches, an Application and its staging Kubernetes
namespace/workload. Existing applications are untouched. The app is retained for
review; cleanup uses the existing Console Application deletion/ownership workflow.
There is no source/release delete API. Failure is not automatically replayed.
A successful report is not by itself proof of Pod readiness or served HTTP;
record separate Kubernetes/HTTP reads against the returned namespace. See
[requirements](../../../orchestrator_docs/usecase/UC-03/catalog-releases.md) and
[operations](../../../orchestrator_docs/operations/catalog-releases.md).

After recreating backend/frontend with the same durable volumes, run the read-only
check with the same auth, app/tag and evidence environment variables:

```bash
node e2e/scenarios/catalog-only/verify.mjs
```

It opens the real UI, confirms an unchanged Preview and the applied release,
then verifies the two-replica Git policy is displayed read-only. It performs no
refresh, selection save or deployment. Override `ORCH_EXPECTED_REPLICAS` if needed.
