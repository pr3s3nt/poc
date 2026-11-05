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
disposable acceptance Application. New-layout live deployment has not been run;
explicit authorization for that Kubernetes/Git mutation was requested separately.

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

## Remaining gate

Rebuild/restart the selected dev backend and run a fresh acceptance Application
only after the pending external-verification authorization. Check one actual
GitRepo for both Environment trees, exact tree/Bundle/live readiness, update and
selective cleanup on Fleet v0.15.2. Existing historical six-controller evidence
cannot substitute for that gate. No new app or preview is claimed ready to open.
