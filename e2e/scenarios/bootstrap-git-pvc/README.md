# Git-only bootstrap and backend PVC retention

Scope: [direct bootstrap](../../../orchestrator_docs/implementation/direct-bootstrap.md),
[Generic Workload](../../../orchestrator_docs/architecture/contracts/generic-workload.md),
[storage acceptance](../../../orchestrator_docs/operations/acceptance-storage-schedule.md).
Uses existing bootstrap/idempotency helpers, real Console mutations and native
read-back. No Catalog, SQL seed, fake adapter or direct workload apply.

Prerequisites: Node, Git, dependencies/Chromium from [E2E guide](../../README.md),
reviewed private dev config with `gitOnly=true`, schema-8 real Console, authorized
kind cluster, Fleet/VSO/Vault and all three acceptance images available there.
Git sources must have exact commits and SHA-256 pins. Credentials are private
file references; config/cache/state/reports stay outside Git.

```sh
node e2e/scenarios/bootstrap-git-pvc/run.mjs --config /private/bootstrap/dev.json
```

Runs platform-only → full acceptance → unchanged rerun, compares server identities,
then proves backend mounts a Bound owned PVC at `/data`, writes/reads the configured
public test value through the app browser and records its hash. Verifies namespace
UID and Pod → ReplicaSet → Deployment owner UIDs, deletes that one task-owned Pod
with UID precondition, then checks replacement readiness, unchanged PVC/Deployment
UID and retained file/hash. A final bootstrap rerun must preserve deployments and
Git/Definition pins. Every assertion must pass; failures preserve evidence.

Side effects: same platform/app/resources as bootstrap, one public storage value,
one UID-guarded backend Pod deletion/recreation. This command is explicitly
mutating and restricted to reviewed dev input, never staging/AWS by implication.
Application/data/Git history/forward remain for review/resume. It does not remove
PVCs, reset shared data or change controllers/RBAC. Cleanup is a separately
requested product remove/Preview/Deploy and Application deletion after checking
the recorded app/namespace ownership and UID. Never delete shared/preexisting
resources to create an empty platform. Evidence is in the configured private
reportDir; screenshots contain only public test data, not credential forms.

## Video evidence

Set `runtime.recordVideo: true` in the private input (enabled in Git target
examples). Only the final successful complete scenario retains a recording:
`runtime.reportDir/videos/backend-pvc-retention.webm`, with a JSON status
receipt. The clip shows the real public app and file/hash before and after Pod
recreation. The runner disables recording in intermediate bootstrap/idempotency
runs and discards failed recordings. A later successful scenario replaces the
previous video; a failed retry preserves the last success. Console credential
entry is excluded. CLI and Kubernetes operations run outside the recording.
