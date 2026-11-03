---
id: VERIFY-SIMPLIFIED-CD-20261104
artifact: verification-record
status: current
last_reviewed: 2026-11-04
related: [ADR-034, ADR-033, ADR-027, UC-04, UC-05, UC-06, UC-18]
---

# Simplified CD configuration — issue #31

Scope: [ADR-034](../architecture/decisions/ADR-034-simplified-cd-configuration.md),
UC-04 CD-08..11, reusable inline Connections, workspace/target discovery,
managed reader authentication, YAML publication and the real private per-Application
GitHub → Fleet → Kubernetes flow. The user authorized mutations on dev and
ownership-scoped cleanup. The separate staging installation was not modified.

## Environment and preserved installation

Dev is Compose project `deploy-main`, Console port 3031/backend port 8091,
real PostgreSQL/Vault, kind `k8s-4f` and Fleet v0.15.2. The tested Console URL is
`http://desktop-bdjp2rq-1.tail342836.ts.net:3031/ui/platform/cd-providers`.
This is a Console URL, not the acceptance workload web endpoint. Authenticated
Playwright opened this exact Tailscale origin, checked the actual page and
same-origin APIs without interception. This proves access from the host through
Tailscale; no claim is made about every device or tailnet ACL.

Before schema 6→7, dev writers were stopped and the actual database was backed
up and restored into an isolated rehearsal database. All original public table
row counts, installation UUID and creation time matched the restore and upgrade.
The migration owner then applied version 7; runtime restarted with its existing
restricted credentials. Seven immutable migration-history entries were verified.
No reset or manual marker/checksum update was used. The rehearsal container was
removed. Private backup/evidence remain under
`~/.local/state/issue31/20261103/`; no credentials, database archives or browser
sessions were added to Git.

## Executed checks and boundaries

| Check | Result |
|---|---|
| Backend `go test ./...` and `go build ./...` | Passed; real PostgreSQL upgrade checks run separately as below |
| Frontend typecheck, lint, tests, build | Passed: 96 files, 1,001 tests; existing hooks/React act and bundle-size warnings remain |
| Bootstrap Node tests | Passed: 34, zero skipped |
| Shared E2E Console helper tests | Passed: 6, zero skipped |
| Read-only CD/Application Settings browser scenario | Passed through the actual Tailscale Console origin |
| Documentation/PlantUML and migration guard | Passed; diagrams rendered and syntax checked |

The immutable migration/version guard passed. Disposable PostgreSQL upgrade tests
covered released prior schemas, populated version 6 retaining its Kubernetes
Connection, version 7 Git persistence, repeated setup, rollback and runtime DDL
isolation. Initial runs exposed stale historical fixture grants/constraints and
a future-version fixture that collided with the new version 7; corrected tests
were rerun successfully. A full adapter run exceeded its original ten-minute
timeout; it is not recorded as a passing full adapter suite. All disposable
PostgreSQL test containers were removed by their cleanup traps.

The real CD form uses the existing AppShell, styles, shared Button/Dialog and
KubeconfigField. Visual comparison used the previous dev CD page and the final
authenticated four-field form. No shell/navigation identity was changed.
Technical UID and redundant controller summaries were removed; connection
selection, discovery, validation and save actions remain visible. The final
screenshot is private `e2e/final-four-fields.png` under the run directory above.

Live setup passed inline Kubernetes/Git registration, discovery, managed-reader
binding verification and explicit platform default selection. Existing
Applications' desired/applied binding pins were recorded and remained unchanged.
Registration-only evidence is not treated as deployment acceptance.

## Runtime findings addressed

The first real baseline deployment succeeded. Strict read-back exposed the
acceptance fixture's noncanonical service alias; it now uses `svc_backend_http`
and retains the full equality assertion. Baseline business checks subsequently
passed with real PostgreSQL, Redis and job data.

Update exposed a keep-policy VSO retirement deadlock. The coordinator waited for
clean Fleet readiness before pruning acknowledged objects omitted by the new
artifact, while Fleet reported those retained objects as deletion-only drift.
The fix requires fresh complete exact-revision healthy inventory and re-proves
every difference is an omitted acknowledged UID before the existing fenced
ownership remover can prune. Observation remains read-only and OutOfSync until
clean reobservation. No unknown UID or unrelated drift is accepted.

Update also exposed GitRepo status at the pinned new SHA with a Bundle still at
the old SHA/content. The exact commit gate correctly held deployment. Managed
updates now preserve Fleet metadata and request one Git render generation per
new admitted non-Job intent. Explicit receipt-scoped recovery can request it
once for a proven mismatch; status is never forged and Helm force/takeOwnership
remain false. The repaired Bundle was observed at the exact expected commit.
An immediate recovery attempt returned 409 while Fleet rendered asynchronously;
a later explicit recovery succeeded after the new Bundle was observed.

The update image set initially existed only in the host Docker store. All three
images were loaded into the authorized kind node before resuming; this was an
environment prerequisite correction, not a reduction of rollout assertions.

All three updated Deployments subsequently became Ready and their delivery
intents were acknowledged. A rollout retired the pod used by the old service
port-forward; the initial browser verification therefore lost its connection.
The bootstrap verifier now probes access and reconnects only an owned forward
with matching namespace/service/kubeconfig/port command arguments. Rerun observed
an unchanged Preview, did not redeploy, and passed all five web business checks
with the original job ID and PostgreSQL/Redis/PVC identities retained.

Technical verification then passed for the private Application repository,
six GitRepos pinned to exact SHAs, corresponding rendered Bundle commits and
19 YAML files. The managed reader ownership and original applied binding were
checked independently; no token or Kubernetes Secret manifest appeared in Git.

## Acceptance completion

Baseline/update/rollback passed on application
`6a67dc62-980e-4e02-b962-3603514974e0`. Binding
`20603e81-f569-4444-b0ce-aa84c2732ced` version 2 reused the existing Git Connection
without token re-entry. After version 2 became the platform default, the Application
kept desired/applied version 1. Rollback deployed the previous image set through
ordinary Preview/Deploy, retained the original data/PVC/job identities, and a
second Preview reused unchanged state without deployment. The configured
acceptance job kept ID `1`; all five business checks passed after rollback.

Final post-rollback Git/Fleet proof also passed for six units and 19 YAML files,
including `helm.force=false` and `takeOwnership=false`. Product cleanup then
marked exactly three workloads for deletion, Preview asserted three REMOVE
actions, and Deploy completed successfully with zero native Deployments.
The initial cleanup runner used a selector that missed the workload-name icon;
it made no removal writes and the empty Preview kept Deploy disabled. The selector
and native confirmation handling were corrected, and the three-workload assertion
was added before rerunning.

Only the run-labeled setup Role/RoleBinding were deleted with UID preconditions.
Application deletion used the ordinary typed-name confirmation. Final assertions
proved Application API 404, original namespace absence and zero PVs referring to
that namespace. The original platform default was restored through the Console;
both recorded preexisting Environment desired/applied selections stayed unchanged.
The run-owned workload forward was stopped; workload port 3033 is no longer a
handoff endpoint. Dev Console port 3031 remains running.

Reusable Kubernetes/Git Connections, binding versions 1 and 2, the binding-1
reader Secret and private Git repository history are deliberately retained under
the product lifecycle. No unrelated preexisting resources were deleted. Private
`e2e/setup.json` records `setupComplete`, `rollbackPassed`,
`workloadRemovalPassed` and `cleanupPassed` as true, with ownership receipts.

The reproducible scenario and exact assertions are in
[`e2e/scenarios/simplified-cd`](../../e2e/scenarios/simplified-cd/README.md).

Remote/default-clientID target matching has unit coverage. This live topology is
Fleet bootstrap-local only; arbitrary custom IDs or ambiguous mappings remain
unsupported. Fleet installation, release namespace and ServiceAccount/RBAC remain
operator prerequisites. The managed private reader Secret is created by the
first admitted Deploy, not by read-only Verify.

## Service restart and retained evidence

Dev runs task images `orchestrator-backend:issue31` and
`orchestrator-frontend:issue31`. After an ordinary host restart, existing volumes
retain dev database/Vault data. Restart the already upgraded services with:

```sh
BACKEND_IMAGE=orchestrator-backend:issue31 \
FRONTEND_IMAGE=orchestrator-frontend:issue31 \
FRONTEND_PORT=3031 BACKEND_PORT=8091 \
docker compose -p deploy-main -f docker-compose.yml \
  -f deploy/local/compose.kind.yml -f deploy/local/compose.bootstrap.yml \
  up -d --no-deps --no-build --wait backend frontend
```

Tailscale Serve retains the existing port-3031 mapping to this Console.
Private run configuration, ownership receipts, screenshots, Git/Fleet proofs,
bootstrap state/reports and the schema-6 backup stay outside Git. Logs under
`/tmp/issue31-*` preserve the earlier failures as well as corrected runs.
