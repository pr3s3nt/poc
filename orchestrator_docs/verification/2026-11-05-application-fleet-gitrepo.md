---
id: VER-APPLICATION-FLEET-GITREPO-20261105
artifact: verification-record
status: current
last_reviewed: 2026-11-05
related: [ADR-036]
---

# Application-scoped Fleet GitRepo refactor

Base: main `b0cdd093`. Task branch: `refactor/application-fleet-gitrepo`.
[Accepted architecture](../architecture/decisions/ADR-036-application-fleet-gitrepo.md).
The user approved one application Git repository/controller for both Environments,
keeping the existing template driver, and separately authorized deleting the old
disposable acceptance Application. The user subsequently authorized live acceptance
on dev kind k8s-4f and continued the interrupted work in the current chat.
Initial local-only results below are retained separately from the live continuation.

## Source and local verification

The adapter packages rendered YAML and Fleet configuration under
`<root>/<environment>/<workload>/<phase>/` (default root `environments`).
Revision metadata is outside watched paths at
`metadata/<environment>/<workload>/<phase>/revision.json`. Controller identity is
Application scoped; per-unit phase/release identities and readiness remain.
Publication verifies every retained tree before advancing the shared pointer.
Observation validates a unit's exact tree at the current shared SHA, its own
Bundle, downstream deployment and native revisions. Withdrawal preserves siblings
and retires the owned GitRepo only after the final unit. Read-only lost-receipt
recovery does not resubmit, push or force-sync Jobs. No schema, template engine,
Helm chart, Console UI, database reset or Terraform ownership change was added.

Checks:

- Focused Go packages: Fleet, application/delivery, Git publication, Store,
  PostgreSQL adapter and HTTP pass. Existing opt-in database/cluster integration
  tests retain their prerequisites; this is not a new real PostgreSQL upgrade run.
- Race detector passes for Fleet, delivery coordinator and Git publisher.
- `go build ./...` passes. No Console UI was changed, so frontend typecheck/lint/
  component/build checks were not rerun. No live new-layout browser E2E was run.
- Bootstrap package: 63 tests pass, zero skipped; changed scenario syntax passes.
- Documentation checker, PlantUML regeneration and `git diff --check` pass.
- Full `go test ./...` was attempted. It has 61 failing top-level tests in 14
  packages. A detached worktree at exact base `b0cdd093` reproduces the same 61
  names with no additional failures in the refactor run. Examples include stale
  standard-render fixtures, retired provisioner/schema assumptions and historical
  Score assertion names. These failures remain, without weakening assertions or
  changing unrelated tests to make the report green.

Adapter tests use a local bare Git remote and a Kubernetes transport subprocess
fixture. They cover two Environments/multiple workloads, unchanged production at
an advanced shared SHA, isolated update, sibling/last-unit withdrawal, wrong
controller UID, unapproved sibling content, readable-path takeover prevention and
lost submission acknowledgement. They do not prove installed Fleet behavior.
The native bootstrap/HTTPRoute assertions now require exactly one app controller
and correlate all active pins/Bundles at its common SHA.

## Authorized cleanup of the previous test app

Target: existing deploy-main Console :3031 and kind k8s-4f. The matched old app was
`acceptance-bootstrap-dev`, ID `8240e76c-6151-49b4-8ab3-55012fe271c2`.
Product deletion operation `8310d389-4900-4694-8d3f-eb3e1e73bf3b` completed on
2026-11-05 at 03:13:13 UTC (attempt 2), preserving original operation checkpoints.
The old deployed backend performed this cleanup; it is not shared-layout runtime
evidence.

The first attempt withdrew all three workloads and six old Fleet GitRepos and
destroyed PostgreSQL/Redis resources. Namespace destruction correctly stopped at
two setup-owned `bootstrap-fleet-access` Role/RoleBinding objects. Native reads
proved the old Fleet managers absent and both RBAC objects carried this exact
Application label. Removed only RoleBinding UID
`e9ceed15-95c6-463d-a9a2-0d87336c546f` and Role UID
`9763312f-0318-474b-8949-f153963e3692`, using UID/resourceVersion preconditions.
Then retried the product deletion with a fresh preview/typed-name confirmation.
No namespace guard bypass, state reset or broad native deletion was used.

Final native reads prove application namespace UID
`c54d3f7a-29ac-4a77-a923-9ee7536d449c` and all six original GitRepo UIDs absent.
All 15 unrelated GitRepo UIDs and the unrelated `dsfsdrwef` Application remain.
Remote repository history is retained per ADR-033. Shared services, controller,
Tailscale routes and user tmux sessions were preserved.

Private inputs/auth/session/checkpoint/native evidence is retained outside Git at
`~/.local/state/application-fleet-20261105/`. Verification logs are under
`/tmp/application-fleet-*.log`. Do not publish credential files or browser sessions.

## Authorized live continuation

The retained `deploy-main` installation has schema 7 and other applications. Its
old backend was restored rather than resetting data or deleting those apps to
satisfy schema 8. A separate Compose project `application-fleet-live` uses a fresh
schema-8 database and the authorized existing kind target. It retains the same
pinned Git Definitions/template contents and acceptance images. Production in
this scenario is the second Application Environment on dev kind, not company prod.

Application: `acceptance-fleet-20261105`, immutable key
`84258224-225c-4da3-b1ac-7b7b73a80c68`. Managed remote:
`https://github.com/pr3s3nt/orch-app-84258224225c4da3b1ac7b7b73a80c68.git`.

The previous session stopped after frontend prerequisite manifests reached Git
without its controller pin. Original operation
`d98490e0-7287-4375-888e-fdf6a4ec5de0` was recovered through actual Console Settings
with a stopped prior backend. Exact remote manifest/intent metadata and sibling
trees were checked, then the original first-unit admission completed against the
same controller UID. A later observation acknowledged it without republishing Git.

A second, independently reproduced defect blocked Preview after runtime pin
selection: empty `secretOutputs` becomes nil on Plan JSON persistence and changed
namespace/PVC execution hashes. The regression first failed with false UPDATE;
normalizing the empty contract fixes it while real secret-output changes remain
UPDATE. No data-migration guard was relaxed. Workload withdrawal also now resolves
its allocated Application repository instead of the URL-less platform policy;
a coordinator regression reproduced the failure before the fix.

Backend executable changes require current renderer binary pins. The setup helper
registered the identical pinned Git template content for each new executable,
created/promoted versioned workload Definitions and explicitly selected them via
product API. No old revision/artifact, database marker or retained receipt was reset.

Staging and production both deployed successfully through Console and passed the
real browser business checks, persistent job rows and backend PVC file checks.
The dual-Environment checkpoint proves one actual GitRepo, all admitted Bundle
trees at its common SHA, and unchanged staging manifests/revisions/native UIDs
after production deploy. Scale and both selective removals have passed: production withdrawal preserved
staging; final staging withdrawal deleted the last controller and all watched
manifest trees while retaining data identities. A rejected controller replacement
was recovered with original guarded withdrawal receipt, not native deletion.

Restoration exposed stale source selection: the coordinator selected an older
successful Submit even after its acknowledged withdrawal. A regression reproduced
that failure; normal and recovery source selection now respect the latest terminal
withdrawal. The initial unsubmitted prerequisite attempt was settled FAILED through
read-only absence proof in original operation
`61547028-9ecc-4d6e-962c-1655170f117c`, atomically releasing its Git lease. Git/native
objects were not created by that recovery. Unit checks reject published candidates,
orphan Helm releases and Job absence inference. Restoration subsequently passed after correcting retained-resource baseline reads.
UNREFERENCED records correctly retain the removal deployment as cleanup ownership;
the read-only baseline capture now follows that successful removal's exact immutable
base-set lineage for execution evidence. Scope/Environment and organization must
match; missing/foreign/cyclic history stays unverified. It does not alter resource
ownership or relax the data-update guard. A real production Preview then reported
all cluster/namespace/PVC/PostgreSQL/Redis resources RETAIN.

Continuation checks: Fleet/delivery/Git publication/HTTP race tests, allocated-repo
withdrawal regression, planner execution-intent regressions, `go build ./...`,
65 bootstrap tests (zero skipped), documentation validation and diff checks pass.
The earlier full-suite/baseline comparison is historical; it was not rerun as a
checkpoint for this focused runtime continuation. No frontend source changed.

Private configs, operation receipts, logs, checkpoints and browser evidence remain
under `~/.local/state/application-fleet-20261105/`; no credentials enter Git.

## Final live result — 2026-11-05 05:47:43 UTC

All checkpoints passed: baseline, production, isolated staging scale,
remove-production, remove-staging, restore-staging, reviewed staging runtime
refresh, restore-production and final. Production restoration preserved the
refreshed staging runtime. The final Preview has zero changes and no pending
Definition, Catalog, delivery or route updates in either Environment.

- One GitRepo `fleet-local/orch-app-1fd5d728cfea9d2f7d609531`, UID
  `5894be7a-d12e-41a7-b2ea-c30bb9c0c149`, admits 12 phase Bundles at common SHA
  `db2709785a5f75f36a8ef6517b709470045dddac`; all are Ready with exact Git tree proof.
- The previous controller UID `ab033037-2bb1-419b-b176-574054821a8b` was deleted
  when its last unit was withdrawn. Watched Git trees were empty before restore;
  the same retained remote repository was reused for the new controller.
- Both Environments retain their original two StatefulSet and three PVC UIDs.
  Browser checks read the same original completed job IDs and PVC file values /
  hashes. A previously verified missing file now fails verification rather than
  being silently seeded again; its local regression checks that no write occurs.
- All 15 unrelated controller UIDs and the original platform delivery default
  remain unchanged. Runtime changes are confined to the isolated Compose project.

Verified exact MagicDNS routes with real browser/backend data on this WSL host:

- [Console/Application](http://desktop-bdjp2rq-1.tail342836.ts.net:3046/ui/applications/84258224-225c-4da3-b1ac-7b7b73a80c68)
- [Staging workload](http://desktop-bdjp2rq-1.tail342836.ts.net:3048/)
- [Production workload](http://desktop-bdjp2rq-1.tail342836.ts.net:3049/)

Console screenshots are `evidence/staging-console.png` and
`evidence/production-console.png`; application/browser checks, scenario snapshots
and per-phase logs remain in the private state directory above. These are host
browser checks through Tailscale, not an assertion about every device's ACL.

Both Environments, namespace-scoped setup RBAC, data and app Service forwards are
retained for review. Tailscale Serve routes 3046/3048/3049 remain configured.
Restart the isolated stack with the private `start-live.sh` in that state directory;
retain its Compose volumes. The scenario `--phase final --verify-again` validates Service UID, can recreate
receipt-owned forwards if their processes stop, and repeats real browser checks.
No frontend source changed; focused/race/build/bootstrap/docs validation passed.
The historical full-suite baseline failures above were not used as a green gate.
