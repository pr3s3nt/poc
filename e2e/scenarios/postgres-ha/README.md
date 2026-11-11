# PostgreSQL HA — issue #75

Real Console authoring, one Environment Deploy with HA and in-cluster postgres,
primary Pod failure, retained data on redeploy and product Application cleanup.
[Requirements](../../../orchestrator_docs/usecase/UC-03/specification.md#postgresql-ha--issue-75),
[operator setup](../../../orchestrator_docs/operations/postgres-ha-cnpg.md),
[verification](../../../orchestrator_docs/verification/2026-11-12-postgres-ha.md).

Requires Node, dependencies installed with `npm ci --prefix scripts/bootstrap`
and `npm ci --prefix e2e`, Playwright Chromium, kubectl, the authorized
`kind-k8s-4f` context and real adapters. CNPG 1.30.1 must be Ready. Console must
have postgres Type with optional secret asyncUri, task HA Definition named
`staging-k8s-4f-postgres-ha`, in-cluster Definition, namespace/workload/cluster
Definitions, default Fleet binding, Git access and a verified Secret Store
named `Dev Platform Vault`. This scenario does not install shared services.

Use the shared Console environment variables in [E2E guide](../../README.md),
plus `KUBECONFIG` (private scoped file) and `ORCH_TEST_RUN_DIR` (private directory
outside Git). Use the verified Tailscale origin for browser handoff. Passwords
come from private files; do not put credentials on the command line.

From repository root, run stages in order:

```bash
node e2e/scenarios/postgres-ha/run.mjs prepare
node e2e/scenarios/postgres-ha/run.mjs author
node e2e/scenarios/postgres-ha/run.mjs deploy
node e2e/scenarios/postgres-ha/run.mjs verify
node e2e/scenarios/postgres-ha/run.mjs redeploy
node e2e/scenarios/postgres-ha/run.mjs cleanup
```

`prepare` creates one `postgres-ha-issue75` Application and selects its store.
`author` saves one PostgreSQL client workload with both postgres dependencies and
output references. `deploy` requires Preview to select the HA Definition, then
uses one Environment Deploy and checks the exact operation. Run-owned namespace
Role/RoleBinding give the configured Fleet agent its scoped access, using the
existing bootstrap helper. This is technical setup, not acceptance by itself.
`verify` checks two Ready instances, client host/URI reads/writes, in-cluster
connectivity and standby replication; deletes only the recorded primary Pod,
requires recovery within 60 seconds and preservation of rows. `redeploy` first saves identical configuration and requires an empty Preview
with Deploy disabled (the current product deliberately blocks empty Deploy).
It then changes only a harmless client variable and runs a real Deploy, checking
the same Cluster UID and both databases' rows with database inputs unchanged.
Direct Terraform no-op apply is separately recorded in verification; this does
not claim the Console executed a disabled empty Deploy.

Private `state.json` records Application/namespace/Cluster identities, review
and operation IDs, failover duration and cleanup. Inspect it and the recorded
operation before retrying any failed stage; Deploy never silently replays a
recorded attempt. Do not remove a receipt to bypass uncertain results.

`cleanup` verifies namespace UID, removes only scenario-owned Fleet access
Role/RoleBinding with UID preconditions, then uses product deletion and requires
SUCCEEDED plus authoritative namespace absence. This checks product teardown;
never delete the namespace directly to hide a failed product cleanup. Keep
failure evidence outside Git and report retained objects/ownership refusals.
The scenario uses real PostgreSQL and shared Vault/Fleet, so explicit external
verification authorization and issue #75 scope are required.
