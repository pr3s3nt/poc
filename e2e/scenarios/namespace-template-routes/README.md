# Namespace placement and template HTTPRoute — issue #84

Scope and assertions: [contract/sequence](../../../orchestrator_docs/architecture/contracts/namespace-template-routes.md),
[implementation gates](../../../orchestrator_docs/implementation/namespace-template-routes.md).
This scenario uses the existing Gateway/Traefik on the authorized kind cluster;
it never creates/adopts/updates/destroys shared Gateway, DNS or TLS objects.

## Full checks

`full-kind.mjs` requires Node >=22, Git, kubectl on PATH, an explicit scoped
KUBECONFIG and access to the existing `kind-k8s-4f` cluster. It creates one labelled
namespace and three disposable Pods: candidate/main backend (each with its own
real PostgreSQL 16 and Terraform 1.9.8), and candidate frontend. Sources are exact
Git commit archives, never host directories or credentials. Database passwords
are fixture-only values local to disposable test Pods, with no exposed Service.
No AWS or retained application mutation is authorized by this runner.

From repository root:

```bash
ORCH_EVIDENCE_DIR=/private/task/full-checks \
ORCH_KUBECONFIG=/private/scoped-kubeconfig ORCH_CONTEXT=kind-k8s-4f \
  node e2e/scenarios/namespace-template-routes/full-kind.mjs
```

The runner enables REQUIRE_POSTGRES/REQUIRE_TERRAFORM, runs all backend tests and
builds both candidate and main, and frontend tests/typecheck/lint/build. Exact
failure/skip sets must be compared after collecting private logs; process exit
alone is not proof that product tests passed. Existing main failures must remain
reported as failures. No assertions are weakened and no suite is skipped.

It records namespace owner label and UID, and uses UID/resourceVersion deletion
preconditions in `finally`, then confirms namespace absence. It does not clean
any preexisting namespace or other task's resources. Preserve the private receipt
and logs if interrupted; inspect ownership and unfinished Pods before resuming.
All logs/state/screenshots/credentials stay outside Git.

## Application acceptance

`run.mjs` uses the shared Console sign-in helper from
`e2e/lib/console.mjs`; create Gateway/endpoint Definitions and both native/Score
workloads through UI, save all drafts, Preview and Deploy once for the Environment.
Assert inherited database placement, Gateway identity preservation, rendered
HTTPRoute/Service/host/path/parent, real browser reachability, update/reuse and
ownership-protected API cleanup. Bootstrap configuration or an HTML shell is not
application acceptance. Read-only native assertions and namespace-local Fleet
access setup use existing shared helpers and separate owner/UID receipts.

Run phases with `ORCH_ROUTE_PHASE=configure|author|deploy|update|deploy-update|browser`.
Supply shared Console environment variables from `e2e/README.md` and an external
`ORCH_EVIDENCE_DIR`. Configuration requires explicit `ORCH_GATEWAY_CLUSTER_REFERENCE`,
`ORCH_GATEWAY_ROOT_DOMAIN`, `ORCH_GATEWAY_NAME`, `ORCH_GATEWAY_NAMESPACE`, and
`ORCH_GATEWAY_LISTENER`; no shared infrastructure identity is guessed. Authoring
requires `ORCH_WORKLOAD_CLASS`, digest-pinned `ORCH_ROUTE_IMAGE`,
`ORCH_SCORE_ENDPOINT_NAME` and `ORCH_NATIVE_ENDPOINT_NAME`. Browser checks require
actual `ORCH_NATIVE_APP_URL` and `ORCH_SCORE_APP_URL` verified over Tailscale.

The two workloads use the same namespace-bound PostgreSQL resource. A PostgreSQL
client init container checks real database availability before each web workload
starts. Native assertions separately check authenticated SQL, preserved database/
PVC/Gateway identities, HTTPRoute status and exact Service/host/parent intent.
Optional namespace-local Fleet setup reuses the existing shared helper with
`ORCH_SCORE_KUBECONFIG` and `ORCH_SCORE_CONTEXT`; it does not create/adopt namespace
or alter controller/global RBAC. Record and remove only the setup-owned Role and
RoleBinding (labels/UIDs must match) when application cleanup reaches that guard.

After backend changes, the full backend candidate suite must run again. The
runner accepts `ORCH_FULL_CHECK_PARTS=candidate` with
`ORCH_REUSE_FULL_RECEIPT=/private/completed/full-kind-receipt.json` only when prior
cleanup is confirmed, main SHA is identical, frontend checks passed and frontend
Git tree is unchanged. It copies earlier evidence without claiming a fresh run;
compare actual Go/Terraform tool versions and exact failure/skip sets. Failed or
changed frontend checks require including `frontend` in the rerun.
