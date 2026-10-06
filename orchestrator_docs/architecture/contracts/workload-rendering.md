---
id: WORKLOAD-RENDERING-CONTRACT
artifact: operation-contract
status: current
last_reviewed: 2026-10-06
---

# Definition-selected workload rendering — contract

This document defines the accepted boundary for [ADR-010](../decisions/ADR-010-score-k8s-workload-rendering.md).
Implementation follows this contract; delivery evidence is recorded separately.
The existing [operation contracts](operation-contracts.md) retain their lifecycle rules.

## Registration shape

The example uses the internal Definition document field names. Registration
requires the server to have the pinned renderer configured.

```yaml
key: workload-score-k8s
resourceType: workload
executionProfile: internal-k8s
driverType: score-k8s
driverInputs:
  values:
    variables:
      render_bundle: score-k8s-internal-v1
criteria:
  - app_id: shop
```

`render_bundle` is a platform-managed immutable identifier, not a file path or
URL. A bundle resolves to exact renderer version/binary digest, adapter contract
version and content digests of the ordered provisioner/patch files. The
inspector must verify availability before registration/planning. An unknown,
mutable or incompatible bundle fails before persistence/provisioning.

Driver Inputs deliberately retain `values.variables`; no generic
`source`, `secret_refs`, inline command or template-upload API is added. The initial bundle is embedded in the backend and pins score-k8s 0.15.0,
its configured binary digest and the adapter/patch content.

Workload Definitions use the current profile guard and specificity rules.
Preserve `workload.default#modules.<id>` and reject ties or unavailable selected bundles.
The legacy renderer is the explicit built-in selection when no eligible
workload Definition matches; a matching Definition overrides that default. Remove actions retain the previously
applied object identity; changing renderer is not permission to orphan objects.

## Rendering operation

Logical contract; implementation extends the existing RenderRequest/WorkloadRenderer
ports rather than adding a public rendering endpoint:

```text
RenderWorkload(
  organization, application, environment, workloadID,
  candidateModule, referencedServiceCatalog,
  namespace, requiredLabels, imagePullSecretReference,
  pinnedDefinition, pinnedRenderBundle,
  dependencyBindings, configurationSecretBindings
) -> manifests
```

| Input | Meaning |
|---|---|
| Scope | Organization/Application/Environment ownership; never inferred from a score-k8s UID |
| candidateModule | Immutable validated product module; normalized to upstream Score without modifying the persisted document |
| referencedServiceCatalog | Non-secret Service names/ports from the complete Candidate Set, including workloads not being redeployed |
| namespace | Already provisioned by UC-08; renderer cannot create another namespace |
| pinnedDefinition/bundle | Definition identity/content hash, bundle digests, renderer and adapter versions |
| dependencyBindings | Workload aliases mapped to canonical private/shared descriptors, non-secret outputs and typed Secret references |
| configurationSecretBindings | Revision-specific UC-12 Secret names/keys after synchronization; no Vault token or cluster credential |

Outputs are execution-internal manifests. Existing delivery derives the Deployment
readiness reference from the protected workload identity. Plans retain safe provenance
(renderer version/bundle digests/Definition identity). Resource outputs and
secret values are not public provenance. Do not store a hash of raw secret values
as public Preview data or log unredacted subprocess output.

## Output and secret bridge

1. UC-08 completes and validates outputs before render; unresolved required
   outputs fail. Map Score aliases through the planner's bindings, not textual
   guesses from resource names. Same shared descriptor has one output source;
   scopes prevent two Environments sharing a credential accidentally.
2. Produce reviewed output-only provisioner inputs for score-k8s. Disable default
   provisioners; forbid resource-creating command provisioners. Every dependency
   must resolve through the supplied bridge; no fallback to an upstream default.
3. Non-secret outputs, such as database host/port, become substitutions. Secret
   outputs become encoded references to a Kubernetes Secret name/key; adapter
   materializes the needed namespace-local Secret using existing secret delivery.
   score-k8s need not receive the database password bytes. Verify the encoding
   helper/renderer behavior against the selected version rather than hand-building
   a magic string in public Score.
4. UC-12 VSO synchronization keeps its existing revision/ownership checks.
   Legacy Vault Agent mode must be supported equivalently or rejected explicitly
   before provisioning for a score-k8s selection; never silently drop injection.
5. No database StatefulSet, PVC, namespace, cloud resource or raw credential is
   permitted in score-k8s output for this slice. Secret reference validation
   checks scope, name/key and existing delivery contract before apply.

Disposable workspace only contains normalized non-secret inputs and references.
Use private permissions and cleanup on success/failure. Do not import arbitrary
URLs at render time; no host credential inheritance for provisioners. Generated
names must depend on product identity, not a fresh score-k8s resource GUID.

## Normalization and manifest postconditions

- Product flat probes map to upstream nested `httpGet`; preserve semantics.
- Product replicas map through controlled platform rendering/patch settings.
- Apply UC-06 BR-11: explicit requests preserved; missing field uses same-field
  limit, otherwise `10m`/`32Mi`; limits only as declared. No input mutation.
- Resolve product Service references against the complete Candidate Set even
  when only one workload is rendered. No regeneration/apply of unrelated workloads.
- Keep Environment public paths outside score-k8s route provisioning: existing
  `PublicRouteManager` owns the single Ingress and route-only retry behavior.
- Preserve workload/Service names, labels, ports, selectors, image-pull references
  and UC-12 revision delivery. Detect patch attempts to alter protected identity,
  namespace, secrets or required ownership before apply.
- Output contains only the target workload Deployment/Service and approved
  configuration objects. Validate object identity uniqueness, target namespace,
  API kinds, readiness refs, secret handling and resource ownership after patches.
- Rendering creates no Kubernetes/cloud side effects. Generation failure cannot
  invoke apply. Apply/readiness failure cannot commit Candidate as current.
- Fleet receives only non-secret workload objects. Secret delivery precedes
  workload readiness; the database and `.score-k8s` workspace never enter Git.

## Preview and execution consistency

Preview performs deterministic selection/normalization/contract checks without
running provisioning or resolving secret bytes. Expose selected Definition,
renderer/bundle version and unresolved dependencies in a safe projection.
Do not present Preview as an exact Kubernetes manifest diff for resources whose
outputs do not exist yet.

Pending tokens bind non-secret rendering intent: Candidate/base version,
Definition content, bundle digests and configuration revision identifiers.
Changed intent requires another Preview before side effects. At Deploy, UC-08
supplies outputs; render once, validate, and deliver that result unchanged.
Rendering selections are stored in existing deployment plan JSON; native selections
use the zero-value/omitted entry for legacy hash compatibility. Pending Preview
compares current selection with each workload's last persisted plan, so changing
a Definition triggers an update even without a Score draft.

## Concrete example

```text
shop/staging: backend Score declares db id=shop-db; frontend declares a Service
  -> planner selects postgres-internal-statefulset and workload-score-k8s
  -> UC-08 resolves cluster, applies namespace and PostgreSQL, waits Ready
  -> binding db -> canonical shared descriptor -> host/port + Secret reference
  -> score-k8s renders backend Deployment/Service using those outputs
  -> delivery applies backend and waits Ready
  -> render/deliver frontend; orchestrate workload order through existing flow
  -> PublicRouteManager reconciles '/' and '/api' from final Candidate Set
```

Workload order is not inferred merely from a literal `BACKEND_URL`.
Redeploy reuses existing database identity/Secret/PVC through UC-08; score-k8s
does not own them. Removing backend deletes its workload objects, subject to
existing shared-resource reference classification; it does not destroy PostgreSQL.

## Impact and planned verification traces

These are implementation and verification obligations; actual test evidence
is tracked in traceability and verification records.

| Artifact | Change / acceptance evidence |
|---|---|
| UC-03 specification BR-08/11/12, realization, sequence/VOPC, OC-04 | Add supported workload driver/bundle inspection; reject missing bundle, bad pair, unknown fields and duplicate registration |
| UC-05 specification/realization, OC-06/07 | Select/pin workload renderer without side effects; missing/ambiguous match and changed-bundle Preview tests |
| UC-06 MS-07/10/11, realization, internal/shared sequences/VOPC, OC-08 | Definition-selected render after outputs; normalized manifest equivalence and commit-after-ready tests |
| UC-08 realization, OC-10 | Confirm resource-only execution; assert score-k8s never called for database/namespace provisioning |
| UC-07 update/remove, UC-12/16 pending contracts | Preserve identity/removal, Secret revisions, no-op/partial retry and stale-token behavior |
| Shared classes/components, schema/ERD | Rendering registry and pinned intent; inspect persistence needs before declaring migration scope |
| Compatibility matrix and planner conformance | Document internal driver extension; preserve existing descriptors/Delta and all fixture results |
| Traceability/current state/code map | Link actual implementation/tests only after delivered; record delivery evidence separately |

Local verification gates: frontend/backend/shared-postgres fixtures; fresh-workspace
rerender has stable identities and Secret references; no database/namespace/PVC
output; CPU/memory/probe/replica equivalence; rejected namespace/secret/identity
patch; unknown resource fails; cross-scope and secret-leak rejection; direct/Fleet
manifest projections and route-owner separation. JSON Patch for product Delta
must not be confused with score-k8s patch-template `set/delete` syntax.

Live kind verification remains a separate scope: real database/Secret/PVC reuse,
VSO/Fleet readiness and route-only retry. AWS is not part of the first experiment.

## Scope boundary

The implementation supports internal-k8s and Definition-selected rendering.
The embedded bundle pins score-k8s 0.15.0, the configured binary digest and
adapter/provisioner source content. Wider public Score support, arbitrary
uploads/commands, durable score-k8s infrastructure state and full Humanitec
Generic Workload compatibility remain outside this decision.

The adapter uses the shared native renderer to obtain platform policy and
auxiliary Secret/ServiceAccount delivery, then builds normalized Score and
controlled patches. Deployment/Service bodies are generated by score-k8s;
protected fields are checked against policy. Probe timing, replicas, labels,
selectors, ports, pull references and Agent annotations use reviewed patches.
Only resolved non-secret environment values and typed Secret references enter
an output-only `orchestrator-bindings` resource; literal substitution syntax is
escaped before passing parameters to the CLI. The fixed provisioner encodes
Secret references with the version's `encodeSecretRef` helper.

The local registry is captured at process startup, so planning stays deterministic
and side-effect free. Before any UC-08 provisioning, Deploy verifies that the
selected binary still matches the captured digest. Restarts capture a changed
binary digest and invalidate previously previewed plan hashes. The configured
binary path is platform configuration, not an API field or Driver Input.
