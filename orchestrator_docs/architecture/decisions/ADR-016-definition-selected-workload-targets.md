---
id: ADR-016
artifact: architecture-decision
status: current
last_reviewed: 2026-10-24
---

# ADR-016 — Definition-selected workload lifecycle and Kubernetes targets

Accepted 2026-10-18 by the user in issue #4 V03 execution conversation (option B).
This supersedes ADR-013 for internal execution and the internal deployment
Connection selection/transition requirements of ADR-012. ADR-014 state,
ownership, secrets, fencing and cleanup guarantees remain. AWS legacy removal
is deferred to V05–V06; no AWS execution is authorized by V03.

## Decision and rationale

Resource Definitions and explicit resource references determine infrastructure
and workload placement. Application/Environment do not select the Internal
account. Internal cluster is a normally matched `k8s-cluster` resource using
public driver `existing-cluster` and its own READY Internal Driver Account.
No builtin binding, host/default credential, executionProfile filter or winner
fallback may redirect it. An account contains private auth; references/outputs
carry only typed opaque target identity and safe metadata.

Each workload/resource has its own persisted target pin, including cluster
descriptor, account identity/version and namespace identity where relevant.
Do not use a global last-resolved cluster, a fixed Application target or an
Environment account to execute consumers. Namespace identity must be qualified
by cluster in execution/reuse checks, even if its display name is unchanged.
V03 supports one resolved Kubernetes target per Environment Candidate Set.
Different workload targets are rejected before external calls with a clear
unsupported-multi-cluster error, rather than a permanent data-model invariant.
This keeps future multi-cluster support possible without redesigning placement.
Cross-cluster networking, Service discovery, routing/UI and live coverage are
deferred; registering multiple accounts is supported now.

## Workload driver

Keep `score-k8s` as the supported workload driver and installed immutable render
bundles. It selects lifecycle policy (render, deploy, readiness, update, remove)
through the existing UC-06/07 ports; it does not require a new process or public
driver protocol. Select the workload Definition before expanding references
and provision rules to a fixed point. Workload remains excluded from UC-08
resource batches. Resource outputs resolve before render. No fallback renderer.

Use the existing Score subset plus optional `metadata.class` (product extension,
nonempty class identifier, default `default` when omitted). Class persists in
the module and workload descriptor/matching intent; changing it invalidates
Preview. Score read/import/edit/export must preserve it. It is not a claim of
upstream Score/Humanitec schema compatibility.

The canonical driver input shape remains `driverInputs.values.variables`:

```yaml
render_bundle: score-k8s-internal-v1
namespace: ${resources['k8s-namespace.default#environments.@app.@env'].outputs.name}
cluster: ${resources['k8s-cluster.internal#environments.@app.@env'].outputs.cluster}
```

`render_bundle` is a literal installed bundle ID. `namespace` and `cluster` are
whole typed resource-output references, not literal names/account keys or
credential paths. Namespace Type exposes string `name`; cluster Type exposes
opaque typed target `cluster`. Both references form graph edges; namespace's
Terraform Kubernetes provider uses the same cluster descriptor through
`provider_bindings.kubernetes = {target: resource, resource_ref: ...}`.
Reject contradictory namespace/cluster targets, missing/ambiguous targets,
wrong output types, unknown fields and cycles before execution. General
Driver Inputs and provision params retain existing Type validation and
reference/substitution rules; V03 workload Driver Inputs accept only render_bundle, namespace and cluster.
No extra typed workload variable is supplied to rendering, so supplying one
is rejected rather than silently ignored. A workload Type with required inputs
is incompatible with this driver; optional Type inputs do not create authoring
controls or accepted driver variables. Score resource params and companion rule
params retain their full existing typed/reference semantics. No new workload
parameter mapping or PE patch authoring is introduced. Provision flags retain
UC-06 BR-08.

Existing-cluster Definition requires READY scoped Internal account and no raw
credential or account-derived context variables in its authored inputs. Its
output contract supplies safe `name`/context metadata as required plus opaque
`cluster`; public DTOs redact private identity/credential refs. Resolve and
check original account at invocation; API access/permission/readiness failure
blocks consumers. This resource is PREEXISTING, never created/destroyed by run.

Workload lifecycle results expose safe metadata/readiness through existing
Deployment/Workload status views. V03 does not publish workload resource-output
bindings: workload Type outputs must be empty for this driver; reject unsupported
outputs before execution rather than accepting declarations never supplied.
Namespace/cluster and other resource outputs remain fully supported. Manifests
remain internal render results, never public credential-bearing outputs.
Retain existing object identities, Secret bridges, immutable renderer pins,
resource bounds and readiness/current-set commit rules. Update/Remove use
persisted applied targets and identities across restart/catalog edits.
Remove marks resources unreferenced; separately confirmed owned cleanup may
delete namespace/resources, never the existing cluster or unrelated objects.

## Settings removal and delivery gate

After the replacement real happy path passes, V03.10 removes deployment
Connection selection and the internal builtin/profile execution path. Secret
Store settings/configuration remain. Old deployment-Connection writes are
rejected clearly rather than silently accepted or converted. Internal Preview
and Deploy require matched Definitions, not Environment configuration. Do not
require a Secret Store when this frontend-only workload has no secret binding.
No old-data migration is required; incompatible SQL/JSON baseline must fail
clearly, never reset itself. Codex may reset the explicitly verified dev
instance under the user's permission; writer cannot touch retained runtime.

V03 verification uses the actual acceptance frontend, with backend/jobs/PG/Redis
deferred to V04 by explicit user decision. Read the coverage map in the
[V03 handoff](../../verification/2026-10-18-v03-handoff.md).
Humanitec matching/graph/driver ideas are references, not full compatibility.

## Required verification

Pure matching/graph tests; workload Definition references and companion flags;
no target configured; class/default/class-change round-trip; missing/tied
Definition, account kind/org/status/version, typed target mismatch, cycle,
unsupported multi-cluster and no fallback; namespace provider agreement;
output-before-render; readiness gating; SQL reopen/immutable original targets;
cleanup owner/UID guards and old deployment-write rejection. Real headed UI
frontend import/Preview/deploy/update/restart/remove/cleanup on existing cluster,
with negative permission/account/unready-target checks and reviewed video.

## Git rendering amendment — 2026-10-24

[ADR-021](ADR-021-git-render-bundles.md) supersedes installed-only/Go-owned
rendering policy and exact-three-workload-variable restrictions for Git bundles.
Legacy installed pins retain their contract. Terraform provisioning, dependency
matching, target/state/ownership and cleanup rules in this ADR are unchanged.
Git behavior, inputs, cache and versioning follow the linked contract; this
amendment does not claim implementation or authorize retained-data migration.
