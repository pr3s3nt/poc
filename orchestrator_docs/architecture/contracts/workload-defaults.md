---
id: WORKLOAD-DEFAULTS-CONTRACT
artifact: operation-contract
status: current
last_reviewed: 2026-11-06
---

# Workload defaults and complete runtime intent

Owns the data contract supporting [WD-01–10](../../usecase/UC-16/workload-defaults-specification.md).
New fields are optional additions to Generic Workload; legacy documents stay valid.
Existing JSONB desired/applied/module snapshots carry the extension without SQL DDL.

`containers.<name>.securityContext` has optional runAsNonRoot, runAsUser,
runAsGroup, privileged, allowPrivilegeEscalation, readOnlyRootFilesystem,
capabilities {add,drop}, seccompProfile {type,localhostProfile}. Pod-only fields
fsGroup, fsGroupChangePolicy and supplementalGroups belong to
`workloadExecution.podSecurityContext` with runAs*/seccompProfile. Integers and
booleans are pointers/optional values, never truthiness defaults. Privileged true,
privilege escalation true and Unconfined are rejected by the standard policy.

`workloadExecution` carries optional `templateDefaults` {environment,profile},
`automountServiceAccountToken`, `serviceAccountName`, controller-specific
`updateStrategy`, `revisionHistoryLimit`, optional `podDisruptionBudget`,
`volumeClaimTemplates`, `claimMounts` and Retain/Retain
`persistentVolumeClaimRetentionPolicy`. Explicit false/zero survive snapshots.
PDB quantities are nonnegative integers or percentage strings, with exactly one
minAvailable/maxUnavailable when enabled. StatefulSet remains one replica.

## Template-owned defaults — ADR-038

[ADR-038](../decisions/ADR-038-template-owned-workload-defaults.md) replaces the
removed backend policy contract. `workloadExecution.defaults` is rejected; the old
test data is deleted with user authorization. There is no legacy policy reader or
automatic migration of its numerical values. No SQL DDL is needed.

Each immutable WorkloadTemplate declares optional `runtimeDefaults`. Fields include
image, ports/probes, resources, container/auxiliary security, Service, volumes and
mounts, replicas/multiReplicaSafe, probeTimings, execution, and environments. The
execution branch can declare Pod security, token mounting, strategy, persistent
strategy, history, schedule, PDB defaults and claim retention. Probe timing defaults
cannot invent endpoints. Schedule defaults cannot invent a cron expression.
Environment variants overlay template base values before workload overrides apply.
Replicas >1 require the template's explicit compatibility declaration.

The backend supplies no platform/environment default values. Resolution is a pure
missing-leaf merge of authored values and the verified selected template profile.
It preserves false/zero, never rewrites the submitted document, and labels sources
as workload or template. PDB defaults apply only when explicitly enabled; probe
settings apply only to existing probes; schedule only to an existing schedule;
claim retention only when claims are explicitly created. Persistent strategy is
selected from template data while backend storage safety still requires Recreate.
Without a template, no new authoring profile is added. Render-bundle fallbacks are
owned by source files. Required identity/token/security and ownership guards remain.

POST `.../environments/{env}/workloads/resolve-defaults` uses scoped authoring
authorization, accepts {workload:<Generic Workload>} and returns
{workload:<effective document>,sources:<path-to-source>}. It has no writes/provider
calls/secret resolution. API admission validates the template pin, compares any
submitted frozen profile with `runtimeDefaults` selected for that environment and
fills `templateDefaults` with the verified profile. Save persists this snapshot
metadata and only the user's overrides. Planner rechecks target environment and
profile equality against the selected source. Pure readers reconstruct inherited
Services/images from that verified snapshot; renderer and Preview use the same
merge. Rollback uses the original pin and snapshot, without selecting latest values.

Source capabilities `template-defaults-v1`, `runtime-security-v1`,
`controller-policy-v1` and `stateful-claims-v1` are required for their features.
The standard 1.1.0 profiles own staging/production resource/history values,
security, rollout, probes and CronJob settings. Verified image profiles alone
publish non-root/read-only settings and writable paths. Service inheritance may
be disabled explicitly through `serviceEnabled:false`. Old backend-policy sources
are retired from the authoring catalog.

PDBs remain ordinary namespaced inventory and are pruned when disabled. Stateful
claims are retained controller-created storage, never independently adopted or
managed by the standalone PVC adapter. Workload removal does not delete claims;
namespace/Application cleanup refuses untracked retained storage. Server admission
checks unsupported fields before publication. None of these storage/identity guards
is removed with the old defaults policy.

Runtime validation uses complete-object create/replace `--dry-run=server`, with
resourceVersion for existing objects. It does not use client-side apply's list
merge, which can produce duplicate port names while changing a container port.
It never persists the validation object or changes Helm ownership.

Recovery of a rejected update before Git publication may settle its submission
as failed only after the original executor is stopped, the recovery lease is held,
both proposed Git publications are proven absent, the shared controller still
pins the exact previous revision/path/environment/digest, remote trees match and
all prior object UIDs/revision/Helm/organization ownership match. No new-only
object may exist. Any missing or conflicting proof keeps the Environment held.
Recovery performs no publication, adoption, pruning or implicit retry.
