---
id: ADR-036
artifact: architecture-decision
status: current
last_reviewed: 2026-11-05
related: [ADR-030, ADR-033, ADR-035]
---

# One managed Fleet GitRepo per Application

## Accepted decision

The user approved one application delivery Git repository and one managed Fleet
GitRepo for both staging and production. Keep the existing bounded template driver;
do not introduce Helm charts, values or a second template engine. ADR-033 still
owns repository allocation and retention. This decision replaces per-phase managed
GitRepos for new application delivery; historical receipts remain cleanup authority.
The user confirmed old applications are disposable tests and authorized deleting
the old acceptance app through guarded product cleanup before creating a fresh
app. No live conversion or data migration is required; unrelated apps remain.

## Publication and controller identity

Use an application-scoped controller identity derived from Organization and
immutable Application ID, independent of Environment, generation and delivery epoch.
Both Environments retain the application's immutable binding/target pins.
The repository contains `<root>/<environment>/<workload>/<phase>/` directories,
where `<root>` is the existing platform policy root (normally `environments`).
The top-level `metadata` tree is reserved and cannot be a manifest policy root.
Each nonempty phase contains rendered YAML and `fleet.yaml`, producing a Bundle.
Phase separation is retained where needed for prerequisite/Job/readiness/route
semantics; it does not create more GitRepos. Nonsecret revision metadata lives
under `metadata/<environment>/<workload>/<phase>/`, outside watched paths.
Do not write source templates, secret bytes, Terraform state or Terraform-owned
namespace/database/storage objects into this delivery repository.

The GitRepo watches only admitted Bundle directories, grouped under the two
Environment trees. It pins one approved repository commit. Its durable metadata
records each admitted unit's path, intent, revision and complete tree digest.
Publication uses the existing repository/branch lease, Environment fences and
ordinary fast-forward pushes. A new pin must verify every retained unit tree at
the candidate commit, including the other Environment. Changing staging never
rerenders or republishes production. Reject foreign tree changes before advancing
the shared pointer; a branch update alone grants no deployment authority.

## Observation, recovery and removal

The current GitRepo SHA can advance while an earlier unit remains authoritative.
Verify the receipt's exact tree at the pinned/observed commit, match the unit's
Bundle by stable release identity, correlate Bundle/BundleDeployment commits and
check live object revisions, readiness and UID inventory. Never interpret a newer
repository SHA as authorization for different unit content. Do not use GitRepo-wide
force-sync to recover one phase or replay completed Jobs.

Withdrawal removes only the receipted unit subtree and its watched path under the
same publication lease. Wait for that Bundle, downstream deployment and release
metadata to disappear before UID-guarded object cleanup. Preserve all sibling
units and the other Environment. Delete the owned GitRepo with UID/resourceVersion
preconditions only when its last admitted unit is withdrawn. Keep Git repository
history per ADR-033. Unknown outcomes retain the operation hold and are retried
by observation with the original receipt.

For a first submission interrupted after Git publication but before controller
admission, explicit stopped-operation recovery may finish that same admission.
It must observe both exact generated manifest and intent metadata trees at one
remote commit, prove an owned existing Application controller with no pin for
that release, reject existing target objects, verify every retained sibling tree,
and use the recovered operation fence/publication lease before a UID and
resourceVersion guarded write. It never republishes Git, creates a missing
controller, replaces a prior unit revision or repeats an acknowledged Job.
A first prerequisite submission may instead be settled FAILED by explicit
read-only recovery when its exact candidate Git trees are not published, the
Application controller and every proposed native object are absent, and target
identity plus the recovered fence/lease are verified. It cannot infer absence
for Jobs, publish content or create a controller; any uncertainty stays held.
Acknowledged withdrawal is terminal source ownership: recreation has no prior
active revision/inventory.
Ordinary controller writes retry bounded API rejections after rereading and
revalidating ownership; uncertain transport outcomes remain held.

## Compatibility and validation

No database schema change or reset is required. Preserve historical per-unit and
preexisting-controller receipt handling; never silently adopt existing controllers.
New shared-controller receipt handles explicitly identify this layout. Unit and
adapter tests must cover two Environments, multiple workloads, exact-tree checks,
foreign ownership, lost acknowledgements, sibling-preserving removal and last-unit
cleanup. Live Fleet verification requires an explicitly authorized environment;
local/fake controller checks do not prove installed Fleet behavior.
