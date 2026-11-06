---
id: UC-16-WORKLOAD-DEFAULTS
artifact: use-case-specification
status: current
last_reviewed: 2026-11-06
---

# Complete workload configuration — issue #37

Approved scope: [issue #37](https://github.com/pr3s3nt/poc/issues/37).
This extends [UC-16](specification.md); implementation/evidence are separate.

- **WD-01:** Support typed Pod/container security contexts, platform-owned
  ServiceAccount identity, tri-state token mounting, controller-specific rollout,
  revision history, optional PDB and StatefulSet claim templates. Form, Generic
  Workload import, desired/applied snapshots, reconstruction and rendering preserve
  absent, false, zero and explicit feature disablement. Unknown fields are errors.
- **WD-02:** New form workloads explicitly select immutable `workload-defaults-v1`
  with the authoring environment. Override > pinned template defaults > frozen
  platform/environment policy. Existing workloads without a policy retain legacy
  10m/32Mi request fallback and their stored intent. Opening/saving never adopts
  a new policy/template. Adoption is explicit and requires Preview then Deploy.
- **WD-03:** App containers start at CPU/memory requests 100m/128Mi in staging,
  200m/256Mi in production, memory limit 512Mi, no CPU limit. Init/Sidecar retain
  component-specific policy. Template profiles can supply larger resources and
  explicitly declare multi-replica compatibility; arbitrary workloads stay at one.
  Production web/API may use two only under that declared compatibility.
- **WD-04:** Arbitrary images do not acquire non-root UID or read-only root by
  assumption. Pod seccomp RuntimeDefault, token mounting false, container privileged
  false and privilege escalation false are policy defaults. Verified profiles may
  add runAsNonRoot, capabilities.drop ALL and readOnlyRootFilesystem with required
  writable mounts. Explicit UID/GID/fsGroup remain optional. Platform security and
  secret-delivery identity constraints override authoring choices or reject them.
- **WD-05:** Stateless Deployment defaults RollingUpdate 0/1; persistent mount
  Deployment stays Recreate until a verified sharing capability exists. StatefulSet
  RollingUpdate partition 0, no Deployment surge fields; replicas remain one.
  Revision history staging 3/production 10, preserving explicit zero. PDB is opt-in,
  requires >=2 stateless replicas and exactly one nonnegative integer/percentage
  minAvailable/maxUnavailable. Default on enable: maxUnavailable 1. AlwaysAllow
  requires supported cluster discovery. PDB controls voluntary eviction, does not
  promise uninterrupted service or replace rollout policy.
- **WD-06:** StatefulSet claim templates are explicitly created, named and mounted
  per container. Size/access modes required; storage class may inherit the cluster
  profile. Retention is Retain on deletion and scale down. Claims are owned by the
  StatefulSet controller, never by the standalone PVC resource adapter. Claim
  identity/configuration is immutable once applied. Removal/controller conversion
  cannot erase retained data; namespace/environment/application deletion must
  refuse while such retained claims exist. Separate explicit owned storage cleanup
  is necessary. No automatic adoption of same-name claims or independent PVC clone.
- **WD-07:** Keep App/Init/native Sidecar order, env references, command/args,
  probes, existing PVC/emptyDir/hostPath/ConfigMap/Secret, subPath, Services and
  CronJob. Add source at the target container, search/reuse sources first; mount
  deletion never deletes a source. Container rename/removal must expose or fix
  references explicitly. emptyDir is Pod-local. Preserve two independent writable
  mounts at /var/cache/nginx and /tmp. Secret files default 0400; hostPath defaults
  Directory/read-only; new emptyDir suggests 256Mi without inventing a PVC size.
- **WD-08:** Never invent probe endpoints. Readiness/liveness timing when enabled:
  period 10, timeout 2, failures 3, successes 1. Startup 5/30; initial delay zero
  with startup. Regular Init has no probes. CronJob requires schedule, inherits
  Asia/Bangkok unless an explicit timezone is stored, Forbid, backoff 0, success/
  failure history 3; omitted deadlines stay absent. Explicit zero is retained.
- **WD-09:** Metadata/default load errors preserve edits and permit read retry;
  no guessed fallback. Reset removes the override. Display effective values as muted placeholders without serializing them as
  overrides; source labels stay out of the form (user decision 2026-11-06). Groups belong to workload/container scope,
  collapse with useful summaries, reveal errors and support keyboard and VI/EN.
  Order common image/environment/compute edits above infrequently changed startup,
  rollout/security/default policy fields; replicas belong to Scaling & updates.
  Only name/image/environment variables and conditional CronJob schedule show
  initially; other groups belong to scoped Show advanced disclosures.
- **WD-10:** Preview is read-only; expose effective config/provenance, Pod rights,
  rollout/history, PDB, claim templates/retention and resource changes. Pin policy,
  template and target with existing stale admission. Deploy alone applies changes.
  Rollback reads the original policy/template/snapshot/artifact. Save does not
  undo separately committed Variable/Secret creation.

The policy tables and conditional exceptions in issue #37 section 5 define the
approved v1 values. No HPA, new controller or platform-wide redesign is included.
