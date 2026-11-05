# One application Fleet GitRepo on real kind

Validates [ADR-036](../../../orchestrator_docs/architecture/decisions/ADR-036-application-fleet-gitrepo.md),
[UC-06](../../../orchestrator_docs/usecase/UC-06/specification.md),
[UC-07](../../../orchestrator_docs/usecase/UC-07/specification.md) and the
[delivery runbook](../../../orchestrator_docs/operations/delivery-reconciliation.md).

Requires explicit mutation authorization for the chosen dev Console, real Git
hosting and kind target. Bootstrap a fresh staging acceptance app with the current
schema-3 bootstrap helper first; use a separate isolated platform if a retained
schema cannot be upgraded without deleting other applications. Never reset a
retained database. Install E2E/bootstrap dependencies per the shared E2E guide.

Supply private staging and production bootstrap JSON files outside Git. They must
use the same Console, expected Application key, binding, pinned source/image inputs
and distinct state/report directories, verification URLs and forwards. The platform
Definitions must explicitly match both Environment types. Production here is the
Application's second Environment on the authorized dev cluster, not company prod.

Run from repository root, one checkpointed phase at a time:

```sh
node e2e/scenarios/application-fleet-gitrepo/run.mjs \
  --staging-config /private/staging.json --production-config /private/production.json \
  --backend-container application-fleet-live-backend-1 \
  --evidence /private/application-fleet-evidence --phase baseline
```

Phases: `baseline` (after staging bootstrap), `production` (after production
bootstrap), `scale`, `remove-production`, `remove-staging`, `restore-staging`,
`restore-production`, `final`. Stop on failure, diagnose/reconcile persisted server
operations before another mutation; do not reset receipts or rerun an unknown
submission. Reruns read existing completion checkpoints.

The runner uses the shared signed-in Console helper. Workload import, Preview,
Deploy and removal use actual UI controls. Native reads inspect only the configured
cluster and app namespaces. Git proof reads only generated tracked files from the
selected backend's scoped checkout, never its credential/askpass files.

Assertions cover one actual GitRepo/UID for two Environment trees, all active
Bundles ready at the common commit, full tree digests, template driver rendering,
PostgreSQL/Redis/PVC identity, application business checks and persistent job rows,
isolated staging scaling with unchanged production pins/UIDs, selective withdrawal,
last-unit GitRepo deletion, empty watched Git trees, repository/data reuse after
restore, unchanged Preview and real Tailscale browser content.

Side effects: changes only the receipt-owned acceptance app; scale backend, remove
and re-import its Workloads, create/retain app-scoped namespace access and access
forwards. PostgreSQL/Redis/PVC and metadata history remain during workload removal.
Both Environment Workloads and the app are restored and retained for review.
This runner does not install controllers, upgrade a retained DB, modify another
app/default, delete the remote repository or grant shared RBAC.

Cleanup after review uses product Application deletion with exact name/preview,
UID-checked cleanup of only setup-owned namespace Role/RoleBinding objects, then
native app/namespace/GitRepo absence checks. Preserve shared services and unrelated
GitRepos. Private configs, sessions, screenshots and results stay outside Git.

## Resume after a stopped deploy or backend runtime upgrade

Read the persisted operation, intent and actual Fleet state before any retry.
`recover.mjs --config /private/staging.json --operation <exact-id> --prior-stopped`
uses the existing Settings recovery controls. Supply `--prior-stopped` only after
verifying the prior owner cannot still execute (for example after its backend
container was replaced). This does not submit a new deployment.

A backend executable change invalidates the renderer binary pin. For an isolated
`dev-test` platform, `refresh-runtime.mjs --staging-config /private/staging.json
--production-config /private/production.json` registers the same exact Git source
content with the current runtime, creates/promotes its workload Definition revision,
and selects that revision through the versioned product API. It preserves the
previous Definition in private reports and updates only setup source/version
receipts. This is platform setup, not deployment evidence. Do not use against a
shared company platform without separate authorization.

After recovery and reviewing the desired state, `resume.mjs --config
/private/staging.json` runs actual Console Preview/Deploy and verifies template
pins, Fleet, application behavior and persistent data. Use `--prepare` only for an unverified Environment whose platform setup and
exact version selection are already reviewed; it seeds the declared configuration
and Workloads through Console controls before Preview. Use full-mode config files.
Any failure stops the runner
and preserves operation receipts; diagnose before another invocation.

When a phase has an UNKNOWN submission marker, run product Settings recovery for
that exact stopped operation first. Then pass `--resume-recovered` for the same
phase: the runner verifies the original review's single RECOVERED operation,
archives its setup attempt marker, preserves the recorded pre-mutation checkpoint
and prepares a fresh review of remaining desired changes. It never changes server
receipts, resets intents, replays unknown deploys or repeats already staged deletes.

If a defect fix changes the runtime executable after staging was restored, use
`refresh-staging` after the exact renderer/Definition pin setup and before the
production restore. It deploys the reviewed staging runtime change, checks data
UIDs and verifies production's applied runtime remains empty/unchanged. Declared
pending creation drafts are preserved; other phases keep strict readiness checks.

After successful handoff, `--phase final --verify-again` repeats only the final
readiness/browser checks and validates/recreates receipt-owned app forwards when
needed. It does not repeat any completed mutation phase. Preserve the same private
configs, state receipts, Compose volumes and scenario evidence directory.
