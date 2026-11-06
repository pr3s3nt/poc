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

`workloadExecution` additionally carries optional `defaults` {version,
environment}, `automountServiceAccountToken`, `serviceAccountName`,
`updateStrategy` {type,rollingUpdate:{maxUnavailable,maxSurge,partition}},
`revisionHistoryLimit`, `podDisruptionBudget` {enabled,minAvailable,
maxUnavailable,unhealthyPodEvictionPolicy}, `volumeClaimTemplates` [{name,size,
accessModes,storageClassName?,volumeMode?}], `claimMounts` [{claim,container,path,
subPath?,readOnly}], and `persistentVolumeClaimRetentionPolicy`
{whenDeleted:Retain,whenScaled:Retain}. Omitted features create no resource.
PDB quantities are JSON integers or percentage strings. Claim retention cannot
be configured to Delete in v1. StatefulSet remains exactly one replica.

`defaults.version=workload-defaults-v1` freezes policy behavior; environment is
staging/production and must match the target on admission. The reference survives
all snapshots and reconstruction. New policy versions never modify old resolver
code. Absence selects legacy behavior. Source defaults are pinned by the existing
immutable template source/preset hash; template profiles are part of that pin.
The backend resolves a copy for validation/render/Preview; desired overrides are
kept separately. Field-level source is workload, template, environment or platform.

POST `.../environments/{env}/workloads/resolve-defaults` uses the existing app/env
authorization, accepts {workload:<Generic Workload>} and returns
{workload:<effective document>,sources:<path-to-source>}. It performs no writes,
provider calls or secret resolution. The form uses this same resolver; failed
metadata requests keep the draft and block operations requiring the result.

New source capabilities `workload-defaults-v1`, `runtime-security-v1`,
`controller-policy-v1`, `stateful-claims-v1` are checked before provisioning.
Explicit identity must match authorized delivery identity; managed ServiceAccount
is read-only when no allow-list mechanism is published. Token-required delivery
reports platform provenance, rejects explicit false, and enables required token
mounting instead of breaking Vault. Rendering guards require declared effective
security/rollout/PDB/claim fields and exact workload selectors/ownership.

PDBs are ordinary namespaced workload inventory and are pruned when disabled.
Claims created by StatefulSet are retained controller-created storage, excluded
from standalone resource apply/delete inventory. Workload policy snapshots retain
claim identity; changes and namespace deletion need storage lifecycle checks.
Preview includes the effective artifact with PDB/claim retention and never creates
claims. Cluster discovery must reject unsupported API fields before apply.

Template `runtimeDefaults` may supply the app image, ports/probes, resources,
container/Pod security, Service, and required Pod-local volumes/mounts. It is
hashed in the immutable preset pin. `serviceEnabled:false` explicitly disables
an inherited Service; omitted inherits, true requires a valid effective port.
Production replicas >1 require `multiReplicaSafe:true`. Only verified image
profiles publish non-root/read-only settings and corresponding writable paths.

On admission the backend freezes verified nonsecret template `runtimeDefaults`
inside `defaults.template`. This is policy metadata, not workload overrides. Its
contents must equal the profile covered by the requested immutable template pin;
clients cannot substitute a profile. Pure effective-value readers use this frozen
snapshot for Services/references and complete-image validation without rewriting
intent. Save fills it only after scoped catalog validation. Existing workloads
without defaults remain unchanged.

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
