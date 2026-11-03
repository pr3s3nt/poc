---
id: ADR-034
artifact: architecture-decision
status: current
last_reviewed: 2026-11-03
related: [ADR-033, ADR-032, ADR-030, ADR-027, ADR-029]
---

# Simplified CD configuration with reusable connections

Issue #31 approved simplifying Continuous Delivery configuration so the Platform
manages the entire GitOps/Fleet flow and users only choose reusable accounts.

## Decision

### Four-field CD configuration

A Fleet CD binding is authored with exactly four user-facing fields:

| Field | Behavior |
|---|---|
| Kubernetes Connection | Select an existing infrastructure account or register a new kubeconfig inline |
| Git Connection | Select an existing Git account or register a new one inline; Platform Settings Git remains the shared default |
| Fleet Workspace | Auto-discovered through the Kubernetes Connection, defaulted, reselectable |
| Target Cluster | Auto-discovered per Workspace, defaulted, reselectable |

Repository URL, branch, root path, internal namespaces, ServiceAccount, reader
Secret, CA bundle and Helm options leave the form. The Platform derives them:
controller/agent identities from read-only discovery, release topology from
fixed internal defaults, and repository identity from the per-Application
allocation of ADR-033. Discovered values are pinned into the immutable binding
version at registration; existing binding versions are unchanged.

### Auto-discovery

Read-only discovery lists every Fleet workspace and every registered
`clusters.fleet.cattle.io` as options instead of requiring exactly
one bootstrap cluster named `local`. Only proven physical target mappings are
selectable. The UI auto-fills when exactly one option
is eligible and always allows an explicit reselection. Discovery reports
bounded actionable reasons and never installs controllers or enrolls clusters.

### YAML manifests

Generated Kubernetes manifests and `fleet.yaml` are published as YAML, not
JSON, per Delivery Unit under `environments/<env>/manifests/orch-<unit-hash>/`.
Fleet consumes both formats; YAML is the human-diffable canonical form.
Publication remains deterministic (stable ordering and digests) and commit
SHA pinning of the Fleet `GitRepo` CRD is unchanged.

### Git connections as reusable accounts

`connections.kind` gains `GIT` with authentication type `GIT_TOKEN`
(URL scope, username, token). Credentials are write-only, stored through the
existing private credential store, and reusable across CD bindings. Inline
registration from the CD screen creates the same managed Connection record.
Platform Settings Git access (ADR-029) remains the shared default connection;
explicit Git connections are an override. Per-Application private repository
allocation, per-Environment/Delivery-Unit paths, SHA pinning and binding
immutability of ADR-030/032/033 are unchanged.

### CD binding stability

A verified binding pinned by an Environment does not move when the platform
default changes; retargeting stays an explicit validated operation (ADR-030/033).

## Consequences

- Schema gains version 7 allowing `kind='GIT'` connections (ADR-027); frozen
  versions 1–6 are unchanged.
- Verification (`Test Connection`) checks Git access and Fleet topology
  read-only before a binding becomes READY.
- Acceptance extends to YAML manifest shape, multi-workspace/cluster
  discovery, connection reuse/inline creation and binding stability.


### Proving the selected target

The selected Fleet Cluster becomes `GitRepo.spec.targets[].clusterName`; Fleet
performs manifest delivery. Platform observation and guarded cleanup still use
an independently authenticated Kubernetes target. Discovery may match the Fleet
registration's default `spec.clientID` to the `kube-system` namespace UID read
through an existing READY Organization-scoped Kubernetes Connection. Bootstrap
local registration additionally requires its known bootstrap markers and equality
with the manager's cluster UID. Names/endpoints alone are insufficient proof.
Only one physical identity may match; ambiguous or custom client IDs without an
identity mapping remain unsupported rather than silently selecting the manager.
The binding pins the discovered account key and Fleet registration UID. Deployment
preflight compares the actual provisioned workload target with that pinned account;
selecting a Fleet target never redirects a planner-selected infrastructure target.

Fleet documents its default registration identity in
[cluster registration](https://fleet.rancher.io/how-tos-for-operators/cluster-registration).


### Managed Fleet reader authentication

New four-field configurations derive a private basic-auth reader Secret per
immutable binding version from the selected Git Connection (or the shared
platform Git default). Credentials stay in the private store and the Kubernetes
Secret, never in repository files or public binding DTOs. The same selected
account authorizes publishing and Fleet fetches; no automatic Git-provider token
minting is assumed. Operators should scope that account to its intended owner.
Previously configured reader Secrets remain external prerequisites and are never
overwritten or adopted. Read-only Verify checks permission to create a managed
Secret; the admitted Deploy creates/refreshes it before writing the GitRepo.
Updates require exact Organization/binding/version ownership and resourceVersion.
Shared binding credentials remain for later deployments and are not deleted by
Application cleanup. Controller installation and cluster-wide RBAC remain operator
prerequisites; fixed release namespace/service account defaults do not grant rights.


### Exact Git rendering on explicit updates

Managed GitRepo updates preserve Fleet-owned annotations/finalizers/owner references.
An admitted new non-Job intent requests one Git render generation at the pinned SHA;
replaying that intent does not increment it. Existing Job units cannot use this
mechanism. Explicit receipted recovery may request it once when GitRepo reports
that SHA but its owned Bundle still carries another commit. No controller status
is written, no destination changes and no Kubernetes object is force-replaced.
Helm force/takeOwnership remain false. Verify checks managed GitRepo CRUD rights
before reporting READY.

The pinned Fleet implementation separates the
[Git job generation](https://github.com/rancher/fleet/blob/v0.15.2/internal/cmd/controller/gitops/reconciler/gitjob_controller.go)
from [Helm ForceReplace](https://github.com/rancher/fleet/blob/v0.15.2/internal/helmdeployer/install.go).
