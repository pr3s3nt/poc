# Namespace placement and template HTTPRoute — issue #84

Scope and assertions: [contract/sequence](../../../orchestrator_docs/architecture/contracts/namespace-template-routes.md),
[implementation gates](../../../orchestrator_docs/implementation/namespace-template-routes.md).
This scenario uses the existing Gateway/Traefik on the authorized kind cluster;
it never creates/adopts/updates/destroys shared Gateway, DNS or TLS objects.

## Full checks

`full-kind.mjs` requires Node >=22, Git, kubectl on PATH, an explicit scoped
KUBECONFIG and access to the existing `kind-k8s-4f` cluster. It creates one labelled
namespace and three disposable Pods: candidate/main backend (each with its own
real PostgreSQL 16 and Terraform 1.9.8), and candidate frontend (Node 22 Bookworm,
including bash for the cleanup-gate unit tests). Sources are exact
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

If the collector was interrupted, do not rerun completed tests. Confirm its
process is stopped, then use the same evidence directory and
`ORCH_RESUME_FULL_RECEIPT=/private/task/full-kind-receipt.json`. Resume validates
the recorded namespace name/UID/owner and Pod owner labels, collects the existing
results and performs guarded cleanup. It does not recreate Pods or upload sources;
the receipt retains its original source SHA. Never resume a completed/cleaned
receipt or run two collectors against one namespace.

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

Run `verify.mjs` with the same evidence directory, explicit
`ORCH_SCORE_KUBECONFIG` / `ORCH_SCORE_CONTEXT`, Gateway name/namespace/listener and
both browser URLs. It reads Kubernetes metadata/status and runs an authenticated
`SELECT 1` inside the receipt-owned PostgreSQL container; no credential is printed.
It records and compares namespace/database/PVC/Gateway/Deployment/HTTPRoute UIDs.
After `deploy-update`, `ORCH_ROUTE_VERIFY_ACTION=restart` recreates only the
native web Pod, checking namespace ownership, app/environment labels,
ReplicaSet-to-Deployment UID chain and API UID/resourceVersion preconditions.
It waits for a new ready Pod and repeats SQL/init/route/identity assertions.
Run the browser phase again after restart.

Cleanup sequence (never force-delete the product namespace):

1. Run UI phase `remove-workloads`, accepting each named workload confirmation;
   then `deploy-removal` previews both changes and submits one Environment Deploy.
2. Run native `ORCH_ROUTE_VERIFY_ACTION=cleanup-rbac`. It requires the successful
   removal receipt and empty Deployment/HTTPRoute inventory, then deletes only
   setup-owned RoleBinding/Role matching the recorded app label and UIDs, using
   API preconditions. This is setup cleanup, not Application cleanup evidence.
3. Run UI phase `delete-application`. Review the server-computed scope, confirm
   the exact receipt-owned Application name, submit once and observe the admitted
   deletion operation. A failed/unknown operation must be inspected before retry;
   never manually delete product resources to manufacture success.
4. Run native `ORCH_ROUTE_VERIFY_ACTION=cleanup-check`: namespace/PV must be absent
   and the preexisting Gateway UID unchanged. Preserve all receipts/logs outside
   Git, including failed harness attempts. A new review Application uses a new
   evidence directory and only reuses catalog Definitions after comparing values.

After backend changes, the full backend candidate suite must run again. Partial
reruns use `ORCH_FULL_CHECK_PARTS=candidate|frontend|candidate,frontend`.
Without a reuse receipt they run only the requested parts, explicitly recording
that subset; absent results are never treated as passed. To copy prior evidence,
set `ORCH_REUSE_FULL_RECEIPT=/private/completed/full-kind-receipt.json`. Prior cleanup
must be confirmed and main SHA identical. Reusing candidate backend results
requires the identical backend Git tree; reusing frontend requires all its checks
passed and its Git tree unchanged. The runner copies earlier evidence without
claiming a fresh run; compare actual Go/Terraform tool versions and exact
failure/skip sets. Failed frontend checks require including `frontend` in rerun.

## Echo Gateway refinement (2026-11-17)

Configuration defaults to the new Echo envelope. To keep the retained legacy
trial app unchanged, use `ORCH_ROUTE_SCOPED_DEFINITIONS=1`: create a new Application
through Console, then register uniquely named Gateway/endpoint Definitions with
app_id matching criteria via the existing criteria form. Save both workload
drafts before one Environment Deploy as usual. Use unused explicit endpoint names
and a separately configured task-owned web forward; do not overwrite old routes.
`ORCH_GATEWAY_DRIVER=existing-gateway` permits read/reuse of an existing legacy
Definition only, never an implicit migration. Receipts record driver and prefix.
